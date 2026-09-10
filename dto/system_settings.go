package dto

import "time"

type SystemBackupConfig struct {
	Enabled            bool   `json:"enabled"`
	ScheduleTime       string `json:"schedule_time"`
	Endpoint           string `json:"endpoint"`
	Region             string `json:"region"`
	Bucket             string `json:"bucket"`
	Prefix             string `json:"prefix"`
	AccessKeyID        string `json:"access_key_id"`
	SecretAccessKey    string `json:"secret_access_key,omitempty"`
	SecretAccessKeySet bool   `json:"secret_access_key_set"`
	UseSSL             bool   `json:"use_ssl"`
	ForcePathStyle     bool   `json:"force_path_style"`
	KeepLocal          int    `json:"keep_local"`
	RetentionDays      int    `json:"retention_days"`
}

type SystemBackupRecord struct {
	ID           string `json:"id"`
	TriggeredBy  string `json:"triggered_by"`
	Status       string `json:"status"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at,omitempty"`
	ArchiveName  string `json:"archive_name,omitempty"`
	SizeBytes    int64  `json:"size_bytes,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
	Bucket       string `json:"bucket,omitempty"`
	ObjectKey    string `json:"object_key,omitempty"`
	ETag         string `json:"etag,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type SystemBackupOverview struct {
	Config          SystemBackupConfig   `json:"config"`
	AgentAvailable  bool                 `json:"agent_available"`
	AgentLastSeenAt string               `json:"agent_last_seen_at,omitempty"`
	Running         bool                 `json:"running"`
	Records         []SystemBackupRecord `json:"records"`
}

