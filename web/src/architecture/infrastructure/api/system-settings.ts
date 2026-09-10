import { get, put, post } from '@/architecture/infrastructure/apiClient/request'

export type RegistrationMode = 'admin_only' | 'email_code' | 'debug_code'
export type EmailMode = 'smtp' | 'log'

export interface EmailSettings {
  mode: EmailMode
  host: string
  port: number
  username: string
  password?: string
  password_set: boolean
  from: string
  from_name: string
}

export interface SystemSettings {
  registration_mode: RegistrationMode
  email: EmailSettings
}

export interface SystemBackupConfig {
  enabled: boolean
  schedule_time: string
  endpoint: string
  region: string
  bucket: string
  prefix: string
  access_key_id: string
  secret_access_key?: string
  secret_access_key_set: boolean
  use_ssl: boolean
  force_path_style: boolean
  keep_local: number
  retention_days: number
}

export interface SystemBackupRecord {
  id: string
  triggered_by: string
  status: string
  started_at: string
  finished_at?: string
  archive_name?: string
  size_bytes?: number
  sha256?: string
  bucket?: string
  object_key?: string
  etag?: string
  error_message?: string
}

export interface SystemBackupOverview {
  config: SystemBackupConfig
  agent_available: boolean
  agent_last_seen_at?: string
  running: boolean
  records: SystemBackupRecord[]
}

export interface TLSCertificateInfo {
  subject: string
  issuer: string
  dns_names: string[]
  ip_addresses: string[]
  not_before: string
  not_after: string
  is_self_signed: boolean
}

export interface TLSSettings {
  mode: string
  base_url: string
  cert_file: string
  key_file: string
  cert_exists: boolean
  key_exists: boolean
  ready: boolean
  writable: boolean
  reload_supported: boolean
  certificate?: TLSCertificateInfo
  message?: string
}

export interface UpdateTLSCertificatePayload {
  certificate_pem: string
  private_key_pem: string
  reload: boolean
}

export interface AuthLoginProviderField {
  key: string
  label: string
  type: string
  required: boolean
  secret: boolean
  help?: string
  placeholder?: string
  value?: string
  value_set: boolean
}

export interface AuthLoginProviderInfo {
  code: string
  name: string
  description: string
  action: string
  enabled: boolean
  configured: boolean
  status: 'unconfigured' | 'disabled' | 'enabled' | string
  callback_path?: string
  docs_url?: string
  fields: AuthLoginProviderField[]
  updated_by?: string
  updated_at?: string
}

export interface ListAuthLoginProvidersResp {
  providers: AuthLoginProviderInfo[]
}

export interface LoginMethodInfo {
  provider: string
  label: string
  action: string
  description?: string
  authorize_path?: string
}

export interface ListLoginMethodsResp {
  methods: LoginMethodInfo[]
}

export interface LoginAnnouncement {
  enabled: boolean
  markdown: string
}

export interface LogArchiveResourceSummary {
  resource_path: string
  count: number
}

export interface LogArchiveBatch {
  attempts?: number
  next_retry_at?: string
  id: number
  archive_key: string
  archive_type: string
  tenant_user: string
  app: string
  range_started_at: string
  range_ended_at: string
  record_count: number
  object_ref: string
  file_name: string
  file_size: number
  sha256: string
  status: 'exporting' | 'uploaded' | 'completed' | 'failed' | string
  summary_json?: { top_resource_paths?: LogArchiveResourceSummary[] }
  error_message?: string
  archived_at?: string
  deleted_at_source?: string
  created_at: string
}

export interface ListLogArchiveBatchesResp {
  list: LogArchiveBatch[]
  total: number
  retention_days: number
  cron_expr: string
  timezone: string
}

export interface SystemResourceComponent {
  key: string
  name: string
  pool_key: string
  used_bytes: number
  available: boolean
}

export interface SystemEnvironmentInfo {
  mode: 'development' | 'production' | string
  deployment: 'source' | 'aio' | 'container' | 'host' | string
  containerized: boolean
  container_engine: string
  container_remote: boolean
  storage_root_source: string
}

export interface SystemStoragePool {
  key: string
  name: string
  path?: string
  total_bytes: number
  used_bytes: number
  free_bytes: number
  used_percent: number
  primary: boolean
  available: boolean
}

export interface SystemResourceSnapshot {
  capacity_schema_version?: number
  collected_at: string
  hostname: string
  operating_system: string
  architecture: string
  cpu_cores: number
  cpu_used_percent: number
  cpu_available: boolean
  load_1: number
  load_5: number
  load_15: number
  load_available: boolean
  uptime_seconds: number
  memory_total_bytes: number
  memory_used_bytes: number
  memory_used_percent: number
  memory_available: boolean
  swap_total_bytes: number
  swap_used_bytes: number
  swap_used_percent: number
  network_rx_bytes: number
  network_tx_bytes: number
  network_available: boolean
  network_rx_bytes_per_second: number
  network_tx_bytes_per_second: number
  disk_read_bytes: number
  disk_write_bytes: number
  disk_io_available: boolean
  disk_read_bytes_per_second: number
  disk_write_bytes_per_second: number
  disk_mount: string
  disk_total_bytes: number
  disk_used_bytes: number
  disk_free_bytes: number
  disk_used_percent: number
  environment: SystemEnvironmentInfo
  storage_pools: SystemStoragePool[]
  components: SystemResourceComponent[]
  database_logical_bytes: number
  database_size_available: boolean
  database_inventory_complete: boolean
  databases: SystemDatabaseSize[]
  largest_databases: SystemDatabaseSize[]
}

