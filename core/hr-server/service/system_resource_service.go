package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kageos/kageos/core/hr-server/model"
	"github.com/kageos/kageos/core/hr-server/repository"
	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/logger"
	"github.com/kageos/kageos/pkg/subjects"
	"github.com/nats-io/nats.go"
)

const (
	runtimeSampleInterval     = 30 * time.Second
	runtimePersistInterval    = 10 * time.Minute
	platformRetryInterval     = time.Minute
	runtimeRetention          = 30 * 24 * time.Hour
	platformRetention         = 90 * 24 * time.Hour
	capacityRetention         = 365 * 24 * time.Hour
	platformRunHour           = 2
	platformRunMinute         = 0
	capacityRunHour           = 2
	capacityRunMinute         = 30
	capacityCollectionTimeout = 30 * time.Minute
	capacityRetryInterval     = 5 * time.Minute
)

type SystemResourceService struct {
	repo         *repository.SystemResourceRepository
	collector    systemResourceCollector
	natsConn     *nats.Conn
	mu           sync.Mutex
	capacityMu   sync.Mutex
	lastRuntime  *dto.SystemResourceSnapshot
	lastCapacity *dto.SystemResourceSnapshot
	lastPlatform *dto.SystemPlatformMetrics
	tasks        map[string]dto.SystemCollectionTaskStatus
	accumulator  runtimeAccumulator
	lastPersist  time.Time
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

func NewSystemResourceService(repo *repository.SystemResourceRepository, connections ...*nats.Conn) *SystemResourceService {
	result := &SystemResourceService{
		repo: repo, collector: newLocalSystemResourceCollector(),
		tasks: map[string]dto.SystemCollectionTaskStatus{
			"runtime": {Key: "runtime", Status: "pending"}, "platform": {Key: "platform", Status: "pending"}, "capacity": {Key: "capacity", Status: "pending"},
		},
	}
	if len(connections) > 0 {
		result.natsConn = connections[0]
	}
	return result
}

func (s *SystemResourceService) Start(ctx context.Context) {
	if s == nil || s.cancel != nil {
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.wg.Add(3)
	go s.runtimeLoop(workerCtx)
	go s.platformLoop(workerCtx)
	go s.capacityLoop(workerCtx)
}

func (s *SystemResourceService) Stop() {
	if s == nil || s.cancel == nil {
		return
	}
	s.cancel()
	s.wg.Wait()
	s.cancel = nil
}

func (s *SystemResourceService) runtimeLoop(ctx context.Context) {
	defer s.wg.Done()
	s.collectRuntime(ctx, true)
	ticker := time.NewTicker(runtimeSampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.collectRuntime(ctx, false)
		}
	}
}

func (s *SystemResourceService) platformLoop(ctx context.Context) {
	defer s.wg.Done()
	if stored, err := s.repo.LatestPlatform(); err == nil {
		s.mu.Lock()
		s.lastPlatform = stored
		s.mu.Unlock()
		s.restoreTask("platform", stored.CollectedAt)
	}
	for {
		now := time.Now()
		if shouldRunScheduledDaily(s.platformCollectedAt(), now, platformRunHour, platformRunMinute) {
			if !s.collectPlatform(ctx) {
				if !waitFor(ctx, platformRetryInterval) {
					return
				}
				s.collectPlatform(ctx)
			}
		}
		next := nextDailyRun(time.Now(), platformRunHour, platformRunMinute)
		s.setTaskNextRun("platform", next)
		if !waitUntil(ctx, next) {
			return
		}
	}
}

func (s *SystemResourceService) capacityLoop(ctx context.Context) {
	defer s.wg.Done()
	if stored, err := s.repo.LatestCapacity(); err == nil {
		s.mu.Lock()
		s.lastCapacity = stored
		s.mu.Unlock()
		s.restoreTask("capacity", stored.CollectedAt)
	}
	for {
		now := time.Now()
		if s.capacityCollectionDue(now) {
			if !s.collectCapacity(ctx) && waitFor(ctx, capacityRetryInterval) {
				s.collectCapacity(ctx)
			}
		}
		next := nextDailyRun(time.Now(), capacityRunHour, capacityRunMinute)
		s.setTaskNextRun("capacity", next)
		if !waitUntil(ctx, next) {
			return
		}
	}
}

func (s *SystemResourceService) collectRuntime(ctx context.Context, forcePersist bool) {
	started := time.Now()
	s.markTaskStarted("runtime", started)
	snapshot, err := s.collector.CollectRuntime()
	if err != nil {
		s.markTaskFinished("runtime", started, err, started.Add(runtimeSampleInterval))
		logger.Warnf(ctx, "[SystemResourceMonitor] runtime collection failed: %v", err)
		return
	}
	s.mu.Lock()
	s.lastRuntime = &snapshot
	s.accumulator.Add(snapshot)
	shouldPersist := forcePersist || s.lastPersist.IsZero() || snapshot.CollectedAt.Sub(s.lastPersist) >= runtimePersistInterval
	var sample *model.SystemResourceSample
	if shouldPersist {
		value := s.accumulator.Sample()
		sample = &value
	}
	s.mu.Unlock()
	if sample != nil {
		if err := s.repo.Create(sample); err != nil {
			logger.Warnf(ctx, "[SystemResourceMonitor] persist runtime rollup failed: %v", err)
			s.markTaskPartial("runtime", started, "runtime metrics collected but history persistence failed", started.Add(runtimeSampleInterval))
			return
		}
		s.mu.Lock()
		s.accumulator.Reset()
		s.lastPersist = snapshot.CollectedAt
		s.mu.Unlock()
	}
	s.markTaskFinished("runtime", started, nil, started.Add(runtimeSampleInterval))
}

func (s *SystemResourceService) collectPlatform(ctx context.Context) bool {
	started := time.Now()
	s.markTaskStarted("platform", started)
	metrics, err := s.repo.CollectPlatformMetrics(started)
	if err == nil {
		if stats, ok := s.requestPlatformStats(subjects.PlatformAppStatsQuerySubject); ok {
			metrics.WorkspacesTotal, metrics.WorkspacesEnabled = stats.WorkspacesTotal, stats.WorkspacesEnabled
			metrics.ServiceDirectories, metrics.FunctionsTotal, metrics.AppStatsAvailable = stats.ServiceDirectories, stats.FunctionsTotal, true
			metrics.Usage = stats.Usage
		}
		if stats, ok := s.requestPlatformStats(subjects.PlatformRuntimeStatsQuerySubject); ok {
			metrics.AppDatabasesTotal, metrics.RuntimeStatsAvailable = stats.AppDatabasesTotal, true
		}
		if stats, ok := s.requestPlatformStats(subjects.PlatformTimerStatsQuerySubject); ok {
			metrics.ScheduledTasksTotal, metrics.ScheduledTasksActive, metrics.TimerStatsAvailable = stats.ScheduledTasksTotal, stats.ScheduledTasksActive, true
		}
	}
	if err == nil {
		err = s.repo.CreatePlatform(metrics)
	}
	if err != nil {
		s.markTaskFinished("platform", started, err, started.Add(platformRetryInterval))
		logger.Warnf(ctx, "[SystemResourceMonitor] platform collection failed: %v", err)
		return false
	}
	s.mu.Lock()
	s.lastPlatform = &metrics
	s.mu.Unlock()
	if !metrics.AppStatsAvailable || !metrics.RuntimeStatsAvailable || !metrics.TimerStatsAvailable {
		s.markTaskPartial("platform", started, "one or more platform metric sources are unavailable", started.Add(platformRetryInterval))
		return false
	}
	s.markTaskFinished("platform", started, nil, nextDailyRun(time.Now(), platformRunHour, platformRunMinute))
	return true
}

func (s *SystemResourceService) collectCapacity(ctx context.Context) bool {
	s.capacityMu.Lock()
	defer s.capacityMu.Unlock()
	started := time.Now()
	s.markTaskStarted("capacity", started)
	capacityCtx, cancel := context.WithTimeout(ctx, capacityCollectionTimeout)
	defer cancel()
	snapshot, err := s.collector.CollectCapacity(capacityCtx)
	remoteDatabaseAvailable := false
	if err == nil {
		platformDatabaseAvailable := false
		snapshot.DatabaseLogicalBytes, snapshot.Databases, platformDatabaseAvailable = s.repo.CollectDatabaseSizes(capacityCtx)
		snapshot.DatabaseSizeAvailable = platformDatabaseAvailable
		if remote, ok := s.requestDatabaseStats(subjects.PlatformDatabaseStatsQuerySubject); ok {
			remoteDatabaseAvailable = true
			if remote.Available {
				snapshot.DatabaseSizeAvailable = true
				// Physical discovery can see workspace schemas on the same MySQL instance.
				// The runtime registry has authoritative ownership; count each schema once.
				snapshot.Databases = mergeDatabaseInventory(snapshot.Databases, remote.Databases)
				snapshot.DatabaseLogicalBytes = 0
				for _, database := range snapshot.Databases {
					snapshot.DatabaseLogicalBytes += database.UsedBytes
				}
				sort.SliceStable(snapshot.Databases, func(i, j int) bool {
					if snapshot.Databases[i].Kind != snapshot.Databases[j].Kind {
						return snapshot.Databases[i].Kind == "platform"
					}
					if snapshot.Databases[i].UsedBytes != snapshot.Databases[j].UsedBytes {
						return snapshot.Databases[i].UsedBytes > snapshot.Databases[j].UsedBytes
					}
					return snapshot.Databases[i].Name < snapshot.Databases[j].Name
				})
			}
		}
		snapshot.DatabaseInventoryComplete = platformDatabaseAvailable && remoteDatabaseAvailable
		for _, database := range snapshot.Databases {
			if database.Status == "missing" {
				snapshot.DatabaseSizeAvailable = false
			}
		}
	}
	if err == nil {
		err = s.repo.CreateCapacity(snapshot)
	}
	now := time.Now()
	cleanupErr := s.repo.PruneHistory(now.Add(-runtimeRetention), now.Add(-platformRetention), now.Add(-capacityRetention))
	if err == nil {
		err = cleanupErr
	}
	next := nextDailyRun(time.Now(), capacityRunHour, capacityRunMinute)
	if err != nil {
		s.markTaskFinished("capacity", started, err, next)
		logger.Warnf(ctx, "[SystemResourceMonitor] capacity collection failed: %v", err)
		return false
	}
	s.mu.Lock()
	s.lastCapacity = &snapshot
	s.mu.Unlock()
	if !snapshot.DatabaseInventoryComplete {
		s.markTaskPartial("capacity", started, "database metric source is unavailable", time.Now().Add(capacityRetryInterval))
		return false
	}
	s.markTaskFinished("capacity", started, nil, next)
	return true
}

func (s *SystemResourceService) Overview(historyHours int, includeHistory bool) (*dto.SystemResourceOverviewResp, error) {
	if historyHours < 24 {
		historyHours = 24
	}
	if historyHours > 24*30 {
		historyHours = 24 * 30
	}
	current, platform, tasks, err := s.currentState()
	if err != nil {
		return nil, err
	}
	history := []dto.SystemResourceHistoryPoint{}
	capacityHistory := []dto.SystemCapacityDailyPoint{}
	if includeHistory {
		samples, historyErr := s.repo.History(time.Now().Add(-time.Duration(historyHours)*time.Hour), historyHours*6+12)
		if historyErr != nil {
			return nil, fmt.Errorf("load resource history: %w", historyErr)
		}
		history = make([]dto.SystemResourceHistoryPoint, 0, len(samples)+1)
		for _, sample := range samples {
			history = append(history, historyPoint(sample))
		}
		if len(history) == 0 || current.CollectedAt.Sub(history[len(history)-1].CollectedAt) > runtimePersistInterval {
			history = append(history, dto.SystemResourceHistoryPoint{CollectedAt: current.CollectedAt, DiskUsedBytes: current.DiskUsedBytes, DiskUsedPercent: current.DiskUsedPercent, MemoryUsedPercent: current.MemoryUsedPercent, CPUUsedPercent: current.CPUUsedPercent, CPUMaxPercent: current.CPUUsedPercent, NetworkRxBytesPS: current.NetworkRxBytesPS, NetworkTxBytesPS: current.NetworkTxBytesPS, DiskReadBytesPS: current.DiskReadBytesPS, DiskWriteBytesPS: current.DiskWriteBytesPS, Load1: current.Load1})
		}
		capacitySnapshots, capacityErr := s.repo.CapacityHistory(time.Now().Add(-31*24*time.Hour), 40)
		if capacityErr != nil {
			return nil, fmt.Errorf("load capacity history: %w", capacityErr)
		}
		capacityHistory = buildCapacityDailyHistory(capacitySnapshots)
	}
	return &dto.SystemResourceOverviewResp{
		Current: *current, History: history, CapacityHistory: capacityHistory, HistoryHours: historyHours,
		SampleIntervalMinutes: int(runtimePersistInterval / time.Minute), RuntimeIntervalSeconds: int(runtimeSampleInterval / time.Second),
		RuntimeRetentionDays: int(runtimeRetention / (24 * time.Hour)), PlatformRetentionDays: int(platformRetention / (24 * time.Hour)),
		CapacityRetentionDays: int(capacityRetention / (24 * time.Hour)), PlatformIntervalHours: 24, CapacityIntervalHours: 24,
		PlatformScheduleLocal: fmt.Sprintf("%02d:%02d", platformRunHour, platformRunMinute), CapacityScheduleLocal: fmt.Sprintf("%02d:%02d", capacityRunHour, capacityRunMinute),
		CapacityCollectedAt: s.capacityCollectedAt(), Forecast: buildStorageForecast(*current, history), Platform: platform, CollectionTasks: tasks,
	}, nil
}

func (s *SystemResourceService) Summary() (*dto.SystemResourceSummaryResp, error) {
	current, platform, _, err := s.currentState()
	if err != nil {
		return nil, err
	}
	// Keep the live response deliberately small. These daily capacity fields are
	// available from the storage and database endpoints when their tabs open.
	current.StoragePools = nil
	current.Components = nil
	current.Databases = nil
	current.LargestDatabases = nil
	return &dto.SystemResourceSummaryResp{
		Current:                *current,
		Platform:               platform,
		Forecast:               buildStorageForecast(*current, nil),
		SampleIntervalMinutes:  int(runtimePersistInterval / time.Minute),
		RuntimeRetentionDays:   int(runtimeRetention / (24 * time.Hour)),
		RuntimeIntervalSeconds: int(runtimeSampleInterval / time.Second),
	}, nil
}

func (s *SystemResourceService) Trends(historyHours int) (*dto.SystemResourceTrendsResp, error) {
	historyHours = normalizedHistoryHours(historyHours)
	current, _, _, err := s.currentState()
	if err != nil {
		return nil, err
	}
	history, err := s.loadRuntimeHistory(current, historyHours)
	if err != nil {
		return nil, err
	}
	return &dto.SystemResourceTrendsResp{
		History:               history,
		HistoryHours:          historyHours,
		SampleIntervalMinutes: int(runtimePersistInterval / time.Minute),
		RuntimeRetentionDays:  int(runtimeRetention / (24 * time.Hour)),
	}, nil
}

func (s *SystemResourceService) Storage() (*dto.SystemResourceStorageResp, error) {
	current, _, _, err := s.currentState()
	if err != nil {
		return nil, err
	}
	capacitySnapshots, err := s.repo.CapacityHistory(time.Now().Add(-31*24*time.Hour), 40)
	if err != nil {
		return nil, fmt.Errorf("load capacity history: %w", err)
	}
	forecastHistory := make([]dto.SystemResourceHistoryPoint, 0, len(capacitySnapshots))
	for _, snapshot := range capacitySnapshots {
		forecastHistory = append(forecastHistory, dto.SystemResourceHistoryPoint{
			CollectedAt:     snapshot.CollectedAt,
			DiskUsedBytes:   snapshot.DiskUsedBytes,
			DiskUsedPercent: snapshot.DiskUsedPercent,
		})
	}
	return &dto.SystemResourceStorageResp{
		CollectedAt:           s.capacityCollectedAt(),
		Environment:           current.Environment,
		StoragePools:          current.StoragePools,
		Components:            current.Components,
		Forecast:              buildStorageForecast(*current, forecastHistory),
		CapacityRetentionDays: int(capacityRetention / (24 * time.Hour)),
		CapacityScheduleLocal: fmt.Sprintf("%02d:%02d", capacityRunHour, capacityRunMinute),
	}, nil
}

func (s *SystemResourceService) Databases(page, pageSize int, scope, keyword string, includeHistory bool, historyOptions ...DatabaseHistoryOptions) (*dto.SystemResourceDatabaseListResp, error) {
	current, _, _, err := s.currentState()
	if err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	scope = strings.ToLower(strings.TrimSpace(scope))
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if scope != "platform" && scope != "workspace" && scope != "unmanaged" {
		scope = "all"
	}
	databases := current.Databases
	if len(databases) == 0 {
		databases = current.LargestDatabases
	}
	items, total, platformCount, workspaceCount := filterAndPaginateDatabases(databases, page, pageSize, scope, keyword)
	options := DatabaseHistoryOptions{Days: 7}
	if len(historyOptions) > 0 {
		options = historyOptions[0]
	}
	if options.Days != 30 {
		options.Days = 7
	}
	capacityHistory := []dto.SystemCapacityDailyPoint(nil)
	historyDatabases := []dto.SystemDatabaseSize{}
	if includeHistory {
		start, _ := localCapacityDay(time.Now().AddDate(0, 0, -options.Days))
		snapshots, historyErr := s.repo.CapacityHistory(start, options.Days+2)
		if historyErr != nil {
			return nil, fmt.Errorf("load capacity history: %w", historyErr)
		}
		seen := map[string]bool{}
		for _, snapshot := range append(snapshots, *current) {
			for _, database := range snapshot.Databases {
				if (scope == "all" || database.Kind == scope) && !seen[database.Key()] {
					seen[database.Key()] = true
					historyDatabases = append(historyDatabases, dto.SystemDatabaseSize{Name: database.Name, Kind: database.Kind, SourceID: database.SourceID})
				}
			}
		}
		sort.Slice(historyDatabases, func(i, j int) bool { return historyDatabases[i].Name < historyDatabases[j].Name })
		capacityHistory = scopedCapacityHistory(snapshots, scope, options, time.Now())
	}
	return &dto.SystemResourceDatabaseListResp{
		Items:                     items,
		Total:                     total,
		Page:                      page,
		PageSize:                  pageSize,
		PlatformCount:             platformCount,
		WorkspaceCount:            workspaceCount,
		DatabaseLogicalBytes:      current.DatabaseLogicalBytes,
		DatabaseSizeAvailable:     current.DatabaseSizeAvailable,
		DatabaseInventoryComplete: current.DatabaseInventoryComplete,
		CollectedAt:               s.capacityCollectedAt(),
		CapacityHistory:           capacityHistory,
		HistoryDatabases:          historyDatabases,
		CapacityRetentionDays:     int(capacityRetention / (24 * time.Hour)),
		CapacityScheduleLocal:     fmt.Sprintf("%02d:%02d", capacityRunHour, capacityRunMinute),
	}, nil
}

func (s *SystemResourceService) Diagnostics() (*dto.SystemResourceDiagnosticsResp, error) {
	current, _, tasks, err := s.currentState()
	if err != nil {
		return nil, err
	}
	return &dto.SystemResourceDiagnosticsResp{
		CollectedAt:            current.CollectedAt,
		Environment:            current.Environment,
		CollectionTasks:        tasks,
		PlatformRetentionDays:  int(platformRetention / (24 * time.Hour)),
		CapacityRetentionDays:  int(capacityRetention / (24 * time.Hour)),
		PlatformScheduleLocal:  fmt.Sprintf("%02d:%02d", platformRunHour, platformRunMinute),
		CapacityScheduleLocal:  fmt.Sprintf("%02d:%02d", capacityRunHour, capacityRunMinute),
		SampleIntervalMinutes:  int(runtimePersistInterval / time.Minute),
		RuntimeRetentionDays:   int(runtimeRetention / (24 * time.Hour)),
		RuntimeIntervalSeconds: int(runtimeSampleInterval / time.Second),
	}, nil
}

func (s *SystemResourceService) Usage(periodDays, rankingPage, rankingPageSize int) (*dto.SystemUsageOverviewResp, error) {
	if periodDays != 30 {
		periodDays = 7
	}
	rankingPage, rankingPageSize = normalizeUsageRankingPage(rankingPage, rankingPageSize)
	current := dto.SystemUsageSnapshot{}
	if stats, ok := s.requestPlatformStats(subjects.PlatformAppStatsQuerySubject); ok && stats.Usage.Available {
		current = stats.Usage
	} else {
		_, platform, _, err := s.currentState()
		if err != nil {
			return nil, err
		}
		current = platform.Usage
	}
	if !current.Available {
		return &dto.SystemUsageOverviewResp{PeriodDays: periodDays, RankingBasis: "cumulative", RankingPage: rankingPage, RankingPageSize: rankingPageSize, SnapshotScheduleLocal: fmt.Sprintf("%02d:%02d", platformRunHour, platformRunMinute)}, nil
	}

	now := time.Now()
	cutoff := now.AddDate(0, 0, -periodDays)
	history, err := s.repo.PlatformHistory(cutoff.AddDate(0, 0, -2), periodDays+5)
	if err != nil {
		return nil, fmt.Errorf("load usage history: %w", err)
	}
	baseline, hasBaseline := usageBaseline(history, cutoff)
	baselineCalls := make(map[string]int64, len(baseline.Functions))
	if hasBaseline {
		for _, function := range baseline.Functions {
			baselineCalls[function.Path] = function.TotalCalls
		}
	}

	functions := make([]dto.SystemFunctionUsageItem, 0, len(current.Functions))
	directoryMap := make(map[string]*dto.SystemDirectoryUsageItem)
	var successfulCalls int64
	for _, function := range current.Functions {
		periodCalls := function.TotalCalls
		if hasBaseline {
			periodCalls = max(int64(0), function.TotalCalls-baselineCalls[function.Path])
		}
		item := dto.SystemFunctionUsageItem{SystemFunctionUsageSnapshot: function, PeriodCalls: periodCalls}
		functions = append(functions, item)
		successfulCalls += periodCalls
		directory := directoryMap[function.DirectoryPath]
		if directory == nil {
			name := function.DirectoryName
			if strings.TrimSpace(name) == "" {
				name = pathDisplayName(function.DirectoryPath)
			}
			directory = &dto.SystemDirectoryUsageItem{Path: function.DirectoryPath, Name: name}
			directoryMap[function.DirectoryPath] = directory
		}
		directory.FunctionCount++
		directory.TotalCalls += function.TotalCalls
		directory.PeriodCalls += periodCalls
	}
	sort.SliceStable(functions, func(i, j int) bool {
		if functions[i].PeriodCalls != functions[j].PeriodCalls {
			return functions[i].PeriodCalls > functions[j].PeriodCalls
		}
		if functions[i].TotalCalls != functions[j].TotalCalls {
			return functions[i].TotalCalls > functions[j].TotalCalls
		}
		return functions[i].Path < functions[j].Path
	})
	directories := make([]dto.SystemDirectoryUsageItem, 0, len(directoryMap))
	for _, item := range directoryMap {
		directories = append(directories, *item)
	}
	sort.SliceStable(directories, func(i, j int) bool {
		if directories[i].PeriodCalls != directories[j].PeriodCalls {
			return directories[i].PeriodCalls > directories[j].PeriodCalls
		}
		if directories[i].TotalCalls != directories[j].TotalCalls {
			return directories[i].TotalCalls > directories[j].TotalCalls
		}
		return directories[i].Path < directories[j].Path
	})

	operationsPeriod, failedOperations := current.OperationsLast7Days, current.FailedOperationsLast7Days
	if periodDays == 30 {
		operationsPeriod, failedOperations = current.OperationsLast30Days, current.FailedOperationsLast30Days
	}
	return &dto.SystemUsageOverviewResp{
		Available: true, CollectedAt: current.CollectedAt, PeriodDays: periodDays,
		RankingBasis:    map[bool]string{true: "period", false: "cumulative"}[hasBaseline],
		OperationsToday: current.OperationsToday, OperationsPeriod: operationsPeriod, FailedOperations: failedOperations,
		SuccessfulCalls: successfulCalls,
		TopDirectories:  paginateDirectories(directories, rankingPage, rankingPageSize),
		TopFunctions:    paginateFunctions(functions, rankingPage, rankingPageSize),
		DirectoryTotal:  len(directories), FunctionTotal: len(functions), RankingPage: rankingPage, RankingPageSize: rankingPageSize,
		DailyHistory: buildUsageDailyHistory(history, current), SnapshotScheduleLocal: fmt.Sprintf("%02d:%02d", platformRunHour, platformRunMinute),
	}, nil
}

func usageBaseline(history []dto.SystemPlatformMetrics, cutoff time.Time) (dto.SystemUsageSnapshot, bool) {
	var result dto.SystemUsageSnapshot
	found := false
	for _, metrics := range history {
		if !metrics.Usage.Available || metrics.CollectedAt.After(cutoff) {
			continue
		}
		if !found || metrics.CollectedAt.After(result.CollectedAt) {
			result, found = metrics.Usage, true
		}
	}
	return result, found
}

func buildUsageDailyHistory(history []dto.SystemPlatformMetrics, current dto.SystemUsageSnapshot) []dto.SystemUsageDailyPoint {
	byDate := make(map[string]dto.SystemUsageDailyPoint)
	for _, metrics := range history {
		usage := metrics.Usage
		if !usage.Available || usage.OperationDay == "" {
			continue
		}
		byDate[usage.OperationDay] = dto.SystemUsageDailyPoint{Date: usage.OperationDay, Operations: usage.OperationsYesterday, Failed: usage.FailedOperationsYesterday}
	}
	today := current.CollectedAt.In(time.Local).Format("2006-01-02")
	byDate[today] = dto.SystemUsageDailyPoint{Date: today, Operations: current.OperationsToday, Failed: current.FailedOperationsToday}
	result := make([]dto.SystemUsageDailyPoint, 0, len(byDate))
	for _, point := range byDate {
		result = append(result, point)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Date < result[j].Date })
	if len(result) > 30 {
		result = result[len(result)-30:]
	}
	return result
}