type EmailSettings struct {
	Mode        string `json:"mode"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	PasswordSet bool   `json:"password_set"`
	From        string `json:"from"`
	FromName    string `json:"from_name"`
}

type SystemSettingsResp struct {
	RegistrationMode string        `json:"registration_mode"`
	Email            EmailSettings `json:"email"`
}

type LoginAnnouncement struct {
	Enabled  bool   `json:"enabled"`
	Markdown string `json:"markdown"`
}

type UpdateLoginAnnouncementReq struct {
	Enabled  bool   `json:"enabled"`
	Markdown string `json:"markdown"`
}

type TLSCertificateInfo struct {
	Subject      string   `json:"subject"`
	Issuer       string   `json:"issuer"`
	DNSNames     []string `json:"dns_names"`
	IPAddresses  []string `json:"ip_addresses"`
	NotBefore    string   `json:"not_before"`
	NotAfter     string   `json:"not_after"`
	IsSelfSigned bool     `json:"is_self_signed"`
}

type TLSSettingsResp struct {
	Mode            string              `json:"mode"`
	BaseURL         string              `json:"base_url"`
	CertFile        string              `json:"cert_file"`
	KeyFile         string              `json:"key_file"`
	CertExists      bool                `json:"cert_exists"`
	KeyExists       bool                `json:"key_exists"`
	Ready           bool                `json:"ready"`
	Writable        bool                `json:"writable"`
	ReloadSupported bool                `json:"reload_supported"`
	Certificate     *TLSCertificateInfo `json:"certificate,omitempty"`
	Message         string              `json:"message,omitempty"`
}

type UpdateTLSCertificateReq struct {
	CertificatePEM string `json:"certificate_pem" binding:"required"`
	PrivateKeyPEM  string `json:"private_key_pem" binding:"required"`
	Reload         bool   `json:"reload"`
}

type UpdateSystemSettingsReq struct {
	RegistrationMode string        `json:"registration_mode" binding:"required,oneof=admin_only email_code debug_code"`
	Email            EmailSettings `json:"email"`
}

type TestEmailReq struct {
	To string `json:"to" binding:"required,email"`
}

type SystemResourceComponent struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	PoolKey   string `json:"pool_key"`
	UsedBytes uint64 `json:"used_bytes"`
	Available bool   `json:"available"`
}

type SystemEnvironmentInfo struct {
	Mode              string `json:"mode"`
	Deployment        string `json:"deployment"`
	Containerized     bool   `json:"containerized"`
	ContainerEngine   string `json:"container_engine"`
	ContainerRemote   bool   `json:"container_remote"`
	StorageRootSource string `json:"storage_root_source"`
}

type SystemStoragePool struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Path        string  `json:"path,omitempty"`
	TotalBytes  uint64  `json:"total_bytes"`
	UsedBytes   uint64  `json:"used_bytes"`
	FreeBytes   uint64  `json:"free_bytes"`
	UsedPercent float64 `json:"used_percent"`
	Primary     bool    `json:"primary"`
	Available   bool    `json:"available"`
}

type SystemResourceSnapshot struct {
	CapacitySchemaVersion     int                       `json:"capacity_schema_version,omitempty"`
	CollectedAt               time.Time                 `json:"collected_at"`
	Hostname                  string                    `json:"hostname"`
	OperatingSystem           string                    `json:"operating_system"`
	Architecture              string                    `json:"architecture"`
	CPUCores                  int                       `json:"cpu_cores"`
	CPUUsedPercent            float64                   `json:"cpu_used_percent"`
	CPUAvailable              bool                      `json:"cpu_available"`
	Load1                     float64                   `json:"load_1"`
	Load5                     float64                   `json:"load_5"`
	Load15                    float64                   `json:"load_15"`
	LoadAvailable             bool                      `json:"load_available"`
	UptimeSeconds             uint64                    `json:"uptime_seconds"`
	MemoryTotalBytes          uint64                    `json:"memory_total_bytes"`
	MemoryUsedBytes           uint64                    `json:"memory_used_bytes"`
	MemoryUsedPercent         float64                   `json:"memory_used_percent"`
	MemoryAvailable           bool                      `json:"memory_available"`
	SwapTotalBytes            uint64                    `json:"swap_total_bytes"`
	SwapUsedBytes             uint64                    `json:"swap_used_bytes"`
	SwapUsedPercent           float64                   `json:"swap_used_percent"`
	NetworkRxBytes            uint64                    `json:"network_rx_bytes"`
	NetworkTxBytes            uint64                    `json:"network_tx_bytes"`
	NetworkAvailable          bool                      `json:"network_available"`
	NetworkRxBytesPS          float64                   `json:"network_rx_bytes_per_second"`
	NetworkTxBytesPS          float64                   `json:"network_tx_bytes_per_second"`
	DiskReadBytes             uint64                    `json:"disk_read_bytes"`
	DiskWriteBytes            uint64                    `json:"disk_write_bytes"`
	DiskIOAvailable           bool                      `json:"disk_io_available"`
	DiskReadBytesPS           float64                   `json:"disk_read_bytes_per_second"`
	DiskWriteBytesPS          float64                   `json:"disk_write_bytes_per_second"`
	DiskMount                 string                    `json:"disk_mount"`
	DiskTotalBytes            uint64                    `json:"disk_total_bytes"`
	DiskUsedBytes             uint64                    `json:"disk_used_bytes"`
	DiskFreeBytes             uint64                    `json:"disk_free_bytes"`
	DiskUsedPercent           float64                   `json:"disk_used_percent"`
	Environment               SystemEnvironmentInfo     `json:"environment"`
	StoragePools              []SystemStoragePool       `json:"storage_pools"`
	Components                []SystemResourceComponent `json:"components"`
	DatabaseLogicalBytes      uint64                    `json:"database_logical_bytes"`
	DatabaseSizeAvailable     bool                      `json:"database_size_available"`
	DatabaseInventoryComplete bool                      `json:"database_inventory_complete"`
	Databases                 []SystemDatabaseSize      `json:"databases"`
	// LargestDatabases is kept for snapshots created before the complete
	// database inventory was introduced. New collections leave it empty.
	LargestDatabases []SystemDatabaseSize `json:"largest_databases"`
}

type SystemResourceHistoryPoint struct {
	CollectedAt       time.Time `json:"collected_at"`
	DiskUsedBytes     uint64    `json:"disk_used_bytes"`
	DiskUsedPercent   float64   `json:"disk_used_percent"`
	MemoryUsedPercent float64   `json:"memory_used_percent"`
	CPUUsedPercent    float64   `json:"cpu_used_percent"`
	CPUMaxPercent     float64   `json:"cpu_max_percent"`
	NetworkRxBytesPS  float64   `json:"network_rx_bytes_per_second"`
	NetworkTxBytesPS  float64   `json:"network_tx_bytes_per_second"`
	DiskReadBytesPS   float64   `json:"disk_read_bytes_per_second"`
	DiskWriteBytesPS  float64   `json:"disk_write_bytes_per_second"`
	Load1             float64   `json:"load_1"`
}

type SystemDatabaseSize struct {
	SourceID  string `json:"source_id,omitempty"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Owner     string `json:"owner"`
	Directory string `json:"directory"`
	Purpose   string `json:"purpose"`
	Status    string `json:"status"`
	UsedBytes uint64 `json:"used_bytes"`
}

