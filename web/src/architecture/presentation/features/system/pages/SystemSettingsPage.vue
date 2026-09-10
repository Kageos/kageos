<template>
  <div class="system-settings-page">
    <el-card shadow="never" class="settings-card">
      <div class="settings-layout">
        <aside class="settings-sidebar" :aria-label="t('systemSettings.categoryLabel')">
          <div class="settings-sidebar-heading">
            <el-icon class="settings-sidebar-mark"><Setting /></el-icon>
            <div>
              <h2>{{ t('route.systemSettings') }}</h2>
              <p>{{ t('systemSettings.subtitle') }}</p>
            </div>
          </div>

          <nav v-for="group in settingsNavigationGroups" :key="group.key" class="settings-nav-group" :aria-label="group.title">
            <p class="settings-nav-group-title">{{ group.title }}</p>
            <div class="settings-nav-list">
              <button
                v-for="section in group.sections"
                :key="section.key"
                type="button"
                class="settings-nav-item"
                :class="{ 'is-active': activeTab === section.key }"
                :aria-current="activeTab === section.key ? 'page' : undefined"
                @click="selectSettingsSection(section.key)"
              >
                <el-icon class="settings-nav-icon"><component :is="section.icon" /></el-icon>
                <span class="settings-nav-title">{{ section.title }}</span>
              </button>
            </div>
          </nav>
        </aside>

        <section class="settings-content">
          <div class="settings-mobile-heading">
            <div>
              <h2>{{ t('route.systemSettings') }}</h2>
              <p>{{ t('systemSettings.subtitle') }}</p>
            </div>
            <label class="settings-mobile-select">
              <span>{{ t('systemSettings.categoryLabel') }}</span>
              <select :value="activeTab" @change="handleMobileSettingsChange">
                <optgroup v-for="group in settingsNavigationGroups" :key="group.key" :label="group.title">
                  <option v-for="section in group.sections" :key="section.key" :value="section.key">
                    {{ section.title }}
                  </option>
                </optgroup>
              </select>
            </label>
          </div>

          <div class="section-header">
            <div>
              <h3>{{ currentSection.title }}</h3>
              <p>{{ currentSection.desc }}</p>
            </div>
            <div class="header-actions">
              <el-button :icon="Refresh" @click="refreshActiveTab">{{ t('common.refresh') }}</el-button>
              <el-button v-if="activeTab === 'email'" type="primary" :icon="Check" :loading="saving" @click="saveSettings">
                {{ t('connectorProvider.save') }}
              </el-button>
              <el-button :icon="QuestionFilled" @click="openCurrentDocs">
                {{ t('systemSettings.viewDocs') }}
              </el-button>
            </div>
          </div>

          <div v-if="activeTab === 'operations'" v-loading="resourcesLoading" class="section-pane operations-pane">
            <template v-if="resourceOverview">
              <el-tabs v-model="operationsTab" class="operations-tabs" @tab-change="handleOperationsTabChange">
                <el-tab-pane :label="t('systemSettings.resources.tabs.overview')" name="overview" />
                <el-tab-pane :label="t('systemSettings.resources.tabs.usage')" name="usage" />
                <el-tab-pane :label="t('systemSettings.resources.tabs.trends')" name="trends" />
                <el-tab-pane :label="t('systemSettings.resources.tabs.storage')" name="storage" />
                <el-tab-pane :label="t('systemSettings.resources.tabs.databases')" name="databases" />
                <el-tab-pane :label="t('systemSettings.resources.tabs.diagnostics')" name="diagnostics" />
              </el-tabs>
              <el-alert
                v-if="operationsTab === 'overview' || operationsTab === 'storage'"
                :title="forecastTitle"
                :description="forecastDescription"
                :type="forecastAlertType"
                show-icon
                :closable="false"
              />
              <el-alert
                v-if="operationsTab === 'diagnostics'"
                :title="environmentTitle"
                :description="environmentDescription"
                type="info"
                show-icon
                :closable="false"
              />

              <div v-if="operationsTab === 'overview'" class="resource-summary-grid">
                <article class="resource-summary-card">
                  <span class="resource-summary-label">{{ t('systemSettings.resources.disk') }}</span>
                  <strong>{{ formatBytes(resourceOverview.current.disk_used_bytes) }} / {{ formatBytes(resourceOverview.current.disk_total_bytes) }}</strong>
                  <el-progress :percentage="roundedPercent(resourceOverview.current.disk_used_percent)" :status="progressStatus(resourceOverview.current.disk_used_percent)" />
                  <small>{{ t('systemSettings.resources.diskFree', { value: formatBytes(resourceOverview.current.disk_free_bytes) }) }}</small>
                </article>
                <article class="resource-summary-card">
                  <span class="resource-summary-label">{{ t('systemSettings.resources.memory') }}</span>
                  <strong v-if="resourceOverview.current.memory_available">
                    {{ formatBytes(resourceOverview.current.memory_used_bytes) }} / {{ formatBytes(resourceOverview.current.memory_total_bytes) }}
                  </strong>
                  <strong v-else>{{ t('systemSettings.resources.unavailable') }}</strong>
                  <el-progress v-if="resourceOverview.current.memory_available" :percentage="roundedPercent(resourceOverview.current.memory_used_percent)" />
                  <small>{{ t('systemSettings.resources.cpuCores', { count: resourceOverview.current.cpu_cores }) }}</small>
                </article>
                <article class="resource-summary-card">
                  <span class="resource-summary-label">{{ t('systemSettings.resources.cpuAndLoad') }}</span>
                  <div v-if="resourceOverview.current.cpu_available" class="resource-value-row">
                    <strong>{{ t('systemSettings.resources.cpuUsageValue', { value: roundedPercent(resourceOverview.current.cpu_used_percent) }) }}</strong>
                    <el-tag v-if="resourceOverview.current.load_available" size="small" effect="plain" :type="loadStatusType">
                      {{ loadStatusLabel }}
                    </el-tag>
                  </div>
                  <strong v-else>{{ t('systemSettings.resources.unavailable') }}</strong>
                  <small
                    v-if="resourceOverview.current.load_available"
                    :title="t('systemSettings.resources.loadCurrent', { one: resourceOverview.current.load_1.toFixed(2), five: resourceOverview.current.load_5.toFixed(2), fifteen: resourceOverview.current.load_15.toFixed(2) })"
                  >
                    {{ t('systemSettings.resources.loadFriendly', { load: resourceOverview.current.load_1.toFixed(1), cores: resourceOverview.current.cpu_cores }) }}
                  </small>
                  <small v-else>{{ t('systemSettings.resources.cpuUsageHint') }}</small>
                </article>
                <article class="resource-summary-card">
                  <span class="resource-summary-label">{{ t('systemSettings.resources.host') }}</span>
                  <strong>{{ resourceOverview.current.hostname || '-' }}</strong>
                  <small>{{ resourceOverview.current.operating_system }} / {{ resourceOverview.current.architecture }} · {{ formatUptime(resourceOverview.current.uptime_seconds) }}</small>
                </article>
                <article class="resource-summary-card">
                  <span class="resource-summary-label">{{ t('systemSettings.resources.network') }}</span>
                  <strong v-if="resourceOverview.current.network_available">↓ {{ formatRate(resourceOverview.current.network_rx_bytes_per_second) }} · ↑ {{ formatRate(resourceOverview.current.network_tx_bytes_per_second) }}</strong>
                  <strong v-else>{{ t('systemSettings.resources.unavailable') }}</strong>
                  <small v-if="resourceOverview.current.network_available">{{ t('systemSettings.resources.networkTotal', { rx: formatBytes(resourceOverview.current.network_rx_bytes), tx: formatBytes(resourceOverview.current.network_tx_bytes) }) }}</small>
                </article>
                <article class="resource-summary-card">
                  <span class="resource-summary-label">{{ t('systemSettings.resources.diskIO') }}</span>
                  <strong v-if="resourceOverview.current.disk_io_available">↓ {{ formatRate(resourceOverview.current.disk_read_bytes_per_second) }} · ↑ {{ formatRate(resourceOverview.current.disk_write_bytes_per_second) }}</strong>
                  <strong v-else>{{ t('systemSettings.resources.unavailable') }}</strong>
                  <small v-if="resourceOverview.current.disk_io_available">{{ t('systemSettings.resources.diskIOTotal', { read: formatBytes(resourceOverview.current.disk_read_bytes), write: formatBytes(resourceOverview.current.disk_write_bytes) }) }}</small>
                </article>
                <article class="resource-summary-card">
                  <span class="resource-summary-label">{{ t('systemSettings.resources.swap') }}</span>
                  <strong v-if="resourceOverview.current.swap_total_bytes > 0">{{ formatBytes(resourceOverview.current.swap_used_bytes) }} / {{ formatBytes(resourceOverview.current.swap_total_bytes) }}</strong>
                  <strong v-else>{{ t('systemSettings.resources.notConfigured') }}</strong>
                  <el-progress v-if="resourceOverview.current.swap_total_bytes > 0" :percentage="roundedPercent(resourceOverview.current.swap_used_percent)" />
                  <small>{{ t('systemSettings.resources.runtimeFrequency', { seconds: resourceOverview.runtime_interval_seconds, minutes: resourceOverview.sample_interval_minutes }) }}</small>
                </article>
              </div>

              <SystemUsagePanel v-if="operationsTab === 'usage'" :key="usagePanelKey" />

              <div v-if="operationsTab === 'storage'" class="resource-panel">
                <div class="resource-panel-heading">
                  <div>
                    <h4>{{ t('systemSettings.resources.storagePoolsTitle') }}</h4>
                    <p>{{ t('systemSettings.resources.storagePoolsDesc') }}</p>
                  </div>
                </div>
                <div class="storage-pool-grid">
                  <article v-for="pool in resourceOverview.current.storage_pools" :key="pool.key" class="storage-pool-card">
                    <div class="storage-pool-title">
                      <strong>{{ storagePoolName(pool.key, pool.name) }}</strong>
                      <el-tag v-if="pool.primary" size="small" effect="plain">{{ t('systemSettings.resources.primaryPool') }}</el-tag>
                    </div>
                    <div v-if="pool.available" class="storage-pool-amount">{{ formatBytes(pool.used_bytes) }} / {{ formatBytes(pool.total_bytes) }}</div>
                    <div v-else class="storage-pool-amount">{{ t('systemSettings.resources.usedKnownCapacityUnknown', { value: formatBytes(pool.used_bytes) }) }}</div>
                    <el-progress v-if="pool.available" :percentage="roundedPercent(pool.used_percent)" :status="progressStatus(pool.used_percent)" />
                    <small v-if="pool.available">{{ t('systemSettings.resources.diskFree', { value: formatBytes(pool.free_bytes) }) }}</small>
                    <small v-else>{{ t('systemSettings.resources.capacityUnavailableHint') }}</small>
                  </article>
                </div>
              </div>

              <div v-if="operationsTab === 'overview'" class="resource-panel">
                <div class="resource-panel-heading">
                  <div>
                    <h4>{{ t('systemSettings.resources.platformTitle') }}</h4>
                    <p>{{ t('systemSettings.resources.platformDesc', { time: formatResourceTime(resourceOverview.platform.collected_at) }) }}</p>
                  </div>
                </div>
                <div class="platform-metric-grid">
                  <article class="platform-metric-card"><span>{{ t('systemSettings.resources.users') }}</span><strong>{{ resourceOverview.platform.users_total }}</strong><small>{{ t('systemSettings.resources.activeAndPending', { active: resourceOverview.platform.users_active, pending: resourceOverview.platform.users_pending }) }}</small></article>
                  <article class="platform-metric-card"><span>{{ t('systemSettings.resources.workspaces') }}</span><strong>{{ resourceOverview.platform.app_stats_available ? resourceOverview.platform.workspaces_total : '-' }}</strong><small>{{ resourceOverview.platform.app_stats_available ? t('systemSettings.resources.enabledCount', { count: resourceOverview.platform.workspaces_enabled }) : t('systemSettings.resources.sourceUnavailable') }}</small></article>
                  <article class="platform-metric-card"><span>{{ t('systemSettings.resources.serviceDirectories') }}</span><strong>{{ resourceOverview.platform.app_stats_available ? resourceOverview.platform.service_directories : '-' }}</strong><small>{{ resourceOverview.platform.app_stats_available ? t('systemSettings.resources.functionsCount', { count: resourceOverview.platform.functions_total }) : t('systemSettings.resources.sourceUnavailable') }}</small></article>
                  <article class="platform-metric-card"><span>{{ t('systemSettings.resources.appDatabases') }}</span><strong>{{ resourceOverview.platform.runtime_stats_available ? resourceOverview.platform.app_databases_total : '-' }}</strong><small>{{ resourceOverview.current.database_size_available ? t('systemSettings.resources.logicalDatabaseSize', { value: formatBytes(resourceOverview.current.database_logical_bytes) }) : t('systemSettings.resources.databaseSizeUnavailable') }}</small></article>
                  <article class="platform-metric-card"><span>{{ t('systemSettings.resources.scheduledTasks') }}</span><strong>{{ resourceOverview.platform.timer_stats_available ? resourceOverview.platform.scheduled_tasks_total : '-' }}</strong><small>{{ resourceOverview.platform.timer_stats_available ? t('systemSettings.resources.activeTasks', { count: resourceOverview.platform.scheduled_tasks_active }) : t('systemSettings.resources.sourceUnavailable') }}</small></article>
                </div>
              </div>

              <div v-if="operationsTab === 'databases'" class="resource-panel database-inventory">
                  <div class="database-inventory-heading">
                    <div>
                      <h4>{{ t('systemSettings.resources.databaseInventoryTitle') }}</h4>
                      <p>{{ t('systemSettings.resources.databaseInventoryDesc', { time: formatResourceTime(resourceOverview.capacity_collected_at) }) }}</p>
                    </div>
                    <div class="database-counts">
                      <el-tag effect="plain">{{ t('systemSettings.resources.databaseCountAll', { count: databaseLoadError ? '—' : databaseTotal }) }}</el-tag>
                      <el-tag type="info" effect="plain">{{ t('systemSettings.resources.databaseCountPlatform', { count: databaseLoadError ? '—' : platformDatabaseCount }) }}</el-tag>
                      <el-tag type="success" effect="plain">{{ t('systemSettings.resources.databaseCountWorkspace', { count: databaseLoadError ? '—' : workspaceDatabaseCount }) }}</el-tag>
                    </div>
                  </div>
                  <el-alert
                    v-if="databaseLoadError || !resourceOverview.current.database_inventory_complete"
                    :title="databaseLoadError || t('systemSettings.resources.databaseInventoryPartial')"
                    type="warning"
                    :closable="false"
                    show-icon
                  />
                  <div class="monitoring-policy-strip">
                    <span>{{ t('systemSettings.resources.capacityPolicy', { hour: resourceOverview.capacity_schedule_local, days: resourceOverview.capacity_retention_days }) }}</span>
                    <span>{{ t('systemSettings.resources.runtimePolicy', { seconds: resourceOverview.runtime_interval_seconds, minutes: resourceOverview.sample_interval_minutes, days: resourceOverview.runtime_retention_days }) }}</span>
                  </div>
                  <div v-if="latestCapacityDaily" class="database-delta-grid">
                    <article>
                      <span>{{ t('systemSettings.resources.databaseDailySize') }}</span>
                      <strong>{{ latestCapacityDaily.database_size_available ? formatBytes(latestCapacityDaily.database_logical_bytes) : '-' }}</strong>
                      <small>{{ latestCapacityDaily.database_logical_delta_available ? formatSignedBytes(latestCapacityDaily.database_logical_delta) : '-' }}</small>
                    </article>
                    <article>
                      <span>{{ t('systemSettings.resources.databaseDailyCount') }}</span>
                      <strong>{{ latestCapacityDaily.database_count_available ? latestCapacityDaily.database_count : '-' }}</strong>
                      <small>{{ latestCapacityDaily.database_count_delta_available ? formatSignedCount(latestCapacityDaily.database_count_delta) : '-' }}</small>
                    </article>
                  </div>
                  <SystemDatabaseHistory
                    :history="resourceOverview.capacity_history || []"
                    :databases="databaseHistoryOptions"
                    :days="databaseHistoryDays"
                    :database="databaseHistoryName"
                    @change="changeDatabaseHistory"
                  />
                  <div class="database-toolbar">
                    <el-input v-model="databaseSearch" clearable :placeholder="t('systemSettings.resources.databaseSearchPlaceholder')" />
                    <el-radio-group v-model="databaseScope" size="small">
                      <el-radio-button value="all">{{ t('systemSettings.resources.databaseScopeAll') }}</el-radio-button>
                      <el-radio-button value="platform">{{ t('systemSettings.resources.databaseScopePlatform') }}</el-radio-button>
                      <el-radio-button value="workspace">{{ t('systemSettings.resources.databaseScopeWorkspace') }}</el-radio-button>
                      <el-radio-button value="unmanaged">{{ t('systemSettings.resources.dashboard.databaseScopeUnmanaged') }}</el-radio-button>
                    </el-radio-group>
                  </div>
                  <el-table :data="databaseLoadError ? [] : databaseInventory" size="small" stripe class="database-size-table">
                    <el-table-column :label="t('systemSettings.resources.databaseType')" width="110">
                      <template #default="{ row }"><el-tag :type="row.kind === 'platform' ? 'info' : 'success'" size="small" effect="plain">{{ databaseKindLabel(row.kind) }}</el-tag></template>
                    </el-table-column>
                    <el-table-column prop="name" :label="t('systemSettings.resources.databaseName')" min-width="150" sortable><template #default="{ row }"><code class="database-code">{{ row.name }}</code><small v-if="row.source_id" class="database-source">{{ row.source_id.slice(0, 8) }}</small></template></el-table-column>
                    <el-table-column prop="owner" :label="t('systemSettings.resources.databaseOwner')" min-width="180" />
                    <el-table-column prop="directory" :label="t('systemSettings.resources.databaseDirectory')" min-width="230"><template #default="{ row }"><code class="database-directory">{{ databaseDirectoryLabel(row.kind, row.directory) }}</code></template></el-table-column>
                    <el-table-column prop="purpose" :label="t('systemSettings.resources.databasePurpose')" min-width="260"><template #default="{ row }">{{ databasePurposeLabel(row.purpose) }}</template></el-table-column>
                    <el-table-column :label="t('systemSettings.resources.databaseStatus')" width="110"><template #default="{ row }"><el-tag :type="databaseStatusType(row.status)" size="small">{{ databaseStatusLabel(row.status) }}</el-tag></template></el-table-column>
                    <el-table-column prop="used_bytes" :label="t('systemSettings.resources.logicalSize')" min-width="140" sortable><template #default="{ row }">{{ row.status === 'missing' ? '—' : formatBytes(row.used_bytes) }}</template></el-table-column>
                  </el-table>
                  <el-pagination
                    v-if="!databaseLoadError && databaseTotal > databasePageSize"
                    v-model:current-page="databasePage"
                    class="database-pagination"
                    background
                    layout="prev, pager, next, total"
                    :page-size="databasePageSize"
                    :total="databaseTotal"
                    @current-change="loadResourceDatabases"
                  />
              </div>

              <div v-if="operationsTab === 'trends' || operationsTab === 'overview'" class="resource-panel overview-trends">
                <div class="resource-panel-heading">
                  <div>
                    <h4>{{ t('systemSettings.resources.historyTitle') }}</h4>
                    <p>{{ t('systemSettings.resources.historyDesc', { minutes: resourceOverview.sample_interval_minutes, days: resourceOverview.runtime_retention_days }) }}</p>
                  </div>
                  <el-radio-group v-model="resourceHistoryHours" size="small" @change="loadResourceTrends">
                    <el-radio-button :value="24">24h</el-radio-button>
                    <el-radio-button :value="168">7d</el-radio-button>
                    <el-radio-button :value="720">30d</el-radio-button>
                  </el-radio-group>
                </div>
                <SystemResourceTrendChart v-if="resourceOverview.history.length > 1" :history="resourceOverview.history" />
                <el-empty v-else :description="t('systemSettings.resources.historyCollecting')" />
              </div>
              <div v-if="operationsTab === 'diagnostics'" class="advanced-content">
                    <div class="resource-panel">
                      <div class="resource-panel-heading"><div><h4>{{ t('systemSettings.resources.collectionTitle') }}</h4><p>{{ t('systemSettings.resources.collectionDesc') }}</p></div></div>
                      <el-table :data="resourceOverview.collection_tasks" size="small">
                        <el-table-column :label="t('systemSettings.resources.collectionTask')" min-width="150"><template #default="{ row }">{{ collectionTaskName(row.key) }}</template></el-table-column>
                        <el-table-column :label="t('systemSettings.resources.collectionStatus')" width="110"><template #default="{ row }"><el-tag :type="collectionStatusType(row.status)" size="small">{{ collectionStatusLabel(row.status) }}</el-tag></template></el-table-column>
                        <el-table-column :label="t('systemSettings.resources.lastSuccess')" min-width="180"><template #default="{ row }">{{ row.last_succeeded_at ? formatResourceTime(row.last_succeeded_at) : '-' }}</template></el-table-column>
                        <el-table-column :label="t('systemSettings.resources.nextCollection')" min-width="180"><template #default="{ row }">{{ row.next_run_at ? formatResourceTime(row.next_run_at) : '-' }}</template></el-table-column>
                        <el-table-column :label="t('systemSettings.resources.collectionDuration')" width="120"><template #default="{ row }">{{ formatDurationMillis(row.duration_millis) }}</template></el-table-column>
                        <el-table-column :label="t('systemSettings.resources.collectionResult')" min-width="180"><template #default="{ row }"><span :class="{ 'collection-error': row.error }">{{ collectionResult(row.error) }}</span></template></el-table-column>
                      </el-table>
                    </div>

              </div>
              <div v-if="operationsTab === 'storage'" class="advanced-content">
                    <div class="resource-panel">
                      <div class="resource-panel-heading">
                        <div>
                          <h4>{{ t('systemSettings.resources.breakdownTitle') }}</h4>
                          <p>{{ t('systemSettings.resources.breakdownDesc') }}</p>
                        </div>
                      </div>
                      <el-table :data="resourceOverview.current.components" size="small">
                        <el-table-column :label="t('systemSettings.resources.service')" min-width="180">
                          <template #default="{ row }">{{ resourceComponentName(row.key, row.name) }}</template>
                        </el-table-column>
                        <el-table-column :label="t('systemSettings.resources.storagePool')" min-width="150">
                          <template #default="{ row }">{{ storagePoolName(row.pool_key, row.pool_key) }}</template>
                        </el-table-column>
                        <el-table-column :label="t('systemSettings.resources.usedSpace')" min-width="160">
                          <template #default="{ row }">{{ row.available ? formatBytes(row.used_bytes) : t('systemSettings.resources.notMounted') }}</template>
                        </el-table-column>
                        <el-table-column :label="t('systemSettings.resources.share')" min-width="220">
                          <template #default="{ row }">
                            <el-progress v-if="row.available" :percentage="componentDiskPercent(row.used_bytes, row.pool_key)" />
                            <span v-else>-</span>
                          </template>
                        </el-table-column>
                      </el-table>
                    </div>
              </div>
              <p v-if="operationsTab === 'overview'" class="resource-collected-at">{{ t('systemSettings.resources.collectedAt', { time: formatResourceTime(resourceOverview.current.collected_at) }) }}</p>
            </template>
            <el-empty v-else-if="!resourcesLoading" :description="t('systemSettings.resources.empty')" />
          </div>

          <div v-else-if="activeTab === 'fileAssets'" class="section-pane">
            <SystemStorageAssetsPanel :key="storageAssetsPanelKey" />
          </div>

          <div v-else-if="activeTab === 'email'" v-loading="loading" class="section-pane">
            <el-alert
              v-if="form.registration_mode === 'admin_only'"
              :title="t('systemSettings.registrationDisabled')"
              type="info"
              show-icon
              :closable="false"
            />

            <el-form ref="formRef" :model="form" label-width="120px" class="settings-form">
              <el-divider content-position="left">{{ t('systemSettings.registration') }}</el-divider>
              <el-form-item :label="t('systemSettings.registrationMode')">
                <el-radio-group v-model="form.registration_mode">
                  <el-radio-button value="admin_only">{{ t('systemSettings.adminOnly') }}</el-radio-button>
                  <el-radio-button value="email_code">{{ t('systemSettings.emailVerification') }}</el-radio-button>
                  <el-radio-button value="debug_code">{{ t('systemSettings.debugCode') }}</el-radio-button>
                </el-radio-group>
              </el-form-item>

              <el-divider content-position="left">{{ t('systemSettings.emailService') }}</el-divider>
              <el-form-item :label="t('systemSettings.emailMode')">
                <el-radio-group v-model="form.email.mode">
                  <el-radio-button value="smtp">SMTP</el-radio-button>
                  <el-radio-button value="log">Log</el-radio-button>
                </el-radio-group>
              </el-form-item>

              <el-form-item :label="t('systemSettings.smtpHost')">
                <el-input v-model="form.email.host" placeholder="smtp.example.com" />
              </el-form-item>
              <el-form-item :label="t('systemSettings.smtpPort')">
                <el-input-number v-model="form.email.port" :min="1" :max="65535" />
              </el-form-item>
              <el-form-item :label="t('systemSettings.username')">
                <el-input v-model="form.email.username" :placeholder="t('systemSettings.smtpUsernamePlaceholder')" />
              </el-form-item>
              <el-form-item :label="t('systemSettings.password')">
                <el-input
                  v-model="form.email.password"
                  type="password"
                  show-password
                  :placeholder="form.email.password_set ? t('systemSettings.smtpPasswordKeepPlaceholder') : t('systemSettings.smtpPasswordPlaceholder')"
                />
              </el-form-item>
              <el-form-item :label="t('systemSettings.from')">
                <el-input v-model="form.email.from" placeholder="noreply@example.com" />
              </el-form-item>
              <el-form-item :label="t('systemSettings.fromName')">
                <el-input v-model="form.email.from_name" placeholder="kageos" />
              </el-form-item>

              <el-divider content-position="left">{{ t('systemSettings.testEmail') }}</el-divider>
              <el-form-item :label="t('systemSettings.recipient')">
                <div class="test-row">
                  <el-input v-model="testEmail" placeholder="admin@example.com" />
                  <el-button :icon="Message" :loading="testing" @click="sendTestEmail">
                    {{ t('systemSettings.sendTest') }}
                  </el-button>
                </div>
              </el-form-item>
            </el-form>
          </div>

          <div v-else-if="activeTab === 'login'" class="section-pane">
            <div v-loading="providersLoading" class="login-provider-section">
              <div class="login-announcement-panel">
                <div class="login-announcement-heading">
                  <div>
                    <h4>{{ t('systemSettings.loginAnnouncementTitle') }}</h4>
                    <p>{{ t('systemSettings.loginAnnouncementDesc') }}</p>
                  </div>
                  <el-switch
                    v-model="loginAnnouncement.enabled"
                    :active-text="t('systemSettings.on')"
                    :inactive-text="t('systemSettings.off')"
                  />
                </div>
                <el-form label-width="96px">
                  <el-form-item :label="t('systemSettings.loginAnnouncementMarkdown')">
                    <el-input
                      v-model="loginAnnouncement.markdown"
                      type="textarea"
                      :rows="5"
                      maxlength="10000"
                      show-word-limit
                      :placeholder="t('systemSettings.loginAnnouncementMarkdownPlaceholder')"
                    />
                  </el-form-item>
                  <div class="login-announcement-actions">
                    <el-button
                      :disabled="!loginAnnouncement.markdown.trim()"
                      @click="announcementPreviewVisible = true"
                    >
                      {{ t('systemSettings.previewLoginAnnouncement') }}
                    </el-button>
                    <el-button type="primary" :icon="Check" :loading="announcementSaving" @click="saveLoginAnnouncement">
                      {{ t('systemSettings.saveLoginAnnouncement') }}
                    </el-button>
                  </div>
                </el-form>
              </div>

              <div class="provider-summary" :aria-label="t('systemSettings.loginOverview')">
                <span>{{ t('systemSettings.loginPresetCount') }} {{ authProviders.length }}</span>
                <el-tag size="small" effect="plain">{{ t('systemSettings.configuredCount') }} {{ configuredProviderCount }}</el-tag>
                <el-tag size="small" type="success" effect="plain">{{ t('systemSettings.enabledCount') }} {{ enabledProviderCount }}</el-tag>
              </div>

              <el-empty v-if="!authProviders.length && !providersLoading" :description="t('systemSettings.noLoginProviders')" />

              <section
                v-for="provider in authProviders"
                :key="provider.code"
                class="provider-panel"
                :class="{ 'is-expanded': isProviderExpanded(provider.code) }"
              >
                <div class="provider-panel-header">
                  <button
                    type="button"
                    class="provider-toggle"
                    :aria-expanded="isProviderExpanded(provider.code)"
                    @click="toggleProvider(provider.code)"
                  >
                    <span class="provider-logo" aria-hidden="true">
                      <img :src="authProviderLogo(provider.code)" alt="" />
                    </span>
                    <span class="provider-heading">
                      <span class="provider-title-row">
                        <span class="provider-name">{{ provider.name }}</span>
                        <el-tag :type="providerStatusType(provider)" size="small">
                          {{ providerStatusLabel(provider) }}
                        </el-tag>
                        <el-tag size="small" effect="plain">
                          {{ providerActionLabel(provider.action) }}
                        </el-tag>
                      </span>
                      <span class="provider-description">{{ provider.description }}</span>
                    </span>
                    <el-icon class="provider-chevron" :class="{ 'is-expanded': isProviderExpanded(provider.code) }">
                      <ArrowDown />
                    </el-icon>
                  </button>
                  <div class="provider-enable" @click.stop>
                    <span>{{ t('systemSettings.enabled') }}</span>
                    <el-switch
                      :model-value="provider.enabled"
                      :disabled="!provider.configured"
                      :loading="providerSwitching[provider.code]"
                      @change="handleProviderSwitchChange(provider, $event)"
                    />
                  </div>
                </div>

                <el-collapse-transition>
                  <div v-show="isProviderExpanded(provider.code)" class="provider-body">
                    <div class="provider-meta">
                      <span v-if="provider.callback_path" class="callback-path">
                        {{ t('systemSettings.callbackUrl') }}：<code>{{ callbackURL(provider) }}</code>
                      </span>
                      <el-button
                        v-if="provider.callback_path"
                        link
                        type="primary"
                        :icon="CopyDocument"
                        @click="copyCallbackURL(provider)"
                      >
                        {{ t('connectorProvider.copy') }}
                      </el-button>
                      <el-link
                        v-if="provider.docs_url"
                        type="primary"
                        :href="provider.docs_url"
                        target="_blank"
                        :underline="false"
                      >
                        {{ t('systemSettings.platformDocs') }}
                      </el-link>
                    </div>

                    <el-form
                      v-if="providerConfigs[provider.code]"
                      :model="providerConfigs[provider.code]"
                      label-width="120px"
                      class="provider-form"
                    >
                  <el-form-item
                    v-for="field in provider.fields"
                    :key="field.key"
                    :label="field.label"
                    :required="field.required"
                  >
                    <div class="provider-field">
                      <el-switch
                        v-if="field.type === 'boolean'"
                        :model-value="providerBooleanFieldValue(provider, field)"
                        :active-text="t('systemSettings.on')"
                        :inactive-text="t('systemSettings.off')"
                        @change="setProviderFieldValue(provider, field, $event)"
                      />
                      <el-input
                        v-else
                        :model-value="providerFieldValue(provider, field)"
                        :type="fieldInputType(field)"
                        :show-password="field.secret"
                        :placeholder="fieldPlaceholder(field)"
                        @update:model-value="setProviderFieldValue(provider, field, $event)"
                      >
                        <template v-if="isCallbackField(field)" #append>
                          <el-button @click="fillCallbackURL(provider, field.key)">
                            {{ t('systemSettings.useCurrentCallback') }}
                          </el-button>
                        </template>
                      </el-input>
                      <div v-if="field.help" class="field-help">{{ field.help }}</div>
                    </div>
                  </el-form-item>
                  <el-form-item>
                    <div class="provider-form-actions">
                      <el-button
                        type="primary"
                        :icon="Check"
                        :loading="providerSaving[provider.code]"
                        @click="saveProviderConfig(provider)"
                      >
                        {{ t('systemSettings.saveProvider') }}
                      </el-button>
                      <el-button
                        :disabled="!provider.configured"
                        :loading="providerSwitching[provider.code]"
                        @click="handleProviderEnabledChange(provider, !provider.enabled)"
                      >
                        {{ provider.enabled ? t('systemSettings.disable') : t('systemSettings.enable') }}
                      </el-button>
                    </div>
                  </el-form-item>
                    </el-form>
                  </div>
                </el-collapse-transition>
              </section>
            </div>
          </div>

          <div v-else-if="activeTab === 'connectors'" class="section-pane">
            <ConnectorProviderManagementPage :key="connectorPanelKey" embedded />
          </div>

          <div v-else-if="activeTab === 'openapi'" class="section-pane">
            <OpenAPITokenManagementPage :key="openapiPanelKey" embedded />
          </div>

          <div v-else-if="activeTab === 'users'" class="section-pane">
            <SystemUserManagementPage :key="usersPanelKey" />
          </div>

          <div v-else-if="activeTab === 'dataBackup'" class="section-pane">
            <SystemBackupPanel ref="backupPanel" />
          </div>

          <div v-else-if="activeTab === 'backups'" v-loading="archivesLoading" class="section-pane backup-pane">
            <el-alert :title="t('systemSettings.archiveFallbackNote')" type="info" show-icon :closable="false" />
            <div class="archive-summary-row">
              <div class="archive-policy-item">
                <span>{{ t('systemSettings.archiveRetentionLabel') }}</span>
                <strong>{{ t('systemSettings.archiveRetentionValue', { days: archiveRetentionDays }) }}</strong>
                <small>{{ t('systemSettings.archiveRetentionHint') }}</small>
              </div>
              <div class="archive-policy-item">
                <span>{{ t('systemSettings.archiveScheduleLabel') }}</span>
                <strong>{{ archiveScheduleFriendly }}</strong>
                <small :title="t('systemSettings.archiveSchedule', { cron: archiveCronExpr, timezone: archiveTimezone })">{{ archiveTimezone }}</small>
              </div>
            </div>
            <el-table v-if="archiveBatches.length" :data="archiveBatches" size="small" class="archive-table">
              <el-table-column type="expand" width="44">
                <template #default="{ row }">
                  <div class="archive-detail-grid">
                    <div><span>{{ t('systemSettings.archiveKey') }}</span><code>{{ row.archive_key || '-' }}</code></div>
                    <div><span>{{ t('systemSettings.archiveObjectRef') }}</span><code>{{ row.object_ref || '-' }}</code></div>
                    <div><span>SHA256</span><code>{{ row.sha256 || '-' }}</code></div>
                    <div v-if="row.error_message"><span>{{ t('systemSettings.archiveError') }}</span><strong class="archive-error">{{ row.error_message }}</strong></div>
                  </div>
                </template>
              </el-table-column>
              <el-table-column :label="t('systemSettings.archiveScope')" min-width="210">
                <template #default="{ row }">
                  <div class="archive-primary-cell">
                    <strong>{{ row.tenant_user }}/{{ row.app }}</strong>
                    <small>{{ formatArchiveTime(row.range_started_at) }} — {{ formatArchiveTime(row.range_ended_at) }}</small>
                  </div>
                </template>
              </el-table-column>
              <el-table-column :label="t('systemSettings.archiveData')" width="150">
                <template #default="{ row }">
                  <div class="archive-primary-cell">
                    <strong>{{ t('systemSettings.archiveRecordValue', { count: row.record_count }) }}</strong>
                    <small>{{ formatFileSize(row.file_size) }}</small>
                  </div>
                </template>
              </el-table-column>
              <el-table-column :label="t('systemSettings.archiveSummary')" min-width="260">
                <template #default="{ row }"><span class="archive-resource-summary">{{ archiveResourceSummary(row) }}</span></template>
              </el-table-column>
              <el-table-column :label="t('systemSettings.archiveRetry')" min-width="190">
                <template #default="{ row }">
                  <div class="archive-primary-cell">
                    <small>{{ t('systemSettings.archiveAttempts', { count: row.attempts || 0 }) }}</small>
                    <small v-if="row.next_retry_at">{{ t('systemSettings.archiveNextRetry') }} {{ formatArchiveTime(row.next_retry_at) }}</small>
                    <el-button v-if="row.status !== 'completed'" type="primary" plain size="small" :loading="retryingArchive === row.id" :disabled="retryingArchive !== null && retryingArchive !== row.id" @click="retryArchive(row)">{{ t('systemSettings.archiveRetry') }}</el-button>
                  </div>
                </template>
              </el-table-column>
              <el-table-column :label="t('systemSettings.archiveStatus')" width="110" align="right">
                <template #default="{ row }"><el-tag :type="archiveStatusType(row.status)" size="small">{{ archiveStatusLabel(row.status) }}</el-tag></template>
              </el-table-column>
            </el-table>
            <el-empty v-else-if="!archivesLoading" :description="t('systemSettings.noArchives')" />
            <el-pagination
              v-if="archiveTotal > archivePageSize"
              v-model:current-page="archivePage"
              :page-size="archivePageSize"
              :total="archiveTotal"
              layout="prev, pager, next"
              @current-change="loadArchives"
            />
          </div>

          <div v-else-if="activeTab === 'appearance'" class="section-pane">
            <div class="preference-grid">
              <button
                v-for="theme in availableThemes"
                :key="theme.name"
                type="button"
                class="preference-card"
                :class="{ 'is-active': currentThemeName === theme.name }"
                @click="handleThemeChange(theme.name)"
              >
                <span class="preference-card-title">{{ theme.label }}</span>
                <span class="preference-card-desc">{{ theme.mode === 'dark' ? t('workspace.darkUi') : t('workspace.lightUi') }}</span>
              </button>
            </div>
          </div>

          <div v-else-if="activeTab === 'language'" class="section-pane">
            <div class="preference-grid">
              <button
                v-for="option in localeStore.localeOptions"
                :key="option.value"
                type="button"
                class="preference-card"
                :class="{ 'is-active': option.value === localeStore.currentLocale }"
                @click="handleLocaleChange(option.value)"
              >
                <span class="preference-card-title">{{ option.flag }} {{ option.nativeLabel }}</span>
                <span class="preference-card-desc">{{ option.englishLabel }}</span>
              </button>
            </div>
          </div>
        </section>
      </div>
    </el-card>

    <el-dialog
      v-model="announcementPreviewVisible"
      :title="t('systemSettings.loginAnnouncementPreviewTitle')"
      width="min(640px, calc(100vw - 32px))"
      align-center
    >
      <div
        class="login-announcement-preview"
        v-html="renderMarkdown(loginAnnouncement.markdown)"
      />
      <template #footer>
        <div class="login-announcement-preview-footer">
          <span class="login-announcement-preview-hint">
            {{ t('systemSettings.loginAnnouncementPreviewHint') }}
          </span>
          <el-button @click="announcementPreviewVisible = false">{{ t('common.close') }}</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import {
  ArrowDown,
  Brush,
  Check,
  Coin,
  Connection,
  CopyDocument,
  Document,
  FolderOpened,
  Key,
  Lock,
  Message,
  Monitor,
  QuestionFilled,
  Reading,
  Refresh,
  Setting,
  User,
} from '@element-plus/icons-vue'
import { useLocaleStore, useThemeStore } from '@/architecture/presentation/context/appStoresContext'
import type { SupportedLocale } from '@/architecture/shared/i18n'
import { authProviderLogo } from '@/architecture/shared/assets/authProviderLogos'
import { getKageosDocsURL, openExternalURL, type KageosDocSlug } from '@/architecture/shared/config/externalLinks'
import { featureFlags } from '@/architecture/shared/config/features'
import ConnectorProviderManagementPage from '@/architecture/presentation/features/connector/pages/ConnectorProviderManagementPage.vue'
import OpenAPITokenManagementPage from '@/architecture/presentation/features/agent/pages/OpenAPITokenManagementPage.vue'
import SystemUserManagementPage from '@/architecture/presentation/features/system/pages/SystemUserManagementPage.vue'
import SystemResourceTrendChart from '@/architecture/presentation/features/system/components/SystemResourceTrendChart.vue'
import SystemDatabaseHistory from '../components/SystemDatabaseHistory.vue'
import SystemUsagePanel from '@/architecture/presentation/features/system/components/SystemUsagePanel.vue'
import SystemStorageAssetsPanel from '@/architecture/presentation/features/system/components/SystemStorageAssetsPanel.vue'
import SystemBackupPanel from '@/architecture/presentation/features/system/components/SystemBackupPanel.vue'
import { useLazyMarkdownRenderer } from '@/architecture/presentation/composables/useLazyMarkdownRenderer'
import {
  getSystemSettings,
  getLoginAnnouncementConfig,
  getSystemResourceSummary,
  getSystemResourceTrends,
  getSystemResourceStorage,
  getSystemResourceDatabases,
  getSystemResourceDiagnostics,
  listAuthLoginProviders,
  listLogArchiveBatches,
  retryLogArchiveBatch,
  type SystemDatabaseSize,
  updateSystemSettings,
  updateLoginAnnouncementConfig,
  updateAuthLoginProviderConfig,
  updateAuthLoginProviderEnabled,
  testSystemEmail,
  type AuthLoginProviderField,
  type AuthLoginProviderInfo,
  type LogArchiveBatch,
  type LoginAnnouncement,
  type SystemResourceOverview,
  type SystemResourceSummary,
  type SystemSettings
} from '@/architecture/presentation/context/api/system-settings'