func pathDisplayName(path string) string {
	path = strings.TrimRight(path, "/")
	if index := strings.LastIndex(path, "/"); index >= 0 && index+1 < len(path) {
		return path[index+1:]
	}
	return path
}

func normalizeUsageRankingPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 5 {
		pageSize = 10
	}
	if pageSize > 50 {
		pageSize = 50
	}
	return page, pageSize
}

func paginateDirectories(items []dto.SystemDirectoryUsageItem, page, pageSize int) []dto.SystemDirectoryUsageItem {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []dto.SystemDirectoryUsageItem{}
	}
	end := min(len(items), start+pageSize)
	return items[start:end]
}

func paginateFunctions(items []dto.SystemFunctionUsageItem, page, pageSize int) []dto.SystemFunctionUsageItem {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []dto.SystemFunctionUsageItem{}
	}
	end := min(len(items), start+pageSize)
	return items[start:end]
}

func normalizedHistoryHours(historyHours int) int {
	if historyHours < 24 {
		return 24
	}
	if historyHours > 24*30 {
		return 24 * 30
	}
	return historyHours
}

func (s *SystemResourceService) loadRuntimeHistory(current *dto.SystemResourceSnapshot, historyHours int) ([]dto.SystemResourceHistoryPoint, error) {
	samples, err := s.repo.History(time.Now().Add(-time.Duration(historyHours)*time.Hour), historyHours*6+12)
	if err != nil {
		return nil, fmt.Errorf("load resource history: %w", err)
	}
	history := make([]dto.SystemResourceHistoryPoint, 0, len(samples)+1)
	for _, sample := range samples {
		history = append(history, historyPoint(sample))
	}
	if len(history) == 0 || current.CollectedAt.Sub(history[len(history)-1].CollectedAt) > runtimePersistInterval {
		history = append(history, dto.SystemResourceHistoryPoint{CollectedAt: current.CollectedAt, DiskUsedBytes: current.DiskUsedBytes, DiskUsedPercent: current.DiskUsedPercent, MemoryUsedPercent: current.MemoryUsedPercent, CPUUsedPercent: current.CPUUsedPercent, CPUMaxPercent: current.CPUUsedPercent, NetworkRxBytesPS: current.NetworkRxBytesPS, NetworkTxBytesPS: current.NetworkTxBytesPS, DiskReadBytesPS: current.DiskReadBytesPS, DiskWriteBytesPS: current.DiskWriteBytesPS, Load1: current.Load1})
	}
	return history, nil
}