// Key distinguishes same-named schemas on separate configured MySQL instances.
func (database SystemDatabaseSize) Key() string {
	if database.SourceID == "" {
		return database.Name
	}
	return database.SourceID + "/" + database.Name
}

type SystemPlatformServiceStats struct {
	WorkspacesTotal      int64               `json:"workspaces_total"`
	WorkspacesEnabled    int64               `json:"workspaces_enabled"`
	ServiceDirectories   int64               `json:"service_directories"`
	FunctionsTotal       int64               `json:"functions_total"`
	AppDatabasesTotal    int64               `json:"app_databases_total"`
	ScheduledTasksTotal  int64               `json:"scheduled_tasks_total"`
	ScheduledTasksActive int64               `json:"scheduled_tasks_active"`
	Usage                SystemUsageSnapshot `json:"usage"`
}

// SystemFunctionUsageSnapshot is the cumulative successful invocation count
// already maintained by service_tree. Daily platform snapshots persist these
// compact counters so period deltas do not require request-level metric rows.
type SystemFunctionUsageSnapshot struct {
	Path          string `json:"path"`
	Name          string `json:"name"`
	DirectoryPath string `json:"directory_path"`
	DirectoryName string `json:"directory_name"`
	TemplateType  string `json:"template_type"`
	TotalCalls    int64  `json:"total_calls"`
}

type SystemUsageSnapshot struct {
	Available                  bool                          `json:"available"`
	CollectedAt                time.Time                     `json:"collected_at"`
	OperationDay               string                        `json:"operation_day"`
	OperationsToday            int64                         `json:"operations_today"`
	OperationsYesterday        int64                         `json:"operations_yesterday"`
	OperationsLast7Days        int64                         `json:"operations_last_7_days"`
	OperationsLast30Days       int64                         `json:"operations_last_30_days"`
	FailedOperationsToday      int64                         `json:"failed_operations_today"`
	FailedOperationsYesterday  int64                         `json:"failed_operations_yesterday"`
	FailedOperationsLast7Days  int64                         `json:"failed_operations_last_7_days"`
	FailedOperationsLast30Days int64                         `json:"failed_operations_last_30_days"`
	Functions                  []SystemFunctionUsageSnapshot `json:"functions"`
}

type SystemFunctionUsageItem struct {
	SystemFunctionUsageSnapshot
	PeriodCalls int64 `json:"period_calls"`
}

type SystemDirectoryUsageItem struct {
	Path          string `json:"path"`
	Name          string `json:"name"`
	FunctionCount int    `json:"function_count"`
	TotalCalls    int64  `json:"total_calls"`
	PeriodCalls   int64  `json:"period_calls"`
}

type SystemUsageDailyPoint struct {
	Date       string `json:"date"`
	Operations int64  `json:"operations"`
	Failed     int64  `json:"failed"`
}

type SystemUsageOverviewResp struct {
	Available             bool                       `json:"available"`
	CollectedAt           time.Time                  `json:"collected_at"`
	PeriodDays            int                        `json:"period_days"`
	RankingBasis          string                     `json:"ranking_basis"`
	OperationsToday       int64                      `json:"operations_today"`
	OperationsPeriod      int64                      `json:"operations_period"`
	FailedOperations      int64                      `json:"failed_operations"`
	SuccessfulCalls       int64                      `json:"successful_calls"`
	TopDirectories        []SystemDirectoryUsageItem `json:"top_directories"`
	TopFunctions          []SystemFunctionUsageItem  `json:"top_functions"`
	DirectoryTotal        int                        `json:"directory_total"`
	FunctionTotal         int                        `json:"function_total"`
	RankingPage           int                        `json:"ranking_page"`
	RankingPageSize       int                        `json:"ranking_page_size"`
	DailyHistory          []SystemUsageDailyPoint    `json:"daily_history"`
	SnapshotScheduleLocal string                     `json:"snapshot_schedule_local"`
}