type SettingsTab = 'operations' | 'fileAssets' | 'email' | 'login' | 'connectors' | 'openapi' | 'users' | 'dataBackup' | 'backups' | 'appearance' | 'language'
type OperationsTab = 'overview' | 'usage' | 'trends' | 'storage' | 'databases' | 'diagnostics'

interface SettingsSection {
  key: SettingsTab
  title: string
  desc: string
  icon: Component
}

type SettingsGroupKey = 'system' | 'access' | 'integrations' | 'preferences'

interface SettingsNavigationGroup {
  key: SettingsGroupKey
  title: string
  sections: SettingsSection[]
}

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const testEmail = ref('')
const defaultSettingsTab: SettingsTab = 'operations'
const activeTab = ref<SettingsTab>(defaultSettingsTab)
const connectorPanelKey = ref(0)
const openapiPanelKey = ref(0)
const usersPanelKey = ref(0)
const backupPanel = ref<InstanceType<typeof SystemBackupPanel> | null>(null)
const usagePanelKey = ref(0)
const storageAssetsPanelKey = ref(0)
const archivesLoading = ref(false)
const archiveBatches = ref<LogArchiveBatch[]>([])
const retryingArchive = ref<number | null>(null)
const databaseHistoryDays = ref(7)
const databaseHistoryName = ref('')
const databaseHistoryOptions = ref<SystemDatabaseSize[]>([])
let databaseRequestId = 0
let lastDatabaseHistoryKey = ''
const databaseLoadError = ref('')
async function retryArchive(batch: LogArchiveBatch) {
  if (retryingArchive.value !== null) return
  retryingArchive.value = batch.id
  try {
    await retryLogArchiveBatch(batch.id)
    ElMessage.success(t('systemSettings.archiveRetrySucceeded'))
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.archiveRetryFailed'))
  } finally {
    retryingArchive.value = null
    await loadArchives()
  }
}
function changeDatabaseHistory(days: number, name: string) {
 databaseHistoryDays.value = days
 databaseHistoryName.value = name
 void loadResourceDatabases()
}
const archivePage = ref(1)
const archivePageSize = 20
const archiveTotal = ref(0)
const archiveRetentionDays = ref(90)
const archiveCronExpr = ref('20 3 * * *')
const archiveTimezone = ref('Asia/Shanghai')
const resourcesLoading = ref(false)
const resourceOverview = ref<SystemResourceOverview | null>(null)
const operationsTab = ref<OperationsTab>('overview')
const loadedOperationsTabs = reactive<Record<OperationsTab, boolean>>({ overview: false, usage: false, trends: false, storage: false, databases: false, diagnostics: false })
const resourceHistoryHours = ref(168)
const databaseScope = ref<'all' | 'platform' | 'workspace'>('all')
const databaseSearch = ref('')
const databasePage = ref(1)
const databasePageSize = 20
const databaseTotal = ref(0)
const platformDatabaseCount = ref(0)
const workspaceDatabaseCount = ref(0)
let resourceRefreshTimer: ReturnType<typeof setInterval> | undefined
let databaseSearchTimer: ReturnType<typeof setTimeout> | undefined
const providersLoading = ref(false)
const announcementSaving = ref(false)
const announcementPreviewVisible = ref(false)
const authProviders = ref<AuthLoginProviderInfo[]>([])
const expandedProviderCodes = ref<string[]>([])
const providerConfigs = reactive<Record<string, Record<string, string>>>({})
const providerSaving = reactive<Record<string, boolean>>({})
const providerSwitching = reactive<Record<string, boolean>>({})
const loginAnnouncement = reactive<LoginAnnouncement>({ enabled: false, markdown: '' })
const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const { renderMarkdown, preloadMarkdown } = useLazyMarkdownRenderer()
const localeStore = useLocaleStore()
const themeStore = useThemeStore()