func databaseMatchesKeyword(database dto.SystemDatabaseSize, keyword string) bool {
	return strings.Contains(strings.ToLower(database.Name), keyword) ||
		strings.Contains(strings.ToLower(database.Owner), keyword) ||
		strings.Contains(strings.ToLower(database.Directory), keyword) ||
		strings.Contains(strings.ToLower(database.Purpose), keyword) ||
		strings.Contains(strings.ToLower(database.Status), keyword)
}

func filterAndPaginateDatabases(databases []dto.SystemDatabaseSize, page, pageSize int, scope, keyword string) ([]dto.SystemDatabaseSize, int, int, int) {
	filtered := make([]dto.SystemDatabaseSize, 0, len(databases))
	platformCount, workspaceCount := 0, 0
	for _, database := range databases {
		switch database.Kind {
		case "platform":
			platformCount++
		case "workspace":
			workspaceCount++
		}
		if scope != "all" && database.Kind != scope {
			continue
		}
		if keyword != "" && !databaseMatchesKeyword(database, keyword) {
			continue
		}
		filtered = append(filtered, database)
	}
	start := min((page-1)*pageSize, len(filtered))
	end := min(start+pageSize, len(filtered))
	return append([]dto.SystemDatabaseSize(nil), filtered[start:end]...), len(filtered), platformCount, workspaceCount
}