type SystemDatabaseCapacityStats struct {
	Error      string               `json:"error,omitempty"`
	Available  bool                 `json:"available"`
	TotalBytes uint64               `json:"total_bytes"`
	Databases  []SystemDatabaseSize `json:"databases"`
}

type SystemPlatformMetrics struct {
	CollectedAt           time.Time           `json:"collected_at"`
	UsersTotal            int64               `json:"users_total"`
	UsersActive           int64               `json:"users_active"`
	UsersPending          int64               `json:"users_pending"`
	WorkspacesTotal       int64               `json:"workspaces_total"`
	WorkspacesEnabled     int64               `json:"workspaces_enabled"`
	ServiceDirectories    int64               `json:"service_directories"`
	FunctionsTotal        int64               `json:"functions_total"`
	AppDatabasesTotal     int64               `json:"app_databases_total"`
	ScheduledTasksTotal   int64               `json:"scheduled_tasks_total"`
	ScheduledTasksActive  int64               `json:"scheduled_tasks_active"`
	AppStatsAvailable     bool                `json:"app_stats_available"`
	RuntimeStatsAvailable bool                `json:"runtime_stats_available"`
	TimerStatsAvailable   bool                `json:"timer_stats_available"`
	Usage                 SystemUsageSnapshot `json:"usage"`
}

type SystemCollectionTaskStatus struct {
	Key             string     `json:"key"`
	Status          string     `json:"status"`
	LastStartedAt   *time.Time `json:"last_started_at,omitempty"`
	LastSucceededAt *time.Time `json:"last_succeeded_at,omitempty"`
	NextRunAt       *time.Time `json:"next_run_at,omitempty"`
	DurationMillis  int64      `json:"duration_millis"`
	Error           string     `json:"error,omitempty"`
}

type StorageExpansionForecast struct {
	Status             string  `json:"status"`
	PoolKey            string  `json:"pool_key"`
	CurrentUsedPercent float64 `json:"current_used_percent"`
	DailyGrowthByte    float64 `json:"daily_growth_bytes"`
	TargetPercent      float64 `json:"target_percent"`
	DaysToTarget       *int    `json:"days_to_target,omitempty"`
	Message            string  `json:"message"`
}

type SystemCapacityDailyPoint struct {
	PreviousCollectedAt           *time.Time `json:"previous_collected_at,omitempty"`
	Date                          string     `json:"date"`
	CollectedAt                   time.Time  `json:"collected_at"`
	DatabaseLogicalBytes          uint64     `json:"database_logical_bytes"`
	DatabaseLogicalDelta          int64      `json:"database_logical_delta"`
	DatabaseLogicalDeltaAvailable bool       `json:"database_logical_delta_available"`
	DatabaseCount                 int        `json:"database_count"`
	DatabaseCountDelta            int        `json:"database_count_delta"`
	DatabaseCountDeltaAvailable   bool       `json:"database_count_delta_available"`
	PlatformDatabaseCount         int        `json:"platform_database_count"`
	WorkspaceDatabaseCount        int        `json:"workspace_database_count"`
	DatabaseSizeAvailable         bool       `json:"database_size_available"`
	DatabaseCountAvailable        bool       `json:"database_count_available"`
}

