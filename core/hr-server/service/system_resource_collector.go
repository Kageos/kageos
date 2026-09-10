package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/config"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"gopkg.in/yaml.v3"
)

type systemResourceCollector interface {
	Collect() (dto.SystemResourceSnapshot, error)
	CollectRuntime() (dto.SystemResourceSnapshot, error)
	CollectCapacity(context.Context) (dto.SystemResourceSnapshot, error)
}

const systemCapacitySchemaVersion = 4

type commandOutputRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

type localSystemResourceCollector struct {
	now        func() time.Time
	runCommand commandOutputRunner
	counterMu  sync.Mutex
	lastAt     time.Time
	lastNetRx  uint64
	lastNetTx  uint64
	lastRead   uint64
	lastWrite  uint64
}

func newLocalSystemResourceCollector() *localSystemResourceCollector {
	return &localSystemResourceCollector{now: time.Now, runCommand: runResourceCommand}
}

func (c *localSystemResourceCollector) Collect() (dto.SystemResourceSnapshot, error) {
	runtimeSnapshot, err := c.CollectRuntime()
	if err != nil {
		return dto.SystemResourceSnapshot{}, err
	}
	capacitySnapshot, err := c.CollectCapacity(context.Background())
	if err != nil {
		return dto.SystemResourceSnapshot{}, err
	}
	mergeCapacitySnapshot(&runtimeSnapshot, capacitySnapshot)
	return runtimeSnapshot, nil
}

func (c *localSystemResourceCollector) CollectRuntime() (dto.SystemResourceSnapshot, error) {
	environment, storageRoot := detectSystemEnvironment()
	snapshot := dto.SystemResourceSnapshot{
		CollectedAt: c.now().UTC(), OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH,
		CPUCores: runtime.NumCPU(), DiskMount: storageRoot, Environment: environment,
	}
	if info, err := host.Info(); err == nil {
		snapshot.Hostname, snapshot.UptimeSeconds = info.Hostname, info.Uptime
		if info.OS != "" {
			snapshot.OperatingSystem = info.OS
		}
		if info.KernelArch != "" {
			snapshot.Architecture = info.KernelArch
		}
	} else {
		snapshot.Hostname, _ = os.Hostname()
	}
	if values, err := cpu.Percent(0, false); err == nil && len(values) > 0 {
		snapshot.CPUUsedPercent, snapshot.CPUAvailable = values[0], true
	}
	if avg, err := load.Avg(); err == nil && avg != nil {
		snapshot.Load1, snapshot.Load5, snapshot.Load15, snapshot.LoadAvailable = avg.Load1, avg.Load5, avg.Load15, true
	}
	if memory, err := mem.VirtualMemory(); err == nil && memory != nil && memory.Total > 0 {
		snapshot.MemoryTotalBytes, snapshot.MemoryUsedBytes = memory.Total, memory.Used
		snapshot.MemoryUsedPercent, snapshot.MemoryAvailable = memory.UsedPercent, true
	}
	if swap, err := mem.SwapMemory(); err == nil && swap != nil {
		snapshot.SwapTotalBytes, snapshot.SwapUsedBytes, snapshot.SwapUsedPercent = swap.Total, swap.Used, swap.UsedPercent
	}
	primaryPool, err := collectStoragePool("primary", primaryPoolName(environment), storageRoot, true)
	if err != nil {
		return dto.SystemResourceSnapshot{}, fmt.Errorf("read storage filesystem %s: %w", storageRoot, err)
	}
	snapshot.StoragePools = append(snapshot.StoragePools, primaryPool)
	copyPrimaryDiskFields(&snapshot, primaryPool)
	c.collectIOCounters(&snapshot)
	return snapshot, nil
}