const allSettingsSections = computed<SettingsSection[]>(() => [
  { key: 'operations', title: t('systemSettings.sections.operationsTitle'), desc: t('systemSettings.sections.operationsDesc'), icon: Monitor },
  { key: 'fileAssets', title: t('systemSettings.sections.fileAssetsTitle'), desc: t('systemSettings.sections.fileAssetsDesc'), icon: FolderOpened },
  { key: 'email', title: t('systemSettings.sections.emailTitle'), desc: t('systemSettings.sections.emailDesc'), icon: Message },
  { key: 'login', title: t('systemSettings.sections.loginTitle'), desc: t('systemSettings.sections.loginDesc'), icon: Lock },
  { key: 'users', title: t('systemSettings.sections.usersTitle'), desc: t('systemSettings.sections.usersDesc'), icon: User },
  { key: 'dataBackup', title: t('systemSettings.sections.dataBackupTitle'), desc: t('systemSettings.sections.dataBackupDesc'), icon: Coin },
  { key: 'backups', title: t('systemSettings.sections.backupsTitle'), desc: t('systemSettings.sections.backupsDesc'), icon: Document },
  { key: 'openapi', title: t('systemSettings.sections.openapiTitle'), desc: t('systemSettings.sections.openapiDesc'), icon: Key },
  { key: 'connectors', title: t('systemSettings.sections.connectorsTitle'), desc: t('systemSettings.sections.connectorsDesc'), icon: Connection },
  { key: 'appearance', title: t('systemSettings.sections.appearanceTitle'), desc: t('systemSettings.sections.appearanceDesc'), icon: Brush },
  { key: 'language', title: t('systemSettings.sections.languageTitle'), desc: t('systemSettings.sections.languageDesc'), icon: Reading },
])