export interface SystemResourceHistoryPoint {
  collected_at: string
  disk_used_bytes: number
  disk_used_percent: number
  memory_used_percent: number
  cpu_used_percent: number
  cpu_max_percent: number
  network_rx_bytes_per_second: number
  network_tx_bytes_per_second: number
  disk_read_bytes_per_second: number
  disk_write_bytes_per_second: number
  load_1: number
}

export interface SystemDatabaseSize {
  source_id?: string
  name: string
  kind: 'platform' | 'workspace' | string
  owner: string
  directory: string
  purpose: string
  status: 'active' | 'pending' | 'missing' | 'orphaned' | string
  used_bytes: number
}

export interface SystemPlatformMetrics {
  collected_at: string
  users_total: number
  users_active: number
  users_pending: number
  workspaces_total: number
  workspaces_enabled: number
  service_directories: number
  functions_total: number
  app_databases_total: number
  scheduled_tasks_total: number
  scheduled_tasks_active: number
  app_stats_available: boolean
  runtime_stats_available: boolean
  timer_stats_available: boolean
}

export interface SystemCollectionTaskStatus {
  key: 'runtime' | 'platform' | 'capacity' | string
  status: 'pending' | 'running' | 'success' | 'partial' | 'failed' | string
  last_started_at?: string
  last_succeeded_at?: string
  next_run_at?: string
  duration_millis: number
  error?: string
}

export interface StorageExpansionForecast {
  status: 'healthy' | 'warning' | 'critical' | string
  pool_key: string
  current_used_percent: number
  daily_growth_bytes: number
  target_percent: number
  days_to_target?: number
  message: string
}

export interface SystemCapacityDailyPoint {
  previous_collected_at?: string
  date?: string
  collected_at: string
  database_logical_bytes: number
  database_logical_delta: number
  database_logical_delta_available: boolean
  database_count: number
  database_count_delta: number
  database_count_delta_available: boolean
  platform_database_count: number
  workspace_database_count: number
  database_size_available: boolean
  database_count_available: boolean
}

export interface SystemResourceOverview {
  current: SystemResourceSnapshot
  history: SystemResourceHistoryPoint[]
  capacity_history: SystemCapacityDailyPoint[]
  history_hours: number
  sample_interval_minutes: number
  runtime_retention_days: number
  platform_retention_days: number
  capacity_retention_days: number
  platform_interval_hours: number
  capacity_interval_hours: number
  platform_schedule_local: string
  capacity_schedule_local: string
  capacity_collected_at: string
  forecast: StorageExpansionForecast
  platform: SystemPlatformMetrics
  collection_tasks: SystemCollectionTaskStatus[]
  runtime_interval_seconds: number
}

export interface SystemResourceSummary {
  current: SystemResourceSnapshot
  platform: SystemPlatformMetrics
  forecast: StorageExpansionForecast
  sample_interval_minutes: number
  runtime_retention_days: number
  runtime_interval_seconds: number
}

export interface SystemResourceTrends {
  history: SystemResourceHistoryPoint[]
  history_hours: number
  sample_interval_minutes: number
  runtime_retention_days: number
}

export interface SystemResourceStorage {
  collected_at: string
  environment: SystemEnvironmentInfo
  storage_pools: SystemStoragePool[]
  components: SystemResourceComponent[]
  forecast: StorageExpansionForecast
  capacity_retention_days: number
  capacity_schedule_local: string
}

export interface SystemResourceDatabaseList {
  history_databases?: SystemDatabaseSize[]
  items: SystemDatabaseSize[]
  total: number
  page: number
  page_size: number
  platform_count: number
  workspace_count: number
  database_logical_bytes: number
  database_size_available: boolean
  database_inventory_complete: boolean
  collected_at: string
  capacity_history: SystemCapacityDailyPoint[]
  capacity_retention_days: number
  capacity_schedule_local: string
}

export interface SystemResourceDiagnostics {
  collected_at: string
  environment: SystemEnvironmentInfo
  collection_tasks: SystemCollectionTaskStatus[]
  platform_retention_days: number
  capacity_retention_days: number
  platform_schedule_local: string
  capacity_schedule_local: string
  sample_interval_minutes: number
  runtime_retention_days: number
  runtime_interval_seconds: number
}