func (c *localSystemResourceCollector) CollectCapacity(ctx context.Context) (dto.SystemResourceSnapshot, error) {
	environment, storageRoot := detectSystemEnvironment()
	snapshot := dto.SystemResourceSnapshot{
		CapacitySchemaVersion: systemCapacitySchemaVersion,
		CollectedAt:           c.now().UTC(),
		DiskMount:             storageRoot,
		Environment:           environment,
	}
	primaryPool, err := collectStoragePool("primary", primaryPoolName(environment), storageRoot, true)
	if err != nil {
		return dto.SystemResourceSnapshot{}, fmt.Errorf("read storage filesystem %s: %w", storageRoot, err)
	}
	snapshot.StoragePools = append(snapshot.StoragePools, primaryPool)
	copyPrimaryDiskFields(&snapshot, primaryPool)
	snapshot.Components = collectLocalStorageComponents(ctx, storageRoot, "primary")
	if err := ctx.Err(); err != nil {
		return dto.SystemResourceSnapshot{}, fmt.Errorf("capacity scan cancelled: %w", err)
	}
	if environment.Mode == "development" && environment.ContainerEngine != "none" {
		engine := collectContainerEngineStorage(ctx, environment.ContainerEngine, c.runCommand)
		if engine.available {
			snapshot.Environment.ContainerRemote = engine.remote
			snapshot.StoragePools = append(snapshot.StoragePools, engine.pool)
			overlayEngineComponents(&snapshot.Components, engine)
		}
	}
	appendUnclassifiedComponents(&snapshot)
	return snapshot, nil
}

func (c *localSystemResourceCollector) collectIOCounters(snapshot *dto.SystemResourceSnapshot) {
	var netRx, netTx, readBytes, writeBytes uint64
	if counters, err := gnet.IOCounters(false); err == nil && len(counters) > 0 {
		netRx, netTx = counters[0].BytesRecv, counters[0].BytesSent
		snapshot.NetworkAvailable = true
	}
	if counters, err := disk.IOCounters(); err == nil {
		snapshot.DiskIOAvailable = true
		for _, counter := range counters {
			readBytes += counter.ReadBytes
			writeBytes += counter.WriteBytes
		}
	}
	snapshot.NetworkRxBytes, snapshot.NetworkTxBytes = netRx, netTx
	snapshot.DiskReadBytes, snapshot.DiskWriteBytes = readBytes, writeBytes
	c.counterMu.Lock()
	defer c.counterMu.Unlock()
	if !c.lastAt.IsZero() {
		seconds := snapshot.CollectedAt.Sub(c.lastAt).Seconds()
		if seconds > 0 {
			snapshot.NetworkRxBytesPS = counterRate(netRx, c.lastNetRx, seconds)
			snapshot.NetworkTxBytesPS = counterRate(netTx, c.lastNetTx, seconds)
			snapshot.DiskReadBytesPS = counterRate(readBytes, c.lastRead, seconds)
			snapshot.DiskWriteBytesPS = counterRate(writeBytes, c.lastWrite, seconds)
		}
	}
	c.lastAt, c.lastNetRx, c.lastNetTx = snapshot.CollectedAt, netRx, netTx
	c.lastRead, c.lastWrite = readBytes, writeBytes
}

func counterRate(current, previous uint64, seconds float64) float64 {
	if current < previous || seconds <= 0 {
		return 0
	}
	return float64(current-previous) / seconds
}

func mergeCapacitySnapshot(target *dto.SystemResourceSnapshot, capacity dto.SystemResourceSnapshot) {
	target.CapacitySchemaVersion = capacity.CapacitySchemaVersion
	if target.DiskMount == "" {
		target.DiskMount = capacity.DiskMount
	}
	target.Environment, target.StoragePools, target.Components = capacity.Environment, capacity.StoragePools, capacity.Components
	target.DatabaseLogicalBytes, target.DatabaseSizeAvailable = capacity.DatabaseLogicalBytes, capacity.DatabaseSizeAvailable
	target.DatabaseInventoryComplete = capacity.DatabaseInventoryComplete
	target.Databases = capacity.Databases
	target.LargestDatabases = capacity.LargestDatabases
	if len(target.Databases) == 0 && len(target.LargestDatabases) > 0 {
		target.Databases = target.LargestDatabases
	}
	for index := range target.StoragePools {
		if target.StoragePools[index].Primary && target.DiskTotalBytes > 0 {
			target.StoragePools[index].TotalBytes, target.StoragePools[index].UsedBytes = target.DiskTotalBytes, target.DiskUsedBytes
			target.StoragePools[index].FreeBytes, target.StoragePools[index].UsedPercent = target.DiskFreeBytes, target.DiskUsedPercent
		}
	}
}