func tailCapacityDailyHistory(history []dto.SystemCapacityDailyPoint, limit int) []dto.SystemCapacityDailyPoint {
	if limit <= 0 || len(history) <= limit {
		return history
	}
	return append([]dto.SystemCapacityDailyPoint(nil), history[len(history)-limit:]...)
}

func (s *SystemResourceService) currentState() (*dto.SystemResourceSnapshot, dto.SystemPlatformMetrics, []dto.SystemCollectionTaskStatus, error) {
	s.mu.Lock()
	runtimeSnapshot, capacitySnapshot, platform := s.lastRuntime, s.lastCapacity, s.lastPlatform
	s.mu.Unlock()
	if runtimeSnapshot == nil {
		value, err := s.collector.CollectRuntime()
		if err != nil {
			return nil, dto.SystemPlatformMetrics{}, nil, fmt.Errorf("collect runtime resources: %w", err)
		}
		runtimeSnapshot = &value
	}
	if capacitySnapshot == nil {
		if stored, err := s.repo.LatestCapacity(); err == nil {
			capacitySnapshot = stored
			s.mu.Lock()
			if s.lastCapacity == nil {
				s.lastCapacity = stored
			}
			s.mu.Unlock()
		} else {
			// Capacity scanning can recursively walk large directories. Never make
			// an HTTP request wait for it; the background task will replace this
			// lightweight filesystem snapshot when its first scan completes.
			capacitySnapshot = runtimeSnapshot
		}
	}
	if platform == nil {
		if stored, err := s.repo.LatestPlatform(); err == nil {
			platform = stored
			s.mu.Lock()
			if s.lastPlatform == nil {
				s.lastPlatform = stored
			}
			s.mu.Unlock()
		} else if value, collectErr := s.repo.CollectPlatformMetrics(time.Now()); collectErr == nil {
			platform = &value
		} else {
			return nil, dto.SystemPlatformMetrics{}, nil, fmt.Errorf("collect platform metrics: %w", collectErr)
		}
	}
	current := *runtimeSnapshot
	mergeCapacitySnapshot(&current, *capacitySnapshot)
	return &current, *platform, s.taskStatuses(), nil
}