type SystemResourceOverviewResp struct {
	Current                SystemResourceSnapshot       `json:"current"`
	History                []SystemResourceHistoryPoint `json:"history"`
	CapacityHistory        []SystemCapacityDailyPoint   `json:"capacity_history"`
	HistoryHours           int                          `json:"history_hours"`
	SampleIntervalMinutes  int                          `json:"sample_interval_minutes"`
	RuntimeRetentionDays   int                          `json:"runtime_retention_days"`
	PlatformRetentionDays  int                          `json:"platform_retention_days"`
	CapacityRetentionDays  int                          `json:"capacity_retention_days"`
	PlatformIntervalHours  int                          `json:"platform_interval_hours"`
	CapacityIntervalHours  int                          `json:"capacity_interval_hours"`
	PlatformScheduleLocal  string                       `json:"platform_schedule_local"`
	CapacityScheduleLocal  string                       `json:"capacity_schedule_local"`
	CapacityCollectedAt    time.Time                    `json:"capacity_collected_at"`
	Forecast               StorageExpansionForecast     `json:"forecast"`
	Platform               SystemPlatformMetrics        `json:"platform"`
	CollectionTasks        []SystemCollectionTaskStatus `json:"collection_tasks"`
	RuntimeIntervalSeconds int                          `json:"runtime_interval_seconds"`
}

// SystemResourceSummaryResp is the small, frequently refreshed monitoring
// payload. Capacity inventories and historical series intentionally live in
// separate endpoints so a live dashboard does not retransmit daily data.
type SystemResourceSummaryResp struct {
	Current                SystemResourceSnapshot   `json:"current"`
	Platform               SystemPlatformMetrics    `json:"platform"`
	Forecast               StorageExpansionForecast `json:"forecast"`
	SampleIntervalMinutes  int                      `json:"sample_interval_minutes"`
	RuntimeRetentionDays   int                      `json:"runtime_retention_days"`
	RuntimeIntervalSeconds int                      `json:"runtime_interval_seconds"`
}

type SystemResourceTrendsResp struct {
	History               []SystemResourceHistoryPoint `json:"history"`
	HistoryHours          int                          `json:"history_hours"`
	SampleIntervalMinutes int                          `json:"sample_interval_minutes"`
	RuntimeRetentionDays  int                          `json:"runtime_retention_days"`
}

type SystemResourceStorageResp struct {
	CollectedAt           time.Time                 `json:"collected_at"`
	Environment           SystemEnvironmentInfo     `json:"environment"`
	StoragePools          []SystemStoragePool       `json:"storage_pools"`
	Components            []SystemResourceComponent `json:"components"`
	Forecast              StorageExpansionForecast  `json:"forecast"`
	CapacityRetentionDays int                       `json:"capacity_retention_days"`
	CapacityScheduleLocal string                    `json:"capacity_schedule_local"`
}

type SystemResourceDatabaseListResp struct {
	HistoryDatabases          []SystemDatabaseSize       `json:"history_databases,omitempty"`
	Items                     []SystemDatabaseSize       `json:"items"`
	Total                     int                        `json:"total"`
	Page                      int                        `json:"page"`
	PageSize                  int                        `json:"page_size"`
	PlatformCount             int                        `json:"platform_count"`
	WorkspaceCount            int                        `json:"workspace_count"`
	DatabaseLogicalBytes      uint64                     `json:"database_logical_bytes"`
	DatabaseSizeAvailable     bool                       `json:"database_size_available"`
	DatabaseInventoryComplete bool                       `json:"database_inventory_complete"`
	CollectedAt               time.Time                  `json:"collected_at"`
	CapacityHistory           []SystemCapacityDailyPoint `json:"capacity_history,omitempty"`
	CapacityRetentionDays     int                        `json:"capacity_retention_days"`
	CapacityScheduleLocal     string                     `json:"capacity_schedule_local"`
}

type SystemResourceDiagnosticsResp struct {
	CollectedAt            time.Time                    `json:"collected_at"`
	Environment            SystemEnvironmentInfo        `json:"environment"`
	CollectionTasks        []SystemCollectionTaskStatus `json:"collection_tasks"`
	PlatformRetentionDays  int                          `json:"platform_retention_days"`
	CapacityRetentionDays  int                          `json:"capacity_retention_days"`
	PlatformScheduleLocal  string                       `json:"platform_schedule_local"`
	CapacityScheduleLocal  string                       `json:"capacity_schedule_local"`
	SampleIntervalMinutes  int                          `json:"sample_interval_minutes"`
	RuntimeRetentionDays   int                          `json:"runtime_retention_days"`
	RuntimeIntervalSeconds int                          `json:"runtime_interval_seconds"`
}