export interface SystemFunctionUsageItem {
  path: string
  name: string
  directory_path: string
  directory_name: string
  template_type: string
  total_calls: number
  period_calls: number
}

export interface SystemDirectoryUsageItem {
  path: string
  name: string
  function_count: number
  total_calls: number
  period_calls: number
}

export interface SystemUsageDailyPoint {
  date: string
  operations: number
  failed: number
}

export interface SystemUsageOverview {
  available: boolean
  collected_at: string
  period_days: number
  ranking_basis: 'period' | 'cumulative'
  operations_today: number
  operations_period: number
  failed_operations: number
  successful_calls: number
  top_directories: SystemDirectoryUsageItem[]
  top_functions: SystemFunctionUsageItem[]
  directory_total: number
  function_total: number
  ranking_page: number
  ranking_page_size: number
  daily_history: SystemUsageDailyPoint[]
  snapshot_schedule_local: string
}

export function getSystemSettings() {
  return get<SystemSettings>('/hr/api/v1/system/settings')
}

export function updateSystemSettings(data: SystemSettings) {
  return put<SystemSettings>('/hr/api/v1/system/settings', data)
}

export function testSystemEmail(to: string) {
  return post('/hr/api/v1/system/settings/email/test', { to })
}

export function getSystemBackupOverview() {
  return get<SystemBackupOverview>('/hr/api/v1/system/settings/backup')
}

export function updateSystemBackupConfig(data: SystemBackupConfig) {
  return put<SystemBackupOverview>('/hr/api/v1/system/settings/backup', data)
}

export function testSystemBackupS3(data: SystemBackupConfig) {
  return post('/hr/api/v1/system/settings/backup/test', data)
}

export function runSystemBackupNow() {
  return post<SystemBackupOverview>('/hr/api/v1/system/settings/backup/run')
}

export function getTLSSettings() {
  return get<TLSSettings>('/hr/api/v1/system/settings/tls')
}

export function updateTLSCertificate(data: UpdateTLSCertificatePayload) {
  return put<TLSSettings>('/hr/api/v1/system/settings/tls', data)
}

export function reloadTLSCertificate() {
  return post<TLSSettings>('/hr/api/v1/system/settings/tls/reload')
}

export function listAuthLoginProviders() {
  return get<ListAuthLoginProvidersResp>('/hr/api/v1/system/auth/providers')
}

export function getLoginAnnouncementConfig() {
  return get<LoginAnnouncement>('/hr/api/v1/system/auth/login-announcement')
}

export function updateLoginAnnouncementConfig(data: LoginAnnouncement) {
  return put<LoginAnnouncement>('/hr/api/v1/system/auth/login-announcement', data)
}

export function updateAuthLoginProviderConfig(code: string, config: Record<string, string>) {
  return put<AuthLoginProviderInfo>(`/hr/api/v1/system/auth/providers/${encodeURIComponent(code)}/config`, {
    config
  })
}

export function updateAuthLoginProviderEnabled(code: string, enabled: boolean) {
  return put<AuthLoginProviderInfo>(`/hr/api/v1/system/auth/providers/${encodeURIComponent(code)}/enabled`, {
    enabled
  })
}

export function listLoginMethods() {
  return get<ListLoginMethodsResp>('/hr/api/v1/auth/methods')
}

export function retryLogArchiveBatch(id: number) {
  return post<{ id: number }>(`/workspace/api/v1/system/log_archives/${id}/retry`, {}, { timeout: 600000 })
}

export function listLogArchiveBatches(page = 1, pageSize = 20) {
  return get<ListLogArchiveBatchesResp>('/workspace/api/v1/system/log_archives', { page, page_size: pageSize })
}

export function getSystemResourceOverview(hours = 24 * 7, includeHistory = true) {
  return get<SystemResourceOverview>('/hr/api/v1/system/settings/resources', {
    hours,
    include_history: includeHistory
  })
}

export function getSystemResourceSummary() {
  return get<SystemResourceSummary>('/hr/api/v1/system/settings/resources/summary')
}

export function getSystemResourceTrends(hours = 24 * 7) {
  return get<SystemResourceTrends>('/hr/api/v1/system/settings/resources/trends', { hours })
}

export function getSystemResourceStorage() {
  return get<SystemResourceStorage>('/hr/api/v1/system/settings/resources/storage')
}

export function getSystemResourceDatabases(params: { page?: number; page_size?: number; scope?: string; keyword?: string; include_history?: boolean; days?: number; database?: string } = {}) {
  return get<SystemResourceDatabaseList>('/hr/api/v1/system/settings/resources/databases', params)
}

export function getSystemResourceDiagnostics() {
  return get<SystemResourceDiagnostics>('/hr/api/v1/system/settings/resources/diagnostics')
}

export function getSystemResourceUsage(days = 7, page = 1, pageSize = 10) {
  return get<SystemUsageOverview>('/hr/api/v1/system/settings/resources/usage', { days, page, page_size: pageSize })
}