func (s *SystemResourceService) requestPlatformStats(subject string) (dto.SystemPlatformServiceStats, bool) {
	if s.natsConn == nil {
		return dto.SystemPlatformServiceStats{}, false
	}
	message, err := s.natsConn.Request(subject, nil, 2*time.Second)
	if err != nil {
		return dto.SystemPlatformServiceStats{}, false
	}
	response, ok := decodePlatformStats(message.Data)
	if !ok {
		return dto.SystemPlatformServiceStats{}, false
	}
	return response, true
}

func (s *SystemResourceService) requestDatabaseStats(subject string) (dto.SystemDatabaseCapacityStats, bool) {
	if s.natsConn == nil {
		return dto.SystemDatabaseCapacityStats{}, false
	}
	message, err := s.natsConn.Request(subject, nil, 15*time.Second)
	if err != nil {
		return dto.SystemDatabaseCapacityStats{}, false
	}
	return decodeDatabaseStats(message.Data)
}

func decodePlatformStats(data []byte) (dto.SystemPlatformServiceStats, bool) {
	var envelope struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Error != "" {
		return dto.SystemPlatformServiceStats{}, false
	}
	var response dto.SystemPlatformServiceStats
	if json.Unmarshal(data, &response) != nil {
		return dto.SystemPlatformServiceStats{}, false
	}
	return response, true
}

