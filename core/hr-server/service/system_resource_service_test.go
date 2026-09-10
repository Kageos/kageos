package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kageos/kageos/dto"
)

func TestCollectStorageComponentsReportsKnownDirectories(t *testing.T) {
	root := t.TempDir()
	writeSizedFile(t, filepath.Join(root, "mysql", "database.bin"), 1024)
	writeSizedFile(t, filepath.Join(root, "namespace", "alice", "app", "release.bin"), 2048)
	writeSizedFile(t, filepath.Join(root, "tls", "fullchain.pem"), 512)

	components := collectLocalStorageComponents(context.Background(), root, "primary")
	byKey := make(map[string]dto.SystemResourceComponent, len(components))
	for _, component := range components {
		byKey[component.Key] = component
	}
	if got := byKey["mysql"]; !got.Available || got.UsedBytes != 1024 {
		t.Fatalf("mysql component = %+v, want available 1024 bytes", got)
	}
	if got := byKey["workspaces"]; !got.Available || got.UsedBytes != 2048 {
		t.Fatalf("workspaces component = %+v, want available 2048 bytes", got)
	}
	if got := byKey["object_storage"]; got.Available || got.UsedBytes != 0 {
		t.Fatalf("object storage component = %+v, want unavailable", got)
	}
	if got := byKey["tls"]; !got.Available || got.UsedBytes != 512 {
		t.Fatalf("tls component = %+v, want available 512 bytes", got)
	}
}

func TestParseContainerVolumeSizes(t *testing.T) {
	output := `Images space usage:

Local Volumes space usage:

VOLUME NAME                       LINKS  SIZE
kageos-dev-mysql3318-data         1      21.81GB
kageos-dev-minio-data             1      4.961GB

Build cache usage:

CACHE ID  CACHE TYPE  SIZE
cache-1   regular     99GB
`
	volumes, links := parseContainerVolumeUsage(output)
	if got := volumes["kageos-dev-mysql3318-data"]; got != 21810000000 {
		t.Fatalf("mysql volume size = %d, want 21810000000", got)
	}
	if got := volumes["kageos-dev-minio-data"]; got != 4961000000 {
		t.Fatalf("minio volume size = %d, want 4961000000", got)
	}
	if links["kageos-dev-minio-data"] != 1 {
		t.Fatalf("minio links = %d, want 1", links["kageos-dev-minio-data"])
	}
	if _, parsed := volumes["cache-1"]; parsed {
		t.Fatal("build cache row must not be parsed as a volume")
	}
}

func TestAppendUnclassifiedComponentsKeepsStoragePoolsSeparate(t *testing.T) {
	snapshot := dto.SystemResourceSnapshot{
		StoragePools: []dto.SystemStoragePool{
			{Key: "primary", UsedBytes: 1000, Available: true},
			{Key: "container_engine", UsedBytes: 500, Available: true},
		},
		Components: []dto.SystemResourceComponent{
			{Key: "workspaces", PoolKey: "primary", UsedBytes: 200, Available: true},
			{Key: "mysql", PoolKey: "container_engine", UsedBytes: 100, Available: true},
		},
	}
	appendUnclassifiedComponents(&snapshot)
	if got := snapshot.Components[2]; got.Key != "other" || got.PoolKey != "primary" || got.UsedBytes != 800 {
		t.Fatalf("primary other = %+v", got)
	}
	if got := snapshot.Components[3]; got.Key != "container_other" || got.PoolKey != "container_engine" || got.UsedBytes != 400 {
		t.Fatalf("container other = %+v", got)
	}
}