const settingsSections = computed<SettingsSection[]>(() => {
  return allSettingsSections.value.filter((section) => {
    if (section.key === 'email') return featureFlags.systemEmailSettings
    if (section.key === 'connectors') return featureFlags.connectorSettings
    if (section.key === 'openapi') return featureFlags.openapiTokens
    return true
  })
})

const settingsNavigationGroups = computed<SettingsNavigationGroup[]>(() => {
  const availableSections = new Map(settingsSections.value.map((section) => [section.key, section]))
  const groups: Array<{ key: SettingsGroupKey; title: string; keys: SettingsTab[] }> = [
    { key: 'system', title: t('systemSettings.navigationGroups.system'), keys: ['operations', 'fileAssets'] },
    { key: 'access', title: t('systemSettings.navigationGroups.access'), keys: ['login', 'users'] },
    { key: 'integrations', title: t('systemSettings.navigationGroups.integrations'), keys: ['dataBackup', 'backups', 'email', 'openapi', 'connectors'] },
    { key: 'preferences', title: t('systemSettings.navigationGroups.preferences'), keys: ['appearance', 'language'] },
  ]

  return groups
    .map((group) => ({ ...group, sections: group.keys.flatMap((key) => availableSections.get(key) ? [availableSections.get(key)!] : []) }))
    .filter((group) => group.sections.length > 0)
})