func decodeDatabaseStats(data []byte) (dto.SystemDatabaseCapacityStats, bool) {
	var envelope struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Error != "" {
		return dto.SystemDatabaseCapacityStats{}, false
	}
	var response dto.SystemDatabaseCapacityStats
	if json.Unmarshal(data, &response) != nil {
		return dto.SystemDatabaseCapacityStats{}, false
	}
	// During mixed-version rollout, an unidentified remote schema cannot be
	// safely deduplicated against physical discovery. Report partial coverage.
	for _, database := range response.Databases {
		if response.Available && database.SourceID == "" {
			return dto.SystemDatabaseCapacityStats{}, false
		}
	}
	return response, true
}

func (s *SystemResourceService) markTaskStarted(key string, started time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.tasks[key]
	status.Status, status.LastStartedAt, status.Error = "running", timePointer(started), ""
	s.tasks[key] = status
}
func (s *SystemResourceService) markTaskFinished(key string, started time.Time, err error, next time.Time) {
	finished := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.tasks[key]
	status.DurationMillis, status.NextRunAt = finished.Sub(started).Milliseconds(), timePointer(next)
	if err != nil {
		status.Status, status.Error = "failed", err.Error()
	} else {
		status.Status, status.Error, status.LastSucceededAt = "success", "", timePointer(finished)
	}
	s.tasks[key] = status
}