func TestCollectContainerEngineStorageKeepsKnownUsageWhenCapacityIsUnavailable(t *testing.T) {
	runner := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[0] == "system" && args[1] == "df" && args[2] == "-v" {
			return []byte("Local Volumes space usage:\n\nVOLUME NAME LINKS SIZE\nkageos-dev-mysql3318-data 1 0B\n"), nil
		}
		if len(args) >= 3 && args[0] == "system" && args[1] == "df" && args[2] == "--format" {
			return []byte("{\"Type\":\"Images\",\"RawSize\":1024}\n{\"Type\":\"Containers\",\"RawSize\":256}\n{\"Type\":\"Build Cache\",\"RawSize\":512}\n"), nil
		}
		return nil, os.ErrNotExist
	}

	engine := collectContainerEngineStorage(context.Background(), "docker", runner)
	if !engine.available || engine.pool.Key != "container_engine" || engine.pool.Available {
		t.Fatalf("docker storage detection = %+v", engine)
	}
	if engine.pool.UsedBytes != 1792 {
		t.Fatalf("known docker usage = %d, want 1792", engine.pool.UsedBytes)
	}
	components := collectLocalStorageComponents(context.Background(), t.TempDir(), "primary")
	overlayEngineComponents(&components, engine)
	for _, component := range components {
		if component.Key == "mysql" && (!component.Available || component.PoolKey != "container_engine" || component.UsedBytes != 0) {
			t.Fatalf("zero-byte known mysql volume = %+v", component)
		}
		if component.Key == "build_cache" && (!component.Available || component.UsedBytes != 512) {
			t.Fatalf("build cache component = %+v", component)
		}
	}
}

func TestOverlayEngineComponentsClassifiesMinIOAndRemainingVolumes(t *testing.T) {
	components := collectLocalStorageComponents(context.Background(), t.TempDir(), "primary")
	engine := containerEngineStorage{
		pool: dto.SystemStoragePool{Key: "container_engine"},
		volumeSizes: map[string]uint64{
			"kageos-infra-minio-data":         900,
			"kageos-dev-minio-data":           1200,
			"kageos-dev-mysql3318-data":       2000,
			"another-project-persistent-data": 3000,
		},
		volumeLinks: map[string]int{
			"kageos-infra-minio-data":   0,
			"kageos-dev-minio-data":     1,
			"kageos-dev-mysql3318-data": 1,
		},
		imageBytes:      4000,
		buildCacheBytes: 5000,
	}
	overlayEngineComponents(&components, engine)
	byKey := make(map[string]dto.SystemResourceComponent, len(components))
	for _, component := range components {
		byKey[component.Key] = component
	}
	if got := byKey["object_storage"]; !got.Available || got.UsedBytes != 1200 {
		t.Fatalf("minio component = %+v, want active Docker volume", got)
	}
	if got := byKey["mysql"]; !got.Available || got.UsedBytes != 2000 {
		t.Fatalf("mysql component = %+v", got)
	}
	if got := byKey["container_storage"]; !got.Available || got.UsedBytes != 4000 {
		t.Fatalf("container storage component = %+v", got)
	}
	if got := byKey["build_cache"]; !got.Available || got.UsedBytes != 5000 {
		t.Fatalf("build cache component = %+v", got)
	}
	if got := byKey["container_volumes"]; !got.Available || got.UsedBytes != 3900 {
		t.Fatalf("other volumes component = %+v, want stale minio plus unrelated volume", got)
	}
}

func TestBuildStorageForecastWarnsBeforeThreshold(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	day := uint64(1 << 30)
	history := []dto.SystemResourceHistoryPoint{
		{CollectedAt: start, DiskUsedBytes: 60 * day},
		{CollectedAt: start.Add(10 * 24 * time.Hour), DiskUsedBytes: 70 * day},
	}
	current := dto.SystemResourceSnapshot{DiskTotalBytes: 100 * day, DiskUsedBytes: 70 * day, DiskUsedPercent: 70}

	forecast := buildStorageForecast(current, history)
	if forecast.Status != "warning" {
		t.Fatalf("status = %q, want warning", forecast.Status)
	}
	if forecast.DaysToTarget == nil || *forecast.DaysToTarget != 15 {
		t.Fatalf("days_to_target = %v, want 15", forecast.DaysToTarget)
	}
	if forecast.DailyGrowthByte != float64(day) {
		t.Fatalf("daily_growth_bytes = %f, want %d", forecast.DailyGrowthByte, day)
	}
}