func detectSystemEnvironment() (dto.SystemEnvironmentInfo, string) {
	root := config.GetKageosRoot()
	environment := dto.SystemEnvironmentInfo{Mode: "production", Deployment: "host", ContainerEngine: "none", StorageRootSource: "auto"}
	if config.IsDevMode() {
		environment.Mode, environment.Deployment = "development", "source"
		environment.ContainerEngine = config.GetDevEngine()
		if environment.ContainerEngine == "" || environment.ContainerEngine == config.ConfigDevEngineAuto {
			environment.ContainerEngine = detectAvailableContainerEngine()
		}
		if isDirectory(root) {
			environment.StorageRootSource = "development_workspace"
			return environment, root
		}
	}
	environment.Containerized = isRunningInContainer()
	if environment.Containerized {
		environment.Deployment = "container"
	}
	for _, candidate := range []struct{ path, source string }{
		{strings.TrimSpace(os.Getenv("KAGEOS_MONITOR_STORAGE_ROOT")), "environment_override"},
		{strings.TrimSpace(os.Getenv("KAGEOS_AIO_DATA_DIR")), "aio_environment"},
	} {
		if isDirectory(candidate.path) {
			environment.StorageRootSource = candidate.source
			if candidate.source == "aio_environment" {
				environment.Deployment = "aio"
			}
			return environment, filepath.Clean(candidate.path)
		}
	}
	if prodRoot := discoverProductionStorageRoot(root); isDirectory(prodRoot) {
		environment.StorageRootSource = "production_config"
		return environment, filepath.Clean(prodRoot)
	}
	if isDirectory("/var/lib/kageos") {
		environment.Deployment, environment.StorageRootSource = "aio", "aio_default"
		return environment, "/var/lib/kageos"
	}
	if isDirectory("/app") {
		environment.StorageRootSource = "container_workdir"
		return environment, "/app"
	}
	if isDirectory(root) {
		environment.StorageRootSource = "source_workspace"
		return environment, root
	}
	environment.StorageRootSource = "filesystem_root"
	return environment, filesystemRoot()
}

func primaryPoolName(environment dto.SystemEnvironmentInfo) string {
	if environment.Mode == "development" {
		return "Development host storage"
	}
	return "kageos persistent storage"
}

func discoverProductionStorageRoot(root string) string {
	if root == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(root, ".kageos", "prod", "kage.yaml"))
	if err != nil {
		return ""
	}
	var payload struct {
		Storage struct {
			Root string `yaml:"root"`
		} `yaml:"storage"`
	}
	if yaml.Unmarshal(data, &payload) != nil {
		return ""
	}
	return strings.TrimSpace(payload.Storage.Root)
}