const settingsDocSlugMap: Record<SettingsTab, KageosDocSlug> = {
  operations: 'runtime',
  fileAssets: 'runtime',
  email: 'runtime',
  login: 'login',
  connectors: 'connectors',
  openapi: 'api',
  users: 'runtime',
  dataBackup: 'data-backup',
  backups: 'runtime',
  appearance: 'docs',
  language: 'docs',
}

const currentSection = computed(() => {
  return settingsSections.value.find((section) => section.key === activeTab.value) || settingsSections.value[0]!
})

const currentDocsURL = computed(() => {
  return getKageosDocsURL(settingsDocSlugMap[activeTab.value], localeStore.currentLocale)
})

const availableThemes = computed(() => themeStore.getAvailableThemes())
const currentThemeName = computed(() => themeStore.currentTheme.name)

const forecastAlertType = computed(() => {
  if (resourceOverview.value?.forecast.status === 'critical') return 'error'
  if (resourceOverview.value?.forecast.status === 'warning') return 'warning'
  return 'success'
})

const forecastTitle = computed(() => {
  const status = resourceOverview.value?.forecast.status || 'healthy'
  return t(`systemSettings.resources.forecast.${status}`)
})

const forecastDescription = computed(() => {
  const forecast = resourceOverview.value?.forecast
  if (!forecast) return ''
  if (typeof forecast.days_to_target === 'number') {
    return t('systemSettings.resources.forecastDays', {
      days: forecast.days_to_target,
      target: forecast.target_percent,
      growth: formatBytes(forecast.daily_growth_bytes),
    })
  }
  if (forecast.status === 'critical') {
    return t('systemSettings.resources.forecastCriticalNow', {
      pool: storagePoolName(forecast.pool_key, forecast.pool_key),
      percent: roundedPercent(forecast.current_used_percent),
    })
  }
  if (forecast.status === 'warning') {
    return t('systemSettings.resources.forecastWarningNow', {
      pool: storagePoolName(forecast.pool_key, forecast.pool_key),
      percent: roundedPercent(forecast.current_used_percent),
      target: forecast.target_percent,
    })
  }
  return t('systemSettings.resources.forecastStable', { target: forecast.target_percent })
})

const databaseInventory = computed(() => resourceOverview.value?.current.databases?.length
  ? resourceOverview.value.current.databases
  : resourceOverview.value?.current.largest_databases || [])
const latestCapacityDaily = computed(() => resourceOverview.value?.capacity_history?.at(-1))

watch(databaseScope, () => {
  databaseHistoryName.value = ''
  databasePage.value = 1
  if (operationsTab.value === 'databases') void loadResourceDatabases()
})
watch(databaseSearch, () => {
  databasePage.value = 1
  if (databaseSearchTimer) clearTimeout(databaseSearchTimer)
  databaseSearchTimer = setTimeout(() => {
    if (operationsTab.value === 'databases') void loadResourceDatabases()
  }, 300)
})

const environmentTitle = computed(() => {
  const environment = resourceOverview.value?.current.environment
  if (!environment) return ''
  return t(`systemSettings.resources.environments.${environment.mode}.${environment.deployment}`)
})

const environmentDescription = computed(() => {
  const environment = resourceOverview.value?.current.environment
  if (!environment) return ''
  const engine = environment.container_engine && environment.container_engine !== 'none'
    ? environment.container_engine
    : t('systemSettings.resources.noContainerEngine')
  return t('systemSettings.resources.environmentDesc', {
    os: resourceOverview.value?.current.operating_system,
    arch: resourceOverview.value?.current.architecture,
    engine,
    source: t(`systemSettings.resources.storageSources.${environment.storage_root_source}`),
  })
})

const form = reactive<SystemSettings>({
  registration_mode: 'admin_only',
  email: {
    mode: 'smtp',
    host: '',
    port: 587,
    username: '',
    password: '',
    password_set: false,
    from: '',
    from_name: 'kageos',
  },
})

const configuredProviderCount = computed(() => authProviders.value.filter((provider) => provider.configured).length)
const enabledProviderCount = computed(() => authProviders.value.filter((provider) => provider.enabled).length)
const loadStatusType = computed<'success' | 'warning' | 'danger' | 'info'>(() => {
  const current = resourceOverview.value?.current
  if (!current?.load_available || current.cpu_cores <= 0) return 'info'
  const ratio = current.load_1 / current.cpu_cores
  if (ratio >= 0.85) return 'danger'
  if (ratio >= 0.6) return 'warning'
  return 'success'
})
const loadStatusLabel = computed(() => t(`systemSettings.resources.loadStatuses.${loadStatusType.value}`))
const archiveScheduleFriendly = computed(() => {
  const parts = archiveCronExpr.value.trim().split(/\s+/)
  if (parts.length === 5 && parts[2] === '*' && parts[3] === '*' && parts[4] === '*') {
    const minute = Number(parts[0])
    const hour = Number(parts[1])
    if (Number.isInteger(minute) && Number.isInteger(hour) && minute >= 0 && minute < 60 && hour >= 0 && hour < 24) {
      return t('systemSettings.archiveDailyAt', {
        time: `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`
      })
    }
  }
  return t('systemSettings.archiveCustomSchedule')
})

function isProviderExpanded(code: string) {
  return expandedProviderCodes.value.includes(code)
}

function toggleProvider(code: string) {
  expandedProviderCodes.value = isProviderExpanded(code)
    ? expandedProviderCodes.value.filter(item => item !== code)
    : [...expandedProviderCodes.value, code]
}

function applySettings(settings: SystemSettings) {
  form.registration_mode = settings.registration_mode
  form.email = {
    ...settings.email,
    password: '',
  }
}

function applyProviderConfigDraft(provider: AuthLoginProviderInfo) {
  const config: Record<string, string> = {}
  provider.fields.forEach((field) => {
    config[field.key] = field.secret ? '' : field.value || ''
  })
  providerConfigs[provider.code] = config
}

function replaceProvider(updated: AuthLoginProviderInfo) {
  const index = authProviders.value.findIndex((item) => item.code === updated.code)
  if (index >= 0) {
    authProviders.value[index] = updated
  } else {
    authProviders.value.push(updated)
  }
  applyProviderConfigDraft(updated)
}

async function loadSettings() {
  loading.value = true
  try {
    applySettings(await getSystemSettings())
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.loadSettingsFailed'))
  } finally {
    loading.value = false
  }
}

async function loadAuthProviders() {
  providersLoading.value = true
  try {
    const [providersResult, announcementResult] = await Promise.allSettled([
      listAuthLoginProviders(),
      getLoginAnnouncementConfig(),
    ])
    if (providersResult.status === 'fulfilled') {
      authProviders.value = providersResult.value.providers || []
      authProviders.value.forEach(applyProviderConfigDraft)
    } else {
      const error: any = providersResult.reason
      ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.loadAuthProvidersFailed'))
    }
    if (announcementResult.status === 'fulfilled') {
      Object.assign(loginAnnouncement, announcementResult.value)
    } else {
      const error: any = announcementResult.reason
      ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.loginAnnouncementLoadFailed'))
    }
  } finally {
    providersLoading.value = false
  }
}

async function saveLoginAnnouncement() {
  if (loginAnnouncement.enabled && !loginAnnouncement.markdown.trim()) {
    ElMessage.warning(t('systemSettings.loginAnnouncementContentRequired'))
    return
  }
  announcementSaving.value = true
  try {
    Object.assign(loginAnnouncement, await updateLoginAnnouncementConfig({ ...loginAnnouncement }))
    ElMessage.success(t('systemSettings.loginAnnouncementSaved'))
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.loginAnnouncementSaveFailed'))
  } finally {
    announcementSaving.value = false
  }
}

async function refreshActiveTab() {
  if (activeTab.value === 'operations') {
		if (operationsTab.value === 'usage') {
			usagePanelKey.value += 1
			return
		}
    await loadResources()
    return
  }
  if (activeTab.value === 'login') {
    await loadAuthProviders()
    return
  }
  if (activeTab.value === 'fileAssets') {
    storageAssetsPanelKey.value += 1
    return
  }
  if (activeTab.value === 'connectors') {
    connectorPanelKey.value += 1
    return
  }
  if (activeTab.value === 'openapi') {
    openapiPanelKey.value += 1
    return
  }
  if (activeTab.value === 'users') {
    usersPanelKey.value += 1
    return
  }
  if (activeTab.value === 'dataBackup') {
    await backupPanel.value?.refresh()
    return
  }
  if (activeTab.value === 'backups') {
    await loadArchives()
    return
  }
  if (activeTab.value === 'appearance' || activeTab.value === 'language') {
    return
  }
  await loadSettings()
}

function handleTabChange(tabName: string | number) {
  if (tabName === 'operations' && !resourceOverview.value) {
    loadResources()
  }
  if (tabName === 'login' && !authProviders.value.length) {
    loadAuthProviders()
  }
  if (tabName === 'backups' && !archiveBatches.value.length) {
    loadArchives()
  }
}

async function loadResources() {
 if (operationsTab.value === 'databases') lastDatabaseHistoryKey = ''
	await loadActiveOperationsTab(true)
}

function overviewFromSummary(summary: SystemResourceSummary): SystemResourceOverview {
  return {
    current: { ...summary.current, storage_pools: [], components: [], databases: [], largest_databases: [] },
    history: [],
    capacity_history: [],
    history_hours: resourceHistoryHours.value,
    sample_interval_minutes: summary.sample_interval_minutes,
    runtime_retention_days: summary.runtime_retention_days,
    platform_retention_days: 0,
    capacity_retention_days: 0,
    platform_interval_hours: 24,
    capacity_interval_hours: 24,
    platform_schedule_local: '',
    capacity_schedule_local: '',
    capacity_collected_at: '',
    forecast: summary.forecast,
    platform: summary.platform,
    collection_tasks: [],
    runtime_interval_seconds: summary.runtime_interval_seconds,
  }
}

async function loadResourceSummary(silent = false) {
	if (!silent) resourcesLoading.value = true
	try {
		const summary = await getSystemResourceSummary()
		if (!resourceOverview.value) {
			resourceOverview.value = overviewFromSummary(summary)
		} else {
			const previous = resourceOverview.value
			resourceOverview.value = {
				...previous,
				current: {
					...summary.current,
					storage_pools: previous.current.storage_pools,
					components: previous.current.components,
					databases: previous.current.databases,
					largest_databases: previous.current.largest_databases,
				},
				platform: summary.platform,
				forecast: loadedOperationsTabs.storage ? previous.forecast : summary.forecast,
				sample_interval_minutes: summary.sample_interval_minutes,
				runtime_retention_days: summary.runtime_retention_days,
				runtime_interval_seconds: summary.runtime_interval_seconds,
			}
		}
		loadedOperationsTabs.overview = true
	} catch (error: any) {
		if (!silent) ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.resources.loadFailed'))
	} finally {
		if (!silent) resourcesLoading.value = false
	}
}

async function loadResourceTrends() {
  resourcesLoading.value = true
  try {
		const result = await getSystemResourceTrends(resourceHistoryHours.value)
		if (resourceOverview.value) {
			Object.assign(resourceOverview.value, {
				history: result.history,
				history_hours: result.history_hours,
				sample_interval_minutes: result.sample_interval_minutes,
				runtime_retention_days: result.runtime_retention_days,
			})
		}
		loadedOperationsTabs.trends = true
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.resources.loadFailed'))
  } finally {
    resourcesLoading.value = false
  }
}