func TestBuildStorageForecastMarksCriticalUsage(t *testing.T) {
	forecast := buildStorageForecast(dto.SystemResourceSnapshot{DiskTotalBytes: 100, DiskUsedBytes: 91, DiskUsedPercent: 91}, nil)
	if forecast.Status != "critical" {
		t.Fatalf("status = %q, want critical", forecast.Status)
	}
}

func TestBuildStorageForecastUsesMostConstrainedStoragePool(t *testing.T) {
	forecast := buildStorageForecast(dto.SystemResourceSnapshot{
		DiskUsedPercent: 40,
		StoragePools: []dto.SystemStoragePool{
			{Key: "primary", UsedPercent: 40, Available: true},
			{Key: "container_engine", UsedPercent: 92, Available: true},
		},
	}, nil)
	if forecast.Status != "critical" || forecast.PoolKey != "container_engine" || forecast.CurrentUsedPercent != 92 {
		t.Fatalf("forecast = %+v, want critical container engine storage", forecast)
	}
}

func TestRuntimeAccumulatorBuildsTenMinuteRollup(t *testing.T) {
	var accumulator runtimeAccumulator
	accumulator.Add(dto.SystemResourceSnapshot{CollectedAt: time.Unix(1, 0), CPUUsedPercent: 20, NetworkRxBytesPS: 100, DiskReadBytesPS: 40})
	accumulator.Add(dto.SystemResourceSnapshot{CollectedAt: time.Unix(2, 0), CPUUsedPercent: 60, NetworkRxBytesPS: 300, DiskReadBytesPS: 80})
	sample := accumulator.Sample()
	if sample.CPUUsedPercent != 40 || sample.CPUMaxPercent != 60 {
		t.Fatalf("cpu rollup = avg %.1f max %.1f", sample.CPUUsedPercent, sample.CPUMaxPercent)
	}
	if sample.NetworkRxBytesPS != 200 || sample.DiskReadBytesPS != 60 {
		t.Fatalf("io rollup = network %.1f disk %.1f", sample.NetworkRxBytesPS, sample.DiskReadBytesPS)
	}
}

func TestNextDailyRunUsesNextLocal0230(t *testing.T) {
	location := time.FixedZone("test", 8*60*60)
	before := time.Date(2026, 8, 27, 1, 0, 0, 0, location)
	if got := nextDailyRun(before, 2, 30); !got.Equal(time.Date(2026, 8, 27, 2, 30, 0, 0, location)) {
		t.Fatalf("next run before schedule = %v", got)
	}
	after := time.Date(2026, 8, 27, 3, 0, 0, 0, location)
	if got := nextDailyRun(after, 2, 30); !got.Equal(time.Date(2026, 8, 28, 2, 30, 0, 0, location)) {
		t.Fatalf("next run after schedule = %v", got)
	}
}

func TestShouldRunScheduledDailySkipsRestartAfterTodaySnapshot(t *testing.T) {
	location := time.FixedZone("test", 8*60*60)
	now := time.Date(2026, 8, 31, 10, 0, 0, 0, location)
	if shouldRunScheduledDaily(time.Date(2026, 8, 31, 2, 31, 0, 0, location), now, 2, 30) {
		t.Fatal("same scheduled period must not be collected twice")
	}
	if !shouldRunScheduledDaily(time.Date(2026, 8, 30, 2, 31, 0, 0, location), now, 2, 30) {
		t.Fatal("missed daily snapshot must be collected after the scheduled time")
	}
}