func detectAvailableContainerEngine() string {
	for _, name := range []string{"podman", "docker"} {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return "none"
}

func isRunningInContainer() bool {
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	data, _ := os.ReadFile("/proc/1/cgroup")
	value := string(data)
	return strings.Contains(value, "docker") || strings.Contains(value, "libpod") || strings.Contains(value, "kubepods")
}

func filesystemRoot() string {
	cwd, _ := os.Getwd()
	volume := filepath.VolumeName(cwd)
	if volume == "" {
		return string(os.PathSeparator)
	}
	return volume + string(os.PathSeparator)
}

func collectStoragePool(key, name, path string, primary bool) (dto.SystemStoragePool, error) {
	usage, err := disk.Usage(path)
	if err != nil {
		return dto.SystemStoragePool{}, err
	}
	return dto.SystemStoragePool{Key: key, Name: name, Path: path, TotalBytes: usage.Total, UsedBytes: usage.Used,
		FreeBytes: usage.Free, UsedPercent: usage.UsedPercent, Primary: primary, Available: true}, nil
}

func copyPrimaryDiskFields(snapshot *dto.SystemResourceSnapshot, pool dto.SystemStoragePool) {
	snapshot.DiskTotalBytes, snapshot.DiskUsedBytes = pool.TotalBytes, pool.UsedBytes
	snapshot.DiskFreeBytes, snapshot.DiskUsedPercent = pool.FreeBytes, pool.UsedPercent
}

func collectLocalStorageComponents(ctx context.Context, root, poolKey string) []dto.SystemResourceComponent {
	definitions := []struct {
		key, name string
		dirs      []string
	}{
		{key: "mysql", name: "MySQL", dirs: []string{"mysql"}},
		{key: "object_storage", name: "Object storage", dirs: []string{"minio"}},
		{key: "workspaces", name: "Workspaces", dirs: []string{"namespace"}},
		{key: "runtime_data", name: "Runtime data", dirs: []string{"data"}},
		{key: "logs", name: "Logs", dirs: []string{"logs"}},
		{key: "tls", name: "TLS certificates", dirs: []string{"tls"}},
		{key: "container_storage", name: "Container images and layers", dirs: []string{"podman_storage", "containers"}},
	}
	components := make([]dto.SystemResourceComponent, 0, len(definitions))
	for _, definition := range definitions {
		component := dto.SystemResourceComponent{Key: definition.key, Name: definition.name, PoolKey: poolKey}
		for _, dir := range definition.dirs {
			path := filepath.Join(root, dir)
			if !isDirectory(path) {
				continue
			}
			if size, err := directorySize(ctx, path); err == nil {
				component.UsedBytes += size
				component.Available = true
			}
		}
		components = append(components, component)
	}
	return components
}

type containerEngineStorage struct {
	available       bool
	remote          bool
	pool            dto.SystemStoragePool
	volumeSizes     map[string]uint64
	volumeLinks     map[string]int
	imageBytes      uint64
	buildCacheBytes uint64
}

func collectContainerEngineStorage(parent context.Context, engine string, run commandOutputRunner) containerEngineStorage {
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()
	result := containerEngineStorage{volumeSizes: map[string]uint64{}, volumeLinks: map[string]int{}}
	if engine == "podman" {
		if output, err := run(ctx, engine, "info", "--format", "json"); err == nil {
			var info struct {
				Host struct {
					ServiceIsRemote bool `json:"serviceIsRemote"`
				} `json:"host"`
				Store struct {
					GraphRoot          string `json:"graphRoot"`
					GraphRootAllocated uint64 `json:"graphRootAllocated"`
					GraphRootUsed      uint64 `json:"graphRootUsed"`
				} `json:"store"`
			}
			if json.Unmarshal(output, &info) == nil && info.Store.GraphRootAllocated > 0 {
				result.available, result.remote = true, info.Host.ServiceIsRemote
				result.pool = dto.SystemStoragePool{Key: "container_engine", Name: "Podman storage", Path: info.Store.GraphRoot,
					TotalBytes: info.Store.GraphRootAllocated, UsedBytes: info.Store.GraphRootUsed,
					FreeBytes:   info.Store.GraphRootAllocated - minUint64(info.Store.GraphRootAllocated, info.Store.GraphRootUsed),
					UsedPercent: percent(info.Store.GraphRootUsed, info.Store.GraphRootAllocated), Available: true}
			}
		}
	}
	if output, err := run(ctx, engine, "system", "df", "-v"); err == nil {
		result.available = true
		result.volumeSizes, result.volumeLinks = parseContainerVolumeUsage(string(output))
	}
	if output, err := run(ctx, engine, "system", "df", "--format", "{{json .}}"); err == nil {
		result.available = true
		result.imageBytes, result.buildCacheBytes = parseContainerDiskUsageBytes(output)
	}
	if result.available && result.pool.Key == "" {
		used := result.imageBytes + result.buildCacheBytes
		for _, size := range result.volumeSizes {
			used += size
		}
		result.pool = dto.SystemStoragePool{
			Key:       "container_engine",
			Name:      strings.ToUpper(engine[:1]) + engine[1:] + " storage",
			UsedBytes: used,
			Available: false,
		}
	}
	return result
}

func overlayEngineComponents(components *[]dto.SystemResourceComponent, engine containerEngineStorage) {
	setComponent := func(key, name string, size uint64) {
		for index := range *components {
			if (*components)[index].Key == key {
				(*components)[index] = dto.SystemResourceComponent{Key: key, Name: name, PoolKey: engine.pool.Key, UsedBytes: size, Available: true}
				return
			}
		}
		*components = append(*components, dto.SystemResourceComponent{Key: key, Name: name, PoolKey: engine.pool.Key, UsedBytes: size, Available: true})
	}
	classifiedVolumes := map[string]struct{}{}
	if name, size, ok := preferredVolumeSize(engine.volumeSizes, engine.volumeLinks,
		"kageos-dev-mysql3318-data", "kageos-infra-mysql-data", "ai-agent-os-infra-mysql-data"); ok {
		setComponent("mysql", "MySQL", size)
		classifiedVolumes[name] = struct{}{}
	}
	if name, size, ok := preferredVolumeSize(engine.volumeSizes, engine.volumeLinks,
		"kageos-dev-minio-data", "kageos-infra-minio-data", "ai-agent-os-infra-minio-data"); ok {
		setComponent("object_storage", "MinIO object data", size)
		classifiedVolumes[name] = struct{}{}
	}
	if engine.imageBytes > 0 {
		setComponent("container_storage", "Container images and layers", engine.imageBytes)
	}
	if engine.buildCacheBytes > 0 {
		setComponent("build_cache", "Container build cache", engine.buildCacheBytes)
	}
	var otherVolumeBytes uint64
	for name, size := range engine.volumeSizes {
		if _, classified := classifiedVolumes[name]; !classified {
			otherVolumeBytes += size
		}
	}
	if otherVolumeBytes > 0 {
		setComponent("container_volumes", "Other named volumes", otherVolumeBytes)
	}
}

func appendUnclassifiedComponents(snapshot *dto.SystemResourceSnapshot) {
	for _, pool := range snapshot.StoragePools {
		if !pool.Available {
			continue
		}
		var classified uint64
		for _, component := range snapshot.Components {
			if component.Available && component.PoolKey == pool.Key {
				classified += component.UsedBytes
			}
		}
		other := uint64(0)
		if pool.UsedBytes > classified {
			other = pool.UsedBytes - classified
		}
		key, name := "other", "Other files on filesystem"
		if pool.Key == "container_engine" {
			key, name = "container_other", "Other container engine data"
		}
		snapshot.Components = append(snapshot.Components, dto.SystemResourceComponent{Key: key, Name: name, PoolKey: pool.Key, UsedBytes: other, Available: true})
	}
}

func parseContainerVolumeSizes(output string) map[string]uint64 {
	sizes, _ := parseContainerVolumeUsage(output)
	return sizes
}

func parseContainerVolumeUsage(output string) (map[string]uint64, map[string]int) {
	sizes := map[string]uint64{}
	links := map[string]int{}
	inVolumes := false
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Local Volumes space usage:") {
			inVolumes = true
			continue
		}
		if inVolumes && strings.HasSuffix(line, "usage:") {
			break
		}
		if !inVolumes || line == "" || strings.HasPrefix(line, "VOLUME NAME") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if size, err := parseHumanBytes(fields[len(fields)-1]); err == nil {
			sizes[fields[0]] = size
			if count, err := strconv.Atoi(fields[len(fields)-2]); err == nil {
				links[fields[0]] = count
			}
		}
	}
	return sizes, links
}