async function loadResourceStorage() {
	resourcesLoading.value = true
	try {
		const result = await getSystemResourceStorage()
		if (resourceOverview.value) {
			resourceOverview.value.current.environment = result.environment
			resourceOverview.value.current.storage_pools = result.storage_pools
			resourceOverview.value.current.components = result.components
			resourceOverview.value.forecast = result.forecast
			resourceOverview.value.capacity_retention_days = result.capacity_retention_days
			resourceOverview.value.capacity_schedule_local = result.capacity_schedule_local
			resourceOverview.value.capacity_collected_at = result.collected_at
		}
		loadedOperationsTabs.storage = true
	} catch (error: any) {
		ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.resources.loadFailed'))
	} finally {
		resourcesLoading.value = false
	}
}

async function loadResourceDatabases() {
 const requestId = ++databaseRequestId
 const historyKey = JSON.stringify([databaseScope.value, databaseHistoryDays.value, databaseHistoryName.value])
 const includeHistory = historyKey !== lastDatabaseHistoryKey
 databaseLoadError.value = ''
 if (includeHistory) {
  lastDatabaseHistoryKey = ''
  if (resourceOverview.value) resourceOverview.value.capacity_history = []
 }
	resourcesLoading.value = true
	try {
		const result = await getSystemResourceDatabases({
			page: databasePage.value,
			page_size: databasePageSize,
			scope: databaseScope.value,
			keyword: databaseSearch.value.trim(),
			include_history: includeHistory,
 days: databaseHistoryDays.value,
 database: databaseHistoryName.value,
		})
		if (requestId !== databaseRequestId) return
		if (resourceOverview.value) {
			resourceOverview.value.current.databases = result.items
			resourceOverview.value.current.largest_databases = []
			resourceOverview.value.current.database_logical_bytes = result.database_logical_bytes
			resourceOverview.value.current.database_size_available = result.database_size_available
			resourceOverview.value.current.database_inventory_complete = result.database_inventory_complete
			if (includeHistory) {
 resourceOverview.value.capacity_history = result.capacity_history || []
 databaseHistoryOptions.value = result.history_databases || []
 lastDatabaseHistoryKey = historyKey
 }
			resourceOverview.value.capacity_retention_days = result.capacity_retention_days
			resourceOverview.value.capacity_schedule_local = result.capacity_schedule_local
			resourceOverview.value.capacity_collected_at = result.collected_at
		}
		databaseTotal.value = result.total
		platformDatabaseCount.value = result.platform_count
		workspaceDatabaseCount.value = result.workspace_count
		loadedOperationsTabs.databases = true
	} catch (error: any) {
		if (requestId !== databaseRequestId) return
		databaseLoadError.value = error?.response?.data?.msg || error?.message || t('systemSettings.resources.loadFailed')
 ElMessage.error(databaseLoadError.value)
	} finally {
		if (requestId === databaseRequestId) resourcesLoading.value = false
	}
}

async function loadResourceDiagnostics() {
	resourcesLoading.value = true
	try {
		const result = await getSystemResourceDiagnostics()
		if (resourceOverview.value) {
			resourceOverview.value.current.environment = result.environment
			resourceOverview.value.collection_tasks = result.collection_tasks
			resourceOverview.value.platform_retention_days = result.platform_retention_days
			resourceOverview.value.capacity_retention_days = result.capacity_retention_days
			resourceOverview.value.platform_schedule_local = result.platform_schedule_local
			resourceOverview.value.capacity_schedule_local = result.capacity_schedule_local
			resourceOverview.value.sample_interval_minutes = result.sample_interval_minutes
			resourceOverview.value.runtime_retention_days = result.runtime_retention_days
			resourceOverview.value.runtime_interval_seconds = result.runtime_interval_seconds
		}
		loadedOperationsTabs.diagnostics = true
	} catch (error: any) {
		ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.resources.loadFailed'))
	} finally {
		resourcesLoading.value = false
	}
}

async function loadActiveOperationsTab(force = false) {
	const tab = operationsTab.value
	if (!force && loadedOperationsTabs[tab]) return
	if (tab === 'overview') { await loadResourceSummary(); if (resourceOverview.value) await loadResourceTrends(); return }
	if (!resourceOverview.value) await loadResourceSummary()
	if (tab === 'usage') {
		loadedOperationsTabs.usage = true
		return
	}
	if (tab === 'trends') return loadResourceTrends()
	if (tab === 'storage') return loadResourceStorage()
	if (tab === 'databases') return loadResourceDatabases()
	return loadResourceDiagnostics()
}

function handleOperationsTabChange(tabName: string | number) {
	operationsTab.value = tabName as OperationsTab
	void loadActiveOperationsTab()
	router.replace({ path: route.path, query: { ...route.query, resource_tab: operationsTab.value } })
}

async function refreshResourcesSilently() {
  if (activeTab.value !== 'operations' || operationsTab.value !== 'overview' || resourcesLoading.value) return
  try {
    await loadResourceSummary(true)
  } catch {
    // Keep the last successful snapshot; the explicit refresh path reports errors.
  }
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value < 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let amount = value
  let unit = 0
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024
    unit += 1
  }
  return `${amount.toFixed(unit === 0 ? 0 : amount >= 100 ? 0 : 1)} ${units[unit]}`
}

function formatRate(value: number) {
  return `${formatBytes(value)}/s`
}

function formatSignedBytes(value: number) {
  if (!Number.isFinite(value)) return '-'
  if (value === 0) return t('systemSettings.resources.dashboard.unchanged')
  return `${value > 0 ? '+' : '-'}${formatBytes(Math.abs(value))}`
}

function formatSignedCount(value: number) {
  if (!Number.isFinite(value)) return '-'
  return value > 0 ? `+${value}` : String(value)
}

function roundedPercent(value: number) {
  return Math.min(100, Math.max(0, Number(value.toFixed(1))))
}

function progressStatus(value: number): 'success' | 'warning' | 'exception' | undefined {
  if (value >= 90) return 'exception'
  if (value >= 80) return 'warning'
  if (value < 60) return 'success'
  return undefined
}

function componentDiskPercent(bytes: number, poolKey: string) {
  const total = resourceOverview.value?.current.storage_pools.find((pool) => pool.key === poolKey)?.total_bytes || 0
  return total > 0 ? roundedPercent(bytes * 100 / total) : 0
}

function storagePoolName(key: string, fallback: string) {
  const translated = t(`systemSettings.resources.pools.${key}`)
  return translated.includes('systemSettings.resources.pools.') ? fallback : translated
}

function resourceComponentName(key: string, fallback: string) {
  const translated = t(`systemSettings.resources.components.${key}`)
  return translated.includes('systemSettings.resources.components.') ? fallback : translated
}

function formatUptime(seconds: number) {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  return t('systemSettings.resources.uptime', { days, hours })
}