func TestCapacityCollectionDueUpgradesIncompleteDailySnapshot(t *testing.T) {
	now := time.Date(2026, 8, 31, 10, 0, 0, 0, time.Local)
	service := &SystemResourceService{lastCapacity: &dto.SystemResourceSnapshot{CollectedAt: now.Add(-time.Hour)}}
	if !service.capacityCollectionDue(now) {
		t.Fatal("legacy snapshot must be replaced even when it was collected today")
	}
	service.lastCapacity.CapacitySchemaVersion = systemCapacitySchemaVersion
	service.lastCapacity.DatabaseInventoryComplete = true
	if service.capacityCollectionDue(now) {
		t.Fatal("complete snapshot from the current schedule period must be reused")
	}
	service.lastCapacity.CapacitySchemaVersion--
	if !service.capacityCollectionDue(now) {
		t.Fatal("snapshot from an older capacity schema must be refreshed once")
	}
}

func TestBuildCapacityDailyHistoryCalculatesDatabaseDeltas(t *testing.T) {
	start := time.Date(2026, 8, 30, 2, 30, 0, 0, time.Local)
	history := buildCapacityDailyHistory([]dto.SystemResourceSnapshot{
		{CollectedAt: start, DatabaseLogicalBytes: 100, DatabaseSizeAvailable: true, DatabaseInventoryComplete: true, Databases: []dto.SystemDatabaseSize{{Kind: "platform"}, {Kind: "workspace"}}},
		{CollectedAt: start.Add(24 * time.Hour), DatabaseLogicalBytes: 160, DatabaseSizeAvailable: true, DatabaseInventoryComplete: true, Databases: []dto.SystemDatabaseSize{{Kind: "platform"}, {Kind: "workspace"}, {Kind: "workspace"}}},
	})
	if len(history) != 2 || history[1].DatabaseLogicalDelta != 60 || history[1].DatabaseCountDelta != 1 ||
		!history[1].DatabaseLogicalDeltaAvailable || !history[1].DatabaseCountDeltaAvailable {
		t.Fatalf("daily capacity history = %#v", history)
	}
	if history[1].PreviousCollectedAt == nil || !history[1].PreviousCollectedAt.Equal(start) {
		t.Fatal("comparison timestamp missing")
	}
	if history[1].PlatformDatabaseCount != 1 || history[1].WorkspaceDatabaseCount != 2 {
		t.Fatalf("daily database breakdown = %#v", history[1])
	}
}

func TestDecodePlatformStatsRejectsErrorEnvelope(t *testing.T) {
	if _, ok := decodePlatformStats([]byte(`{"error":"query failed"}`)); ok {
		t.Fatal("error response must not be accepted as platform stats")
	}
	stats, ok := decodePlatformStats([]byte(`{"workspaces_total":3,"functions_total":8}`))
	if !ok || stats.WorkspacesTotal != 3 || stats.FunctionsTotal != 8 {
		t.Fatalf("decoded platform stats = %+v, available=%v", stats, ok)
	}
}

func TestDecodeDatabaseStatsDistinguishesDisabledFromUnavailable(t *testing.T) {
	stats, ok := decodeDatabaseStats([]byte(`{"available":false,"total_bytes":0,"databases":[]}`))
	if !ok || stats.Available {
		t.Fatalf("disabled database stats = %+v, source available=%v", stats, ok)
	}
	if _, ok := decodeDatabaseStats([]byte(`{"error":"query failed"}`)); ok {
		t.Fatal("error response must not be accepted as database stats")
	}
}

func TestOverviewCanSkipHistoricalQueryForLiveRefresh(t *testing.T) {
	now := time.Now().UTC()
	service := &SystemResourceService{
		lastRuntime:  &dto.SystemResourceSnapshot{CollectedAt: now, DiskTotalBytes: 100, DiskUsedBytes: 40, DiskFreeBytes: 60, DiskUsedPercent: 40},
		lastCapacity: &dto.SystemResourceSnapshot{CollectedAt: now},
		lastPlatform: &dto.SystemPlatformMetrics{CollectedAt: now},
		tasks:        map[string]dto.SystemCollectionTaskStatus{},
	}
	overview, err := service.Overview(24*30, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.History) != 0 || overview.HistoryHours != 24*30 {
		t.Fatalf("lightweight overview history = %d points, %d hours", len(overview.History), overview.HistoryHours)
	}
	if overview.RuntimeRetentionDays != 30 || overview.CapacityRetentionDays != 365 || overview.PlatformIntervalHours != 24 {
		t.Fatalf("monitoring policy = %+v", overview)
	}
}