func parseContainerDiskUsageBytes(output []byte) (uint64, uint64) {
	var imageBytes, buildCacheBytes uint64
	for _, line := range strings.Split(string(output), "\n") {
		var row struct {
			Type    string `json:"Type"`
			RawSize uint64 `json:"RawSize"`
		}
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		switch strings.ToLower(strings.ReplaceAll(row.Type, " ", "")) {
		case "images", "containers":
			imageBytes += row.RawSize
		case "buildcache":
			buildCacheBytes += row.RawSize
		}
	}
	return imageBytes, buildCacheBytes
}

func parseHumanBytes(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	units := []struct {
		suffix string
		factor float64
	}{{"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"KB", 1e3}, {"B", 1}}
	for _, unit := range units {
		if !strings.HasSuffix(value, unit.suffix) {
			continue
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, unit.suffix)), 64)
		if err != nil {
			return 0, err
		}
		return uint64(parsed * unit.factor), nil
	}
	return 0, fmt.Errorf("unsupported byte value %q", value)
}

func preferredVolumeSize(values map[string]uint64, links map[string]int, names ...string) (string, uint64, bool) {
	selectedName := ""
	selectedLinks := -1
	for _, name := range names {
		_, ok := values[name]
		if !ok {
			continue
		}
		if selectedName == "" || links[name] > selectedLinks {
			selectedName, selectedLinks = name, links[name]
		}
	}
	if selectedName == "" {
		return "", 0, false
	}
	return selectedName, values[selectedName], true
}

func runResourceCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func isDirectory(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func directorySize(ctx context.Context, root string) (uint64, error) {
	var size uint64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if path == root {
				return walkErr
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return nil
		}
		if info, err := entry.Info(); err == nil && info.Mode().IsRegular() {
			size += uint64(info.Size())
		}
		return nil
	})
	return size, err
}

func minUint64(left, right uint64) uint64 {
	if left < right {
		return left
	}
	return right
}
func percent(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) * 100 / float64(total)
}