function formatResourceTime(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function databaseKindLabel(kind: string) {
  return kind === 'unmanaged' ? t('systemSettings.resources.dashboard.databaseScopeUnmanaged') : t(`systemSettings.resources.databaseKinds.${kind}`)
}

function databaseStatusLabel(status: string) {
  return t(`systemSettings.resources.databaseStatuses.${status}`)
}

function databaseStatusType(status: string): 'success' | 'warning' | 'danger' | 'info' {
  if (status === 'active') return 'success'
  if (status === 'pending') return 'warning'
  if (status === 'missing') return 'danger'
  return 'info'
}

function databasePurposeLabel(purpose: string) {
  if (purpose === 'unmanaged_database') return t('systemSettings.resources.dashboard.databaseScopeUnmanaged')
  const translated = t(`systemSettings.resources.databasePurposes.${purpose}`)
  return translated.includes('systemSettings.resources.databasePurposes.') ? purpose : translated
}

function databaseDirectoryLabel(kind: string, directory: string) {
  return kind === 'platform' ? t('systemSettings.resources.databasePlatformDirectory') : directory
}

function collectionTaskName(key: string) {
  return t(`systemSettings.resources.collectionTasks.${key}`)
}

function collectionStatusLabel(status: string) {
  return t(`systemSettings.resources.collectionStatuses.${status}`)
}

function collectionStatusType(status: string): 'success' | 'warning' | 'danger' | 'info' {
  if (status === 'success') return 'success'
  if (status === 'running' || status === 'partial') return 'warning'
  if (status === 'failed') return 'danger'
  return 'info'
}

function collectionResult(error?: string) {
  if (!error) return t('systemSettings.resources.collectionNormal')
  const knownErrors: Record<string, string> = {
    'one or more platform metric sources are unavailable': 'platformSourceUnavailable',
    'application database metric source is unavailable': 'databaseSourceUnavailable',
    'database metric source is unavailable': 'databaseSourceUnavailable',
    'runtime metrics collected but history persistence failed': 'runtimePersistenceFailed'
  }
  const key = knownErrors[error]
  return key ? t(`systemSettings.resources.collectionErrors.${key}`) : error
}

function formatDurationMillis(value: number) {
  if (value < 1000) return `${value} ms`
  return `${(value / 1000).toFixed(1)} s`
}

async function loadArchives() {
  archivesLoading.value = true
  try {
    const resp = await listLogArchiveBatches(archivePage.value, archivePageSize)
    archiveBatches.value = resp.list || []
    archiveTotal.value = resp.total || 0
    archiveRetentionDays.value = resp.retention_days || 90
    archiveCronExpr.value = resp.cron_expr || '20 3 * * *'
    archiveTimezone.value = resp.timezone || 'Asia/Shanghai'
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.loadArchivesFailed'))
  } finally {
    archivesLoading.value = false
  }
}

function formatArchiveTime(value?: string) {
  if (!value) return '-'
  const date = new Date(value.replace(' ', 'T'))
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function formatFileSize(size: number) {
  if (!size) return '-'
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

function archiveStatusType(status: string) {
  if (status === 'completed') return 'success'
  if (status === 'failed') return 'danger'
  return 'warning'
}

function archiveStatusLabel(status: string) {
  return t(`systemSettings.archiveStatuses.${status}`)
}

function archiveResourceSummary(batch: LogArchiveBatch) {
  const paths = batch.summary_json?.top_resource_paths || []
  if (!paths.length) return '-'
  return paths.slice(0, 3).map((item) => `${item.resource_path || '/'} (${item.count})`).join('；')
}

function isSettingsTab(value: unknown): value is SettingsTab {
  return typeof value === 'string' && settingsSections.value.some((section) => section.key === value)
}

function isOperationsTab(value: unknown): value is OperationsTab {
	return value === 'overview' || value === 'usage' || value === 'trends' || value === 'storage' || value === 'databases' || value === 'diagnostics'
}

function selectSettingsSection(tabName: SettingsTab) {
  activeTab.value = tabName
  handleTabChange(tabName)
  router.replace({
    path: route.path,
    query: { ...route.query, tab: tabName }
  })
}

function handleMobileSettingsChange(event: Event) {
  const tabName = (event.target as HTMLSelectElement).value
  if (isSettingsTab(tabName)) selectSettingsSection(tabName)
}

function openCurrentDocs() {
  openExternalURL(currentDocsURL.value)
}

function handleThemeChange(themeName: string) {
  const theme = themeStore.getAvailableThemes().find((item) => item.name === themeName)
  if (theme) {
    themeStore.setTheme(theme)
  }
}

function handleLocaleChange(locale: string) {
  localeStore.setAppLocale(locale as SupportedLocale)
}

async function saveSettings() {
  saving.value = true
  try {
    applySettings(await updateSystemSettings(JSON.parse(JSON.stringify(form))))
    ElMessage.success(t('systemSettings.settingsSaved'))
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.saveSettingsFailed'))
  } finally {
    saving.value = false
  }
}

function providerStatusType(provider: AuthLoginProviderInfo) {
  if (provider.status === 'enabled') {
    return 'success'
  }
  if (provider.status === 'disabled') {
    return 'warning'
  }
  return 'info'
}

function providerStatusLabel(provider: AuthLoginProviderInfo) {
  if (provider.status === 'enabled') {
    return t('systemSettings.statusEnabled')
  }
  if (provider.status === 'disabled') {
    return t('systemSettings.statusConfiguredDisabled')
  }
  return t('systemSettings.statusUnconfigured')
}

function providerActionLabel(action: string) {
  if (action === 'qrcode') {
    return t('systemSettings.actionQrcode')
  }
  if (action === 'redirect') {
    return t('systemSettings.actionRedirect')
  }
  return action || t('systemSettings.actionAuthorize')
}

function fieldInputType(field: AuthLoginProviderField) {
  if (field.secret) {
    return 'password'
  }
  if (field.type === 'url') {
    return 'url'
  }
  return 'text'
}

function fieldPlaceholder(field: AuthLoginProviderField) {
  if (field.secret && field.value_set) {
    return t('systemSettings.secretKeepPlaceholder')
  }
  if (field.placeholder) {
    return field.placeholder
  }
  return field.secret ? t('systemSettings.secretPlaceholder') : t('systemSettings.configValuePlaceholder')
}

function isCallbackField(field: AuthLoginProviderField) {
  return field.key === 'redirect_url' || field.key === 'callback_url'
}

function providerFieldValue(provider: AuthLoginProviderInfo, field: AuthLoginProviderField) {
  return providerConfigs[provider.code]?.[field.key] || ''
}

function providerBooleanFieldValue(provider: AuthLoginProviderInfo, field: AuthLoginProviderField) {
  return providerFieldValue(provider, field) === 'true'
}

function setProviderFieldValue(provider: AuthLoginProviderInfo, field: AuthLoginProviderField, value: string | number | boolean) {
  let config = providerConfigs[provider.code]
  if (!config) {
    config = {}
    providerConfigs[provider.code] = config
  }
  if (field.type === 'boolean') {
    config[field.key] = value ? 'true' : 'false'
    return
  }
  config[field.key] = String(value ?? '')
}

function callbackURL(provider: AuthLoginProviderInfo) {
  const path = provider.callback_path || ''
  if (!path) {
    return ''
  }
  if (/^https?:\/\//i.test(path)) {
    return path
  }
  if (typeof window === 'undefined') {
    return path
  }
  return `${window.location.origin}${path.startsWith('/') ? path : `/${path}`}`
}

function fillCallbackURL(provider: AuthLoginProviderInfo, fieldKey: string) {
  const config = providerConfigs[provider.code]
  if (!config) {
    return
  }
  config[fieldKey] = callbackURL(provider)
}

async function copyCallbackURL(provider: AuthLoginProviderInfo) {
  try {
    await navigator.clipboard.writeText(callbackURL(provider))
    ElMessage.success(t('connectorProvider.callbackCopied'))
  } catch {
    ElMessage.error(t('connectorProvider.copyFailed'))
  }
}

function buildProviderConfigPayload(provider: AuthLoginProviderInfo) {
  const draft = providerConfigs[provider.code] || {}
  const payload: Record<string, string> = {}
  provider.fields.forEach((field) => {
    const value = (draft[field.key] || '').trim()
    if (field.secret && !value) {
      return
    }
    payload[field.key] = value
  })
  return payload
}

function validateProviderConfig(provider: AuthLoginProviderInfo) {
  const draft = providerConfigs[provider.code] || {}
  const missing = provider.fields.find((field) => {
    if (!field.required) {
      return false
    }
    if (field.secret && field.value_set) {
      return false
    }
    return !(draft[field.key] || '').trim()
  })
  if (missing) {
    ElMessage.warning(t('systemSettings.providerFieldMissing', { provider: provider.name, field: missing.label }))
    return false
  }
  return true
}

async function saveProviderConfig(provider: AuthLoginProviderInfo) {
  if (!validateProviderConfig(provider)) {
    return
  }
  providerSaving[provider.code] = true
  try {
    const updated = await updateAuthLoginProviderConfig(provider.code, buildProviderConfigPayload(provider))
    replaceProvider(updated)
    ElMessage.success(t('systemSettings.providerSaved'))
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.providerSaveFailed'))
  } finally {
    providerSaving[provider.code] = false
  }
}

async function handleProviderEnabledChange(provider: AuthLoginProviderInfo, enabled: boolean) {
  if (enabled && !provider.configured) {
    ElMessage.warning(t('systemSettings.saveBeforeEnable'))
    return
  }
  providerSwitching[provider.code] = true
  try {
    const updated = await updateAuthLoginProviderEnabled(provider.code, enabled)
    replaceProvider(updated)
    ElMessage.success(enabled ? t('systemSettings.providerEnabled') : t('systemSettings.providerDisabled'))
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.providerStatusFailed'))
  } finally {
    providerSwitching[provider.code] = false
  }
}

function handleProviderSwitchChange(provider: AuthLoginProviderInfo, value: string | number | boolean) {
  handleProviderEnabledChange(provider, Boolean(value))
}

async function sendTestEmail() {
  if (!testEmail.value.trim()) {
    ElMessage.warning(t('systemSettings.recipientRequired'))
    return
  }
  testing.value = true
  try {
    await testSystemEmail(testEmail.value.trim())
    ElMessage.success(t('systemSettings.testEmailSent'))
  } catch (error: any) {
    ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.testEmailFailed'))
  } finally {
    testing.value = false
  }
}

onMounted(() => {
  preloadMarkdown()
  const tab = Array.isArray(route.query.tab) ? route.query.tab[0] : route.query.tab
	const resourceTab = Array.isArray(route.query.resource_tab) ? route.query.resource_tab[0] : route.query.resource_tab
	const legacyAssetsTab = resourceTab === 'assets'
	if (isOperationsTab(resourceTab)) operationsTab.value = resourceTab
  if (legacyAssetsTab) {
    activeTab.value = 'fileAssets'
  } else if (isSettingsTab(tab)) {
    activeTab.value = tab
  } else if (!isSettingsTab(activeTab.value)) {
    activeTab.value = settingsSections.value[0]?.key || defaultSettingsTab
  }
  if (activeTab.value === 'email') {
    loadSettings()
  } else {
    handleTabChange(activeTab.value)
  }
  resourceRefreshTimer = setInterval(refreshResourcesSilently, 30_000)
})

onBeforeUnmount(() => {
  if (resourceRefreshTimer) clearInterval(resourceRefreshTimer)
	if (databaseSearchTimer) clearTimeout(databaseSearchTimer)
})
</script>

<style scoped>
.system-settings-page {
  --settings-page-bg: var(--app-shell-bg, var(--bg-primary));
  --settings-sidebar-bg: color-mix(in srgb, var(--settings-page-bg) 96%, var(--color-primary) 4%);
  --settings-sidebar-border: color-mix(in srgb, var(--color-primary) 12%, var(--border-light));
  --settings-nav-hover-bg: color-mix(in srgb, var(--color-primary) 6%, transparent);
  --settings-nav-active-bg: color-mix(in srgb, var(--color-primary) 14%, var(--settings-sidebar-bg));
  --settings-nav-active-mark: color-mix(in srgb, var(--color-primary) 82%, var(--text-primary));
  min-height: 100vh;
  padding: 0;
  background: var(--settings-page-bg);
}

.settings-card {
  width: 100%;
  min-height: 100vh;
  border: 0;
  border-radius: 0;
  background: transparent;
}

.settings-card :deep(.el-card__body) { padding: 0; }

.header-actions,
.test-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.settings-layout {
  display: grid;
  grid-template-columns: 236px minmax(0, 1fr);
  gap: 0;
  align-items: stretch;
  min-height: 100vh;
}

.settings-sidebar {
  border-right: 1px solid var(--settings-sidebar-border);
  background: var(--settings-sidebar-bg);
  padding: 28px 16px 24px;
  display: flex;
  flex-direction: column;
  gap: 22px;
}

.settings-sidebar-heading {
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr);
  align-items: start;
  gap: 10px;
  padding: 0 8px 6px;
}

.settings-sidebar-mark {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  border: 1px solid color-mix(in srgb, var(--color-primary) 18%, transparent);
  background: color-mix(in srgb, var(--color-primary) 10%, transparent);
  color: var(--settings-nav-active-mark);
  font-size: 18px;
}

.settings-sidebar-heading h2,
.settings-mobile-heading h2 {
  margin: 0;
  color: var(--text-primary);
  font-size: 18px;
  font-weight: 650;
  line-height: 1.25;
}

.settings-sidebar-heading p,
.settings-mobile-heading p {
  margin: 4px 0 0;
  color: var(--text-secondary);
  font-size: 11px;
  line-height: 1.45;
}

.settings-nav-group {
  display: grid;
  gap: 7px;
}

.settings-nav-group-title {
  margin: 0;
  padding: 0 11px;
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.04em;
}

.settings-nav-list {
  display: grid;
  gap: 3px;
}

.settings-nav-item {
  position: relative;
  width: 100%;
  min-height: 40px;
  display: grid;
  grid-template-columns: 20px minmax(0, 1fr);
  align-items: center;
  gap: 10px;
  padding: 8px 11px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--text-primary);
  cursor: pointer;
  text-align: left;
  transition: background-color 0.18s ease, color 0.18s ease;
}

.settings-nav-item:hover {
  background: var(--settings-nav-hover-bg);
}

.settings-nav-item.is-active {
  background: var(--settings-nav-active-bg);
  color: var(--settings-nav-active-mark);
}

.settings-nav-item.is-active::before {
  position: absolute;
  top: 9px;
  bottom: 9px;
  left: 0;
  width: 3px;
  border-radius: 0 4px 4px 0;
  background: var(--settings-nav-active-mark);
  content: '';
}

.settings-nav-icon {
  color: var(--text-secondary);
  font-size: 17px;
}

.settings-nav-item:hover .settings-nav-icon,
.settings-nav-item.is-active .settings-nav-icon {
  color: currentColor;
}

.settings-nav-title {
  display: block;
  font-size: 14px;
  font-weight: 500;
  line-height: 1.35;
}

.settings-nav-item.is-active .settings-nav-title { font-weight: 650; }

.settings-content {
  min-width: 0;
  background: var(--settings-page-bg);
  padding: 34px clamp(28px, 4vw, 56px) 48px;
}

.settings-mobile-heading { display: none; }

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  padding-bottom: 20px;
  margin-bottom: 24px;
  border-bottom: 1px solid var(--border-light);
}

.section-header h3 {
  margin: 0 0 8px;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}

.section-header p {
  margin: 0;
  color: var(--text-secondary);
  font-size: 14px;
  line-height: 1.5;
}

.section-pane {
  min-width: 0;
}

.settings-form {
  max-width: 760px;
}

.archive-summary-row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 260px));
  gap: 12px;
  margin: 16px 0 18px;
}

.archive-policy-item {
  display: grid;
  gap: 4px;
  padding: 12px 14px;
  border: 1px solid var(--border-light);
  border-radius: var(--border-radius-base);
  background: var(--bg-tertiary);
}

.archive-policy-item span,
.archive-policy-item small {
  color: var(--text-secondary);
  font-size: 12px;
}

.archive-policy-item strong {
  color: var(--text-primary);
  font-size: 15px;
}

.archive-table {
  margin-bottom: 16px;
}

.archive-primary-cell {
  display: grid;
  gap: 4px;
}

.archive-primary-cell strong {
  color: var(--text-primary);
  font-size: 13px;
}

.archive-primary-cell small,
.archive-resource-summary {
  color: var(--text-secondary);
  font-size: 12px;
  line-height: 1.45;
}

.archive-detail-grid {
  display: grid;
  gap: 10px;
  padding: 8px 18px 12px 44px;
}

.archive-detail-grid > div {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr);
  gap: 12px;
  color: var(--text-secondary);
  font-size: 12px;
}

.archive-detail-grid code {
  color: var(--text-primary);
  overflow-wrap: anywhere;
}

.archive-error { color: var(--el-color-danger); }
.backup-pane :deep(.el-pagination) { justify-content: flex-end; }

.test-row {
  width: 100%;
}

.login-provider-section {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-height: 160px;
}

.login-announcement-panel {
  padding: 14px 16px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 8px;
  background: var(--el-bg-color);
}

.login-announcement-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 12px;
}

.login-announcement-heading h4 {
  margin: 0;
  font-size: 16px;
  color: var(--text-primary);
}

.login-announcement-heading p {
  margin: 8px 0 0;
  color: var(--text-secondary);
  line-height: 1.5;
}

.login-announcement-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.login-announcement-preview {
  min-height: 120px;
  color: var(--el-text-color-primary);
  font-size: 15px;
  line-height: 1.75;
  overflow-wrap: anywhere;
}

.login-announcement-preview :deep(:first-child) {
  margin-top: 0;
}