func TestSummaryOmitsDailyCapacityCollections(t *testing.T) {
	now := time.Now().UTC()
	service := &SystemResourceService{
		lastRuntime: &dto.SystemResourceSnapshot{CollectedAt: now, DiskTotalBytes: 100, DiskUsedBytes: 40},
		lastCapacity: &dto.SystemResourceSnapshot{
			CollectedAt:  now,
			StoragePools: []dto.SystemStoragePool{{Key: "primary"}},
			Components:   []dto.SystemResourceComponent{{Key: "mysql"}},
			Databases:    []dto.SystemDatabaseSize{{Name: "hr-server"}},
		},
		lastPlatform: &dto.SystemPlatformMetrics{CollectedAt: now},
		tasks:        map[string]dto.SystemCollectionTaskStatus{},
	}
	summary, err := service.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Current.StoragePools) != 0 || len(summary.Current.Components) != 0 || len(summary.Current.Databases) != 0 {
		t.Fatalf("live summary contains daily collections: %+v", summary.Current)
	}
	if summary.RuntimeIntervalSeconds != 30 || summary.SampleIntervalMinutes != 10 || summary.RuntimeRetentionDays != 30 {
		t.Fatalf("live summary policy = %+v", summary)
	}
}

func TestSummaryPayloadStaysSmallAsDatabaseInventoryGrows(t *testing.T) {
	now := time.Now().UTC()
	databases := make([]dto.SystemDatabaseSize, 160)
	for index := range databases {
		databases[index] = dto.SystemDatabaseSize{Name: "workspace_database", Kind: "workspace", Owner: "workspace", Directory: "/business/service", Purpose: "workspace_business_data"}
	}
	service := &SystemResourceService{
		lastRuntime:  &dto.SystemResourceSnapshot{CollectedAt: now, DiskTotalBytes: 100, DiskUsedBytes: 40},
		lastCapacity: &dto.SystemResourceSnapshot{CollectedAt: now, Databases: databases},
		lastPlatform: &dto.SystemPlatformMetrics{CollectedAt: now},
		tasks:        map[string]dto.SystemCollectionTaskStatus{},
	}
	legacy, err := service.Overview(24, false)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.Summary()
	if err != nil {
		t.Fatal(err)
	}
	legacyJSON, _ := json.Marshal(legacy)
	summaryJSON, _ := json.Marshal(summary)
	if len(summaryJSON)*4 >= len(legacyJSON) {
		t.Fatalf("summary payload %d bytes is not substantially smaller than legacy %d bytes", len(summaryJSON), len(legacyJSON))
	}
}

func TestFilterAndPaginateDatabasesUsesServerSideScopeAndKeyword(t *testing.T) {
	databases := []dto.SystemDatabaseSize{
		{Name: "hr-server", Kind: "platform", Owner: "hr-server"},
		{Name: "kage_a", Kind: "workspace", Owner: "Alice", Directory: "/sales"},
		{Name: "kage_b", Kind: "workspace", Owner: "Bob", Directory: "/support"},
	}
	items, total, platformCount, workspaceCount := filterAndPaginateDatabases(databases, 1, 1, "workspace", "alice")
	if total != 1 || len(items) != 1 || items[0].Name != "kage_a" {
		t.Fatalf("filtered database page = %#v, total=%d", items, total)
	}
	if platformCount != 1 || workspaceCount != 2 {
		t.Fatalf("database kind counts = platform %d, workspace %d", platformCount, workspaceCount)
	}
	items, total, _, _ = filterAndPaginateDatabases(databases, 2, 2, "all", "")
	if total != 3 || len(items) != 1 || items[0].Name != "kage_b" {
		t.Fatalf("second database page = %#v, total=%d", items, total)
	}
}