func (s *SystemResourceService) markTaskPartial(key string, started time.Time, message string, next time.Time) {
	finished := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.tasks[key]
	status.Status, status.Error = "partial", message
	status.DurationMillis, status.NextRunAt, status.LastSucceededAt = finished.Sub(started).Milliseconds(), timePointer(next), timePointer(finished)
	s.tasks[key] = status
}
func (s *SystemResourceService) setTaskNextRun(key string, next time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.tasks[key]
	status.NextRunAt = timePointer(next)
	s.tasks[key] = status
}
func (s *SystemResourceService) restoreTask(key string, collectedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.tasks[key]
	status.Status, status.LastSucceededAt = "success", timePointer(collectedAt)
	s.tasks[key] = status
}
func (s *SystemResourceService) platformCollectedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastPlatform == nil {
		return time.Time{}
	}
	return s.lastPlatform.CollectedAt
}
func (s *SystemResourceService) capacityCollectedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastCapacity == nil {
		return time.Time{}
	}
	return s.lastCapacity.CollectedAt
}
func (s *SystemResourceService) capacityCollectionDue(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastCapacity == nil ||
		s.lastCapacity.CapacitySchemaVersion < systemCapacitySchemaVersion ||
		!s.lastCapacity.DatabaseInventoryComplete {
		return true
	}
	return shouldRunScheduledDaily(s.lastCapacity.CollectedAt, now, capacityRunHour, capacityRunMinute)
}
func (s *SystemResourceService) taskStatuses() []dto.SystemCollectionTaskStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]dto.SystemCollectionTaskStatus, 0, 3)
	for _, key := range []string{"runtime", "platform", "capacity"} {
		result = append(result, s.tasks[key])
	}
	return result
}
func timePointer(value time.Time) *time.Time { copy := value.UTC(); return &copy }
func nextDailyRun(now time.Time, hour, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
func scheduledRunAtOrBefore(now time.Time, hour, minute int) time.Time {
	scheduled := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if scheduled.After(now) {
		scheduled = scheduled.AddDate(0, 0, -1)
	}
	return scheduled
}
func shouldRunScheduledDaily(last, now time.Time, hour, minute int) bool {
	return last.IsZero() || last.Before(scheduledRunAtOrBefore(now, hour, minute))
}
func waitUntil(ctx context.Context, at time.Time) bool {
	timer := time.NewTimer(time.Until(at))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
func waitFor(ctx context.Context, duration time.Duration) bool {
	return waitUntil(ctx, time.Now().Add(duration))
}

type runtimeAccumulator struct {
	count                                                         int
	last                                                          dto.SystemResourceSnapshot
	cpuSum, cpuMax, netRxSum, netTxSum, diskReadSum, diskWriteSum float64
}

func (a *runtimeAccumulator) Add(snapshot dto.SystemResourceSnapshot) {
	a.count++
	a.last = snapshot
	a.cpuSum += snapshot.CPUUsedPercent
	a.netRxSum += snapshot.NetworkRxBytesPS
	a.netTxSum += snapshot.NetworkTxBytesPS
	a.diskReadSum += snapshot.DiskReadBytesPS
	a.diskWriteSum += snapshot.DiskWriteBytesPS
	if snapshot.CPUUsedPercent > a.cpuMax {
		a.cpuMax = snapshot.CPUUsedPercent
	}
}
func (a *runtimeAccumulator) Sample() model.SystemResourceSample {
	count := float64(a.count)
	if count == 0 {
		count = 1
	}
	return model.SystemResourceSample{CollectedAt: a.last.CollectedAt, DiskTotalBytes: a.last.DiskTotalBytes, DiskUsedBytes: a.last.DiskUsedBytes, DiskFreeBytes: a.last.DiskFreeBytes, DiskUsedPercent: a.last.DiskUsedPercent, MemoryTotalBytes: a.last.MemoryTotalBytes, MemoryUsedBytes: a.last.MemoryUsedBytes, MemoryUsedPercent: a.last.MemoryUsedPercent, CPUUsedPercent: a.cpuSum / count, CPUMaxPercent: a.cpuMax, NetworkRxBytesPS: a.netRxSum / count, NetworkTxBytesPS: a.netTxSum / count, DiskReadBytesPS: a.diskReadSum / count, DiskWriteBytesPS: a.diskWriteSum / count, Load1: a.last.Load1}
}
func (a *runtimeAccumulator) Reset() { *a = runtimeAccumulator{} }

func historyPoint(sample model.SystemResourceSample) dto.SystemResourceHistoryPoint {
	return dto.SystemResourceHistoryPoint{CollectedAt: sample.CollectedAt, DiskUsedBytes: sample.DiskUsedBytes, DiskUsedPercent: sample.DiskUsedPercent, MemoryUsedPercent: sample.MemoryUsedPercent, CPUUsedPercent: sample.CPUUsedPercent, CPUMaxPercent: sample.CPUMaxPercent, NetworkRxBytesPS: sample.NetworkRxBytesPS, NetworkTxBytesPS: sample.NetworkTxBytesPS, DiskReadBytesPS: sample.DiskReadBytesPS, DiskWriteBytesPS: sample.DiskWriteBytesPS, Load1: sample.Load1}
}

func buildCapacityDailyHistory(snapshots []dto.SystemResourceSnapshot) []dto.SystemCapacityDailyPoint {
	result := make([]dto.SystemCapacityDailyPoint, 0, len(snapshots))
	indexByDay := make(map[string]int, len(snapshots))
	versionByDay := make(map[string]int, len(snapshots))
	for _, snapshot := range snapshots {
		day := snapshot.CollectedAt.In(time.Local).Format("2006-01-02")
		versionByDay[day] = snapshot.CapacitySchemaVersion
		platformCount, workspaceCount := 0, 0
		for _, database := range snapshot.Databases {
			if database.Kind == "platform" {
				platformCount++
			} else if database.Kind == "workspace" {
				workspaceCount++
			}
		}
		point := dto.SystemCapacityDailyPoint{
			Date:        day,
			CollectedAt: snapshot.CollectedAt, DatabaseLogicalBytes: snapshot.DatabaseLogicalBytes,
			DatabaseCount: len(snapshot.Databases), PlatformDatabaseCount: platformCount, WorkspaceDatabaseCount: workspaceCount,
			DatabaseSizeAvailable:  snapshot.DatabaseSizeAvailable && snapshot.DatabaseInventoryComplete,
			DatabaseCountAvailable: snapshot.DatabaseInventoryComplete,
		}
		if index, exists := indexByDay[day]; exists {
			result[index] = point
			continue
		}
		indexByDay[day] = len(result)
		result = append(result, point)
	}
	for index := 1; index < len(result); index++ {
		current, previous := &result[index], result[index-1]
		previousDay, _ := localCapacityDay(previous.CollectedAt)
		currentDay, _ := localCapacityDay(current.CollectedAt)
		if !previousDay.AddDate(0, 0, 1).Equal(currentDay) || versionByDay[previous.Date] != versionByDay[current.Date] {
			continue
		}
		if current.DatabaseSizeAvailable && previous.DatabaseSizeAvailable {
			current.PreviousCollectedAt = &previous.CollectedAt
			current.DatabaseLogicalDelta = signedByteDelta(current.DatabaseLogicalBytes, previous.DatabaseLogicalBytes)
			current.DatabaseLogicalDeltaAvailable = true
		}
		if current.DatabaseCountAvailable && previous.DatabaseCountAvailable {
			current.DatabaseCountDelta = current.DatabaseCount - previous.DatabaseCount
			current.DatabaseCountDeltaAvailable = true
		}
	}
	return result
}

func signedByteDelta(current, previous uint64) int64 {
	if current >= previous {
		return int64(current - previous)
	}
	return -int64(previous - current)
}

func buildStorageForecast(current dto.SystemResourceSnapshot, history []dto.SystemResourceHistoryPoint) dto.StorageExpansionForecast {
	poolKey, currentPercent := "primary", current.DiskUsedPercent
	for _, pool := range current.StoragePools {
		if pool.Available && pool.UsedPercent > currentPercent {
			poolKey, currentPercent = pool.Key, pool.UsedPercent
		}
	}
	forecast := dto.StorageExpansionForecast{Status: "healthy", PoolKey: poolKey, CurrentUsedPercent: currentPercent, TargetPercent: 85, Message: "Storage capacity is healthy"}
	if currentPercent >= 90 {
		forecast.Status, forecast.Message = "critical", "Storage usage is critical; expand capacity now"
		return forecast
	} else if currentPercent >= 80 {
		forecast.Status, forecast.Message = "warning", "Storage usage is high; prepare capacity expansion"
		return forecast
	}
	if len(history) < 2 || current.DiskTotalBytes == 0 {
		return forecast
	}
	first, last := history[0], history[len(history)-1]
	days := last.CollectedAt.Sub(first.CollectedAt).Hours() / 24
	if days < 1 || last.DiskUsedBytes <= first.DiskUsedBytes {
		return forecast
	}
	forecast.DailyGrowthByte = float64(last.DiskUsedBytes-first.DiskUsedBytes) / days
	targetBytes := float64(current.DiskTotalBytes) * forecast.TargetPercent / 100
	remaining := targetBytes - float64(current.DiskUsedBytes)
	if remaining <= 0 {
		zero := 0
		forecast.DaysToTarget = &zero
		return forecast
	}
	daysToTarget := int(math.Ceil(remaining / forecast.DailyGrowthByte))
	forecast.DaysToTarget = &daysToTarget
	if daysToTarget <= 30 && forecast.Status == "healthy" {
		forecast.Status, forecast.Message = "warning", "Storage is projected to reach the expansion threshold within 30 days"
	}
	return forecast
}

// DatabaseHistoryOptions selects the calendar window independently of list pagination.
type DatabaseHistoryOptions struct {
	Days int
	Name string
}

func localCapacityDay(at time.Time) (time.Time, time.Time) {
	at = at.In(time.Local)
	start := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location())
	return start, start.AddDate(0, 0, 1)
}

func scopedCapacityHistory(snapshots []dto.SystemResourceSnapshot, scope string, options DatabaseHistoryOptions, now time.Time) []dto.SystemCapacityDailyPoint {
	filtered := make([]dto.SystemResourceSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		selected := snapshot
		if scope == "unmanaged" && snapshot.CapacitySchemaVersion < 3 {
			selected.DatabaseInventoryComplete = false
		}
		selected.Databases = nil
		selected.DatabaseLogicalBytes = 0
		for _, database := range snapshot.Databases {
			if scope != "all" && database.Kind != scope || options.Name != "" && database.Key() != options.Name {
				continue
			}
			selected.Databases = append(selected.Databases, database)
			if database.Status == "missing" {
				selected.DatabaseSizeAvailable = false
			}
			selected.DatabaseLogicalBytes += database.UsedBytes
		}
		if options.Name != "" && len(selected.Databases) == 0 {
			selected.DatabaseSizeAvailable = false
			selected.DatabaseInventoryComplete = false
		}
		filtered = append(filtered, selected)
	}
	points := buildCapacityDailyHistory(filtered)
	byDay := map[string]dto.SystemCapacityDailyPoint{}
	for _, point := range points {
		byDay[point.CollectedAt.In(time.Local).Format("2006-01-02")] = point
	}
	start, _ := localCapacityDay(now.AddDate(0, 0, 1-options.Days))
	result := make([]dto.SystemCapacityDailyPoint, 0, options.Days)
	for index := 0; index < options.Days; index++ {
		day := start.AddDate(0, 0, index)
		point, ok := byDay[day.Format("2006-01-02")]
		if !ok {
			point.CollectedAt = day
			point.Date = day.Format("2006-01-02")
		}
		result = append(result, point)
	}
	return result
}

func mergeDatabaseInventory(local, remote []dto.SystemDatabaseSize) []dto.SystemDatabaseSize {
	remoteKeys := make(map[string]bool, len(remote))
	for _, database := range remote {
		remoteKeys[database.Key()] = true
	}
	result := make([]dto.SystemDatabaseSize, 0, len(local)+len(remote))
	for _, database := range local {
		if !remoteKeys[database.Key()] {
			result = append(result, database)
		}
	}
	return append(result, remote...)
}