.login-announcement-preview :deep(:last-child) {
  margin-bottom: 0;
}

.login-announcement-preview-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.login-announcement-preview-hint {
  color: var(--el-text-color-secondary);
  font-size: 13px;
  text-align: left;
}

.provider-summary {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding: 2px 0;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.provider-panel {
  border: 1px solid var(--el-border-color-light);
  border-radius: var(--border-radius-base);
  background: var(--el-bg-color);
  overflow: hidden;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

.provider-panel:hover,
.provider-panel.is-expanded {
  border-color: var(--el-border-color);
}

.provider-panel.is-expanded {
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.04);
}

.provider-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 66px;
  padding: 10px 14px;
}

.provider-toggle {
  display: grid;
  grid-template-columns: 38px minmax(0, 1fr) 20px;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex: 1;
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
  text-align: left;
}

.provider-toggle:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 4px;
  border-radius: 4px;
}

.provider-logo {
  width: 36px;
  height: 36px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px;
  background: #fff;
  overflow: hidden;
}

.provider-logo img { width: 28px; height: 28px; object-fit: contain; }

.provider-chevron {
  color: var(--text-secondary);
  transition: transform 0.2s ease;
}

.provider-chevron.is-expanded { transform: rotate(180deg); }

.provider-body {
  padding: 0 16px 14px 66px;
  border-top: 1px solid var(--el-border-color-extra-light);
}

.provider-heading {
  min-width: 0;
  flex: 1;
}

.provider-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.provider-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.provider-description {
  display: block;
  margin-top: 4px;
  color: var(--text-secondary);
  font-size: 12px;
  line-height: 1.4;
}

.provider-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 10px;
  font-size: 13px;
  color: var(--text-secondary);
}

.callback-path {
  min-width: 0;
  word-break: break-all;
}

.callback-path code {
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--el-fill-color-light);
  color: var(--text-primary);
}

.provider-enable {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--text-secondary);
  white-space: nowrap;
}

.provider-form {
  max-width: 860px;
  padding-top: 12px;
}

.provider-field {
  width: 100%;
}

.field-help {
  margin-top: 6px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.5;
}

.provider-form-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.preference-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
  max-width: 720px;
}

.preference-card {
  min-height: 82px;
  padding: 16px 20px;
  border: 1px solid transparent;
  border-radius: var(--border-radius-lg);
  background: var(--bg-tertiary);
  color: var(--text-regular);
  text-align: left;
  cursor: pointer;
  transition: all 0.25s cubic-bezier(0.25, 0.8, 0.25, 1);
}

.preference-card:hover {
  background: var(--el-fill-color);
}

.preference-card.is-active {
  background: var(--el-fill-color-blank);
  border-color: var(--color-primary);
  box-shadow: 0 0 0 2px rgba(var(--color-primary-rgb), 0.15);
  color: var(--color-primary);
}

.preference-card-title {
  display: block;
  font-size: 15px;
  font-weight: 700;
}

.preference-card-desc {
  display: block;
  margin-top: 8px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.45;
}

.operations-pane { display: grid; gap: 20px; }

.operations-tabs {
  margin-bottom: 2px;
  overflow: hidden;
}
.operations-tabs :deep(.el-tabs__header) { margin: 0; }
.operations-tabs :deep(.el-tabs__nav-wrap) { overflow-x: auto; }
.operations-tabs :deep(.el-tabs__item) {
  height: 42px;
  padding: 0 18px;
  color: var(--text-secondary);
  font-weight: 500;
}
.operations-tabs :deep(.el-tabs__item.is-active) {
  color: var(--color-primary);
  font-weight: 650;
}

.resource-summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}

.resource-summary-card,
.resource-panel {
  border: 1px solid var(--border-light);
  border-radius: var(--border-radius-lg);
  background: var(--bg-primary);
}

.resource-summary-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
  padding: 18px;
}

.resource-summary-card strong {
  overflow: hidden;
  color: var(--text-primary);
  font-size: 18px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.resource-value-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
}

.resource-value-row strong { min-width: 0; }

.resource-summary-card small,
.resource-panel-heading p,
.resource-collected-at { color: var(--text-secondary); }

.resource-summary-label {
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 600;
}

.resource-panel { padding: 20px; }

.storage-pool-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 14px;
}

.storage-pool-card {
  display: grid;
  gap: 10px;
  padding: 16px;
  border: 1px solid var(--border-light);
  border-radius: var(--border-radius-lg);
  background: var(--bg-secondary);
}

.storage-pool-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.storage-pool-amount { color: var(--text-primary); font-size: 16px; }
.storage-pool-card small { color: var(--text-secondary); }

.platform-metric-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.platform-metric-card {
  display: grid;
  gap: 7px;
  padding: 14px;
  border: 1px solid var(--border-light);
  border-radius: var(--border-radius-lg);
  background: var(--bg-secondary);
}

.platform-metric-card span,
.platform-metric-card small { color: var(--text-secondary); font-size: 12px; }
.platform-metric-card strong { color: var(--text-primary); font-size: 24px; }
.database-inventory-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.database-inventory-heading h4 { margin: 0; color: var(--text-primary); font-size: 15px; }
.database-inventory-heading p { margin: 6px 0 0; color: var(--text-secondary); font-size: 12px; line-height: 1.5; }
.database-counts { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
.monitoring-policy-strip { display: flex; flex-wrap: wrap; gap: 8px 18px; margin: 14px 0; color: var(--text-secondary); font-size: 12px; }
.monitoring-policy-strip span { display: inline-flex; align-items: center; }
.monitoring-policy-strip span::before { width: 6px; height: 6px; margin-right: 7px; border-radius: 50%; background: var(--el-color-primary); content: ''; }
.database-delta-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin: 14px 0; }
.database-delta-grid article { display: grid; gap: 6px; padding: 14px; border: 1px solid var(--border-light); border-radius: var(--border-radius-lg); background: var(--bg-secondary); }
.database-delta-grid span,
.database-delta-grid small { color: var(--text-secondary); font-size: 12px; }
.database-delta-grid strong { color: var(--text-primary); font-size: 20px; }
.database-daily-history { margin: 18px 0; }
.database-daily-history h5 { margin: 0 0 10px; color: var(--text-primary); font-size: 13px; }
.database-toolbar { display: grid; grid-template-columns: minmax(240px, 1fr) auto; gap: 12px; align-items: center; margin: 16px 0 12px; }
.database-size-table { margin-top: 8px; }
.database-code,
.database-directory { color: var(--text-primary); font-size: 12px; overflow-wrap: anywhere; }
.database-pagination { justify-content: flex-end; margin-top: 16px; }
.collection-error { color: var(--el-color-danger); }

.operations-advanced {
  border: 1px solid var(--border-light);
  border-radius: var(--border-radius-lg);
  background: var(--bg-primary);
  overflow: hidden;
}

.operations-advanced :deep(.el-collapse-item__header) {
  height: auto;
  min-height: 58px;
  padding: 10px 18px;
  border-bottom: 0;
  background: transparent;
}

.operations-advanced :deep(.el-collapse-item__wrap) { border-bottom: 0; background: transparent; }
.operations-advanced :deep(.el-collapse-item__content) { padding: 0 16px 16px; }

.advanced-title {
  display: grid;
  gap: 2px;
  text-align: left;
}

.advanced-title strong { color: var(--text-primary); font-size: 14px; }
.advanced-title span { color: var(--text-secondary); font-size: 12px; font-weight: 400; }
.advanced-content { display: grid; gap: 14px; }

.resource-panel-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
}

.resource-panel-heading h4,
.resource-panel-heading p { margin: 0; }
.resource-panel-heading p { margin-top: 5px; font-size: 13px; }
.resource-collected-at { margin: -6px 0 0; text-align: right; font-size: 12px; }

@media (max-width: 980px) {
  .settings-layout { grid-template-columns: 216px minmax(0, 1fr); }
  .settings-sidebar { padding-inline: 12px; }
  .settings-content { padding-inline: 28px; }
}

@media (max-width: 768px) {
  .settings-layout {
    grid-template-columns: 1fr;
  }

  .settings-sidebar {
    display: none;
  }

  .settings-content {
    padding: 22px 18px 36px;
  }

  .settings-mobile-heading {
    display: grid;
    gap: 18px;
    margin-bottom: 24px;
    padding-bottom: 20px;
    border-bottom: 1px solid var(--border-light);
  }

  .settings-mobile-heading h2 { font-size: 20px; }
  .settings-mobile-heading p { font-size: 13px; }

  .settings-mobile-select {
    display: grid;
    gap: 7px;
    color: var(--text-secondary);
    font-size: 12px;
    font-weight: 600;
  }

  .settings-mobile-select select {
    width: 100%;
    min-height: 42px;
    padding: 0 38px 0 12px;
    border: 1px solid var(--border-light);
    border-radius: 9px;
    outline: none;
    background: var(--bg-secondary);
    color: var(--text-primary);
    font: inherit;
    font-size: 14px;
  }

  .settings-mobile-select select:focus-visible {
    border-color: var(--color-primary);
    box-shadow: 0 0 0 3px rgba(var(--color-primary-rgb), 0.14);
  }

  .section-header {
    flex-direction: column;
    align-items: stretch;
  }

  .section-header .header-actions {
    width: 100%;
  }

  .archive-summary-row {
    grid-template-columns: 1fr;
  }

  .provider-body { padding-left: 16px; }

  .resource-summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .database-inventory-heading { flex-direction: column; }
  .database-toolbar { grid-template-columns: 1fr; }
  .database-counts { justify-content: flex-start; }
  .database-delta-grid { grid-template-columns: 1fr; }

  .settings-form,
  .provider-form {
    max-width: none;
  }

  .test-row {
    flex-direction: column;
    align-items: stretch;
  }
}

@media (max-width: 520px) {
  .preference-grid,
  .resource-summary-grid {
    grid-template-columns: 1fr;
  }

  .section-header .header-actions :deep(.el-button) {
    flex: 1 1 auto;
    margin-left: 0;
  }

  .resource-panel-heading { flex-direction: column; }

  .provider-panel-header {
    align-items: flex-start;
  }

  .provider-enable > span { display: none; }

  .provider-toggle {
    grid-template-columns: 34px minmax(0, 1fr) 18px;
    gap: 8px;
  }

  .provider-logo { width: 32px; height: 32px; border-radius: 8px; }

  .archive-detail-grid { padding-left: 8px; }
  .archive-detail-grid > div { grid-template-columns: 1fr; gap: 4px; }
}

.operations-pane { --dashboard-accent:#5985ed; }
.resource-summary-grid { gap:16px; }
.resource-summary-card { padding:22px; border-radius:14px; background:var(--bg-primary); }
.resource-summary-card > strong { font-size:26px; letter-spacing:-.5px; font-variant-numeric:tabular-nums; }
.platform-metric-grid { gap:14px; }
.platform-metric-card { padding:20px; border-radius:14px; background:var(--bg-primary); border-top:3px solid var(--dashboard-accent); }
.platform-metric-card strong { font-size:30px; font-variant-numeric:tabular-nums; }
.database-inventory { padding:24px; }
.database-inventory-heading h4 { font-size:22px; letter-spacing:-.5px; }
.database-counts { flex-wrap:wrap; }
.database-delta-grid { gap:16px; }
.database-delta-grid article { background:var(--bg-primary); border-radius:14px; padding:20px 24px; }
.database-delta-grid strong { font-size:28px; font-variant-numeric:tabular-nums; }
.database-size-table :deep(.el-table__cell) { padding:14px 0; }
.database-size-table :deep(.cell) { font-size:12px; }
.database-source { display:block; color:var(--text-secondary); font-size:10px; margin-top:3px; }
.archive-table :deep(.el-table__cell) { padding:16px 0; }
.archive-policy-item { border-radius:14px; background:var(--bg-primary); padding:24px; }
.monitoring-policy-strip { font-size:11px; opacity:.8; }
.overview-trends :deep(.trend-card) { border-radius:14px; background:var(--bg-primary); }
@media (max-width:720px) { .database-inventory { padding:12px; } .database-toolbar { flex-direction:column; align-items:stretch; } }
</style>