func TestDatabasesCanSkipRepeatedDailyHistory(t *testing.T) {
	now := time.Now().UTC()
	service := &SystemResourceService{
		lastRuntime: &dto.SystemResourceSnapshot{CollectedAt: now},
		lastCapacity: &dto.SystemResourceSnapshot{CollectedAt: now, Databases: []dto.SystemDatabaseSize{
			{Name: "hr-server", Kind: "platform"},
			{Name: "kage_sales", Kind: "workspace", Owner: "sales"},
		}},
		lastPlatform: &dto.SystemPlatformMetrics{CollectedAt: now},
		tasks:        map[string]dto.SystemCollectionTaskStatus{},
	}
	result, err := service.Databases(1, 20, "workspace", "sales", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Items) != 1 || len(result.CapacityHistory) != 0 {
		t.Fatalf("database response = %+v", result)
	}
}

func TestUsageBaselineUsesLatestSnapshotAtOrBeforeCutoff(t *testing.T) {
	cutoff := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	history := []dto.SystemPlatformMetrics{
		{CollectedAt: cutoff.Add(-24 * time.Hour), Usage: dto.SystemUsageSnapshot{Available: true, CollectedAt: cutoff.Add(-24 * time.Hour), Functions: []dto.SystemFunctionUsageSnapshot{{Path: "/a", TotalCalls: 4}}}},
		{CollectedAt: cutoff.Add(-time.Hour), Usage: dto.SystemUsageSnapshot{Available: true, CollectedAt: cutoff.Add(-time.Hour), Functions: []dto.SystemFunctionUsageSnapshot{{Path: "/a", TotalCalls: 8}}}},
		{CollectedAt: cutoff.Add(time.Hour), Usage: dto.SystemUsageSnapshot{Available: true, CollectedAt: cutoff.Add(time.Hour), Functions: []dto.SystemFunctionUsageSnapshot{{Path: "/a", TotalCalls: 12}}}},
	}
	baseline, ok := usageBaseline(history, cutoff)
	if !ok || len(baseline.Functions) != 1 || baseline.Functions[0].TotalCalls != 8 {
		t.Fatalf("baseline = %+v, ok=%v", baseline, ok)
	}
}

func TestBuildUsageDailyHistoryUsesStoredCompleteDaysAndLiveToday(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.Local)
	history := []dto.SystemPlatformMetrics{{Usage: dto.SystemUsageSnapshot{
		Available: true, OperationDay: "2026-09-01", OperationsYesterday: 20, FailedOperationsYesterday: 2,
	}}}
	current := dto.SystemUsageSnapshot{Available: true, CollectedAt: now, OperationsToday: 7, FailedOperationsToday: 1}
	points := buildUsageDailyHistory(history, current)
	if len(points) != 2 || points[0].Date != "2026-09-01" || points[0].Operations != 20 || points[1].Date != "2026-09-02" || points[1].Operations != 7 {
		t.Fatalf("daily usage = %+v", points)
	}
}

func TestUsageRankingPaginationKeepsItemsBeyondFirstPage(t *testing.T) {
	directories := make([]dto.SystemDirectoryUsageItem, 0, 23)
	functions := make([]dto.SystemFunctionUsageItem, 0, 23)
	for index := 0; index < 23; index++ {
		directories = append(directories, dto.SystemDirectoryUsageItem{Path: fmt.Sprintf("/directory/%02d", index)})
		functions = append(functions, dto.SystemFunctionUsageItem{SystemFunctionUsageSnapshot: dto.SystemFunctionUsageSnapshot{Path: fmt.Sprintf("/function/%02d", index)}})
	}
	directoryPage := paginateDirectories(directories, 2, 10)
	functionPage := paginateFunctions(functions, 3, 10)
	if len(directoryPage) != 10 || directoryPage[0].Path != "/directory/10" {
		t.Fatalf("directory page = %+v", directoryPage)
	}
	if len(functionPage) != 3 || functionPage[0].Path != "/function/20" {
		t.Fatalf("function page = %+v", functionPage)
	}
}

func writeSizedFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScopedCapacityHistoryKeepsGapsAndSingleDatabase(t *testing.T) {
	now := time.Date(2026, 9, 7, 3, 0, 0, 0, time.Local)
	snapshots := []dto.SystemResourceSnapshot{}
	for _, offset := range []int{-3, -1, 0} {
		snapshots = append(snapshots, dto.SystemResourceSnapshot{CollectedAt: now.AddDate(0, 0, offset), DatabaseInventoryComplete: true, DatabaseSizeAvailable: true, Databases: []dto.SystemDatabaseSize{{Name: "a", Kind: "workspace", Status: "active", UsedBytes: uint64(100 + offset)}, {Name: "b", Kind: "platform", Status: "active", UsedBytes: 999}}})
	}
	points := scopedCapacityHistory(snapshots, "workspace", DatabaseHistoryOptions{Days: 7, Name: "a"}, now)
	if len(points) != 7 || points[4].DatabaseSizeAvailable || points[5].DatabaseLogicalDeltaAvailable {
		t.Fatalf("gap was fabricated: %+v", points)
	}
	if points[6].DatabaseLogicalBytes != 100 || points[6].DatabaseCount != 1 || points[6].DatabaseLogicalDelta != 1 || !points[6].DatabaseLogicalDeltaAvailable {
		t.Fatalf("wrong scoped values: %+v", points[6])
	}
	points = scopedCapacityHistory(snapshots, "all", DatabaseHistoryOptions{Days: 30, Name: "unknown"}, now)
	if len(points) != 30 || points[29].DatabaseSizeAvailable {
		t.Fatal("missing database must not have a zero-value sample")
	}
}

func TestCapacityScopeVersionChangeDoesNotReportDailyGrowth(t *testing.T) {
	now := time.Now()
	points := buildCapacityDailyHistory([]dto.SystemResourceSnapshot{
		{CollectedAt: now.AddDate(0, 0, -1), CapacitySchemaVersion: 2, DatabaseInventoryComplete: true, DatabaseSizeAvailable: true, DatabaseLogicalBytes: 100},
		{CollectedAt: now, CapacitySchemaVersion: 3, DatabaseInventoryComplete: true, DatabaseSizeAvailable: true, DatabaseLogicalBytes: 500},
	})
	if len(points) != 2 || points[1].DatabaseLogicalDeltaAvailable || points[1].DatabaseCountDeltaAvailable {
		t.Fatal("discovery scope change must not be reported as daily growth")
	}
}

func TestDatabaseInventoryDoesNotMergeSameNamesOnDifferentInstances(t *testing.T) {
	local := []dto.SystemDatabaseSize{{SourceID: "one", Name: "orders", Kind: "unmanaged", UsedBytes: 100}, {SourceID: "two", Name: "orders", Kind: "unmanaged", UsedBytes: 200}}
	remote := []dto.SystemDatabaseSize{{SourceID: "two", Name: "orders", Kind: "workspace", UsedBytes: 200}}
	merged := mergeDatabaseInventory(local, remote)
	if len(merged) != 2 || merged[0].SourceID != "one" || merged[1].Kind != "workspace" {
		t.Fatalf("distinct instances lost: %+v", merged)
	}
	now := time.Now()
	points := scopedCapacityHistory([]dto.SystemResourceSnapshot{{CollectedAt: now, DatabaseInventoryComplete: true, DatabaseSizeAvailable: true, Databases: merged}}, "all", DatabaseHistoryOptions{Days: 7, Name: "one/orders"}, now)
	if points[6].DatabaseLogicalBytes != 100 || points[6].DatabaseCount != 1 {
		t.Fatalf("single database identity ignored: %+v", points[6])
	}
}
