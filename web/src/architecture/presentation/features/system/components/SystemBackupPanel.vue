<template>
  <div v-loading="loading" class="system-backup-panel">
    <el-alert
      :title="t('systemSettings.dataBackup.scopeTitle')"
      :description="t('systemSettings.dataBackup.scopeDesc')"
      type="info"
      show-icon
      :closable="false"
    />

    <div class="backup-status-row">
      <div>
        <span>{{ t('systemSettings.dataBackup.autoBackup') }}</span>
        <strong>{{ overview?.config.enabled ? t('systemSettings.on') : t('systemSettings.off') }}</strong>
      </div>
      <div>
        <span>{{ t('systemSettings.dataBackup.agent') }}</span>
        <strong :class="overview?.agent_available ? 'is-success' : 'is-warning'">
          {{ overview?.agent_available ? t('systemSettings.dataBackup.agentReady') : t('systemSettings.dataBackup.agentMissing') }}
        </strong>
      </div>
      <div>
        <span>{{ t('systemSettings.dataBackup.latest') }}</span>
        <strong>{{ latestRecord ? statusLabel(latestRecord.status) : '-' }}</strong>
      </div>
    </div>

    <section class="backup-config-card">
      <header class="backup-card-header">
        <div>
          <div class="backup-card-title"><h3>{{ t('systemSettings.dataBackup.configTitle') }}</h3><el-tag v-if="hasConfig" size="small" type="info">{{ t('systemSettings.dataBackup.savedState') }}</el-tag></div>
          <p>{{ overview?.config.bucket || t('systemSettings.dataBackup.notConfigured') }}</p>
        </div>
        <el-button type="primary" :disabled="loading || busy || !overview" @click="openEditor">{{ t(hasConfig ? 'systemSettings.dataBackup.editConfig' : 'systemSettings.dataBackup.addConfig') }}</el-button>
      </header>
      <div v-if="hasConfig" class="backup-config-summary">
        <span>{{ t('systemSettings.dataBackup.region') }}: {{ overview?.config.region }}</span>
        <span>{{ t('systemSettings.dataBackup.prefix') }}: {{ overview?.config.prefix }}</span>
        <span>{{ t('systemSettings.dataBackup.scheduleTime') }}: {{ overview?.config.enabled ? overview.config.schedule_time : t('systemSettings.off') }}</span>
      </div>
    </section>
    <el-dialog class="backup-config-dialog" v-model="dialogVisible" :title="t(hasConfig ? 'systemSettings.dataBackup.editConfig' : 'systemSettings.dataBackup.addConfig')" width="min(860px, calc(100vw - 32px))" top="5vh" append-to-body :close-on-click-modal="false" :before-close="closeEditor">
    <el-form label-position="top" class="backup-form" :disabled="busy || loading" @submit.prevent="save()">
      <p class="backup-edit-hint">{{ t('systemSettings.dataBackup.editHint') }}</p>
      <section class="backup-form-section">
        <header class="backup-form-heading">
          <div><h4>{{ t('systemSettings.dataBackup.scheduleTitle') }}</h4><p>{{ t('systemSettings.dataBackup.scheduleDesc') }}</p></div>
          <div class="backup-enabled-control"><span>{{ t('systemSettings.dataBackup.enabled') }}</span><el-switch v-model="form.enabled" inline-prompt :active-text="t('systemSettings.on')" :inactive-text="t('systemSettings.off')" /></div>
        </header>
        <div class="backup-policy-grid">
          <el-form-item :label="t('systemSettings.dataBackup.scheduleTime')">
            <el-time-select v-model="form.schedule_time" :disabled="!form.enabled" start="00:00" step="00:30" end="23:30" />
          </el-form-item>
          <el-form-item :label="t('systemSettings.dataBackup.keepLocal')">
            <el-input-number v-model="form.keep_local" :min="1" :max="30" controls-position="right" />
          </el-form-item>
          <el-form-item :label="t('systemSettings.dataBackup.retentionDays')">
            <el-input-number v-model="form.retention_days" :min="1" :max="3650" controls-position="right" />
          </el-form-item>
        </div>
      </section>

      <section class="backup-form-section">
        <header class="backup-form-heading">
          <div><h4>{{ t('systemSettings.dataBackup.s3Destination') }}</h4><p>{{ t('systemSettings.dataBackup.s3DestinationDesc') }}</p></div>
        </header>
        <div class="backup-destination-grid">
          <el-form-item class="backup-field-wide" :label="t('systemSettings.dataBackup.endpoint')">
            <el-input v-model="form.endpoint" :placeholder="t('systemSettings.dataBackup.endpointPlaceholder')" />
            <small>{{ t('systemSettings.dataBackup.endpointHint') }}</small>
          </el-form-item>
          <el-form-item :label="t('systemSettings.dataBackup.region')"><el-input v-model="form.region" placeholder="us-east-1" /></el-form-item>
          <el-form-item :label="t('systemSettings.dataBackup.bucket')"><el-input v-model="form.bucket" placeholder="company-backups" /></el-form-item>
          <el-form-item :label="t('systemSettings.dataBackup.prefix')"><el-input v-model="form.prefix" placeholder="kageos-backups" /></el-form-item>
        </div>

        <div class="backup-subheading"><strong>{{ t('systemSettings.dataBackup.credentialsTitle') }}</strong><span>{{ t('systemSettings.dataBackup.credentialsDesc') }}</span></div>
        <div class="backup-credentials-grid">
          <el-form-item :label="t('systemSettings.dataBackup.accessKey')"><el-input v-model="form.access_key_id" autocomplete="off" /></el-form-item>
          <el-form-item :label="t('systemSettings.dataBackup.secretKey')">
            <el-input v-model="form.secret_access_key" type="password" show-password autocomplete="new-password" :placeholder="form.secret_access_key_set ? t('systemSettings.dataBackup.secretKeep') : ''" />
          </el-form-item>
        </div>
        <div class="backup-options">
          <strong>{{ t('systemSettings.dataBackup.advancedOptions') }}</strong>
          <el-checkbox v-model="form.use_ssl">{{ t('systemSettings.dataBackup.useSSL') }}</el-checkbox>
          <el-checkbox v-model="form.force_path_style">{{ t('systemSettings.dataBackup.pathStyle') }}</el-checkbox>
        </div>
      </section>

      <el-alert v-if="feedback" :title="feedback" :type="feedbackType" :closable="false" show-icon />
    </el-form>
      <template #footer>
        <div class="backup-actions">
          <span v-if="dirty" class="is-warning">{{ t('systemSettings.dataBackup.unsavedHint') }}</span>
          <el-button :disabled="busy" @click="closeEditor()">{{ t('common.cancel') }}</el-button>
          <el-button :loading="testing" :disabled="busy && !testing" @click="testConnection">{{ t('systemSettings.dataBackup.test') }}</el-button>
          <el-button type="primary" :loading="saving" :disabled="busy || (!dirty && hasConfig)" @click="save()">{{ t('userSettings.saveConfig') }}</el-button>
        </div>
      </template>
    </el-dialog>

    <section class="backup-run-card">
      <div><h4>{{ t('systemSettings.dataBackup.runNow') }}</h4><p>{{ runDisabledReason || t('systemSettings.dataBackup.runHint') }}</p></div>
      <el-button :disabled="!!runDisabledReason || busy || loading" :loading="running" @click="runNow">{{ t('systemSettings.dataBackup.runNow') }}</el-button>
    </section>

    <div class="backup-history-heading">
      <div><h4>{{ t('systemSettings.dataBackup.history') }}</h4><p>{{ t('systemSettings.dataBackup.historyHint') }}</p></div>
      <el-button text :disabled="busy || loading" @click="load(false)">{{ t('common.refresh') }}</el-button>
    </div>
    <el-table v-if="overview?.records.length" :data="overview.records" size="small">
      <el-table-column :label="t('systemSettings.dataBackup.startedAt')" min-width="170">
        <template #default="{ row }">{{ formatTime(row.started_at) }}</template>
      </el-table-column>
      <el-table-column :label="t('systemSettings.dataBackup.trigger')" width="100">
        <template #default="{ row }">{{ row.triggered_by === 'manual' ? t('systemSettings.dataBackup.manual') : t('systemSettings.dataBackup.scheduled') }}</template>
      </el-table-column>
      <el-table-column :label="t('systemSettings.dataBackup.status')" width="110">
        <template #default="{ row }"><el-tag :type="statusType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag></template>
      </el-table-column>
      <el-table-column :label="t('systemSettings.dataBackup.size')" width="120">
        <template #default="{ row }">{{ formatBytes(row.size_bytes || 0) }}</template>
      </el-table-column>
      <el-table-column :label="t('systemSettings.dataBackup.location')" min-width="240">
        <template #default="{ row }"><code v-if="row.object_key">s3://{{ row.bucket }}/{{ row.object_key }}</code><span v-else>-</span></template>
      </el-table-column>
      <el-table-column type="expand" width="44">
        <template #default="{ row }"><div class="backup-record-detail"><div>SHA256：<code>{{ row.sha256 || '-' }}</code></div><div v-if="row.error_message" class="is-error">{{ row.error_message }}</div></div></template>
      </el-table-column>
    </el-table>
    <el-empty v-else :description="t('systemSettings.dataBackup.noHistory')" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  getSystemBackupOverview,
  runSystemBackupNow,
  testSystemBackupS3,
  updateSystemBackupConfig,
  type SystemBackupConfig,
  type SystemBackupOverview,
} from '@/architecture/presentation/context/api/system-settings'

const { t } = useI18n()
const dialogVisible = ref(false)
const hasConfig = computed(() => !!overview.value?.config.bucket)
const baseline = ref('')
const feedback = ref('')
const feedbackType = ref<'success' | 'error'>('success')
const queued = ref(false)
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const running = ref(false)
const overview = ref<SystemBackupOverview | null>(null)
const form = reactive<SystemBackupConfig>({ enabled: false, schedule_time: '03:30', endpoint: '', region: 'us-east-1', bucket: '', prefix: 'kageos-backups', access_key_id: '', secret_access_key: '', secret_access_key_set: false, use_ssl: true, force_path_style: false, keep_local: 2, retention_days: 30 })
const latestRecord = computed(() => overview.value?.records[0])

const dirty = computed(() => !!baseline.value && JSON.stringify(form) !== baseline.value)
const busy = computed(() => saving.value || testing.value || running.value)
const runDisabledReason = computed(() => {
  if (!overview.value) return t('systemSettings.dataBackup.loadFailed')
  if (overview.value.running) return t('systemSettings.dataBackup.backupRunning')
  if (queued.value) return t('systemSettings.dataBackup.queued')
  if (dirty.value) return t('systemSettings.dataBackup.saveBeforeRun')
  if (!overview.value.config.enabled) return t('systemSettings.dataBackup.enableBeforeRun')
  if (!overview.value.agent_available) return t('systemSettings.dataBackup.agentRequired')
  return ''
})
function resetForm() {
  if (!overview.value) return
  Object.assign(form, overview.value.config, { secret_access_key: '' })
  baseline.value = JSON.stringify(form)
  feedback.value = ''
}
async function load(replaceForm = true) {
  if (busy.value || loading.value) return
  loading.value = true
  try {
    overview.value = await getSystemBackupOverview()
    if (replaceForm) resetForm()
    if (overview.value.running || overview.value.records[0]?.id !== lastRecordId.value) queued.value = false
  }
  catch (error: any) { ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.dataBackup.loadFailed')) }
  finally { loading.value = false }
}
const lastRecordId = ref<string>()
function openEditor() {
  resetForm()
  dialogVisible.value = true
}
async function closeEditor(done?: () => void) {
  if (busy.value) return
  if (dirty.value) {
    try {
      await ElMessageBox.confirm(t('systemSettings.dataBackup.closeUnsaved'), t('systemSettings.dataBackup.unsaved'), {
        confirmButtonText: t('systemSettings.dataBackup.discard'),
        cancelButtonText: t('systemSettings.dataBackup.keepEditing'), type: 'warning',
      })
    } catch { return }
  }
  resetForm()
  if (done) done()
  else dialogVisible.value = false
}
async function save(): Promise<boolean> {
  if (busy.value) return false
  saving.value = true
  feedback.value = ''
  try {
    overview.value = await updateSystemBackupConfig({ ...form })
    resetForm()
    dialogVisible.value = false
    ElMessage.success(t('systemSettings.dataBackup.saved'))
    return true
  }
  catch (error: any) {
    feedbackType.value = 'error'
    feedback.value = error?.response?.data?.msg || error?.message || t('systemSettings.dataBackup.saveFailed')
    return false
  }
  finally { saving.value = false }
}
async function testConnection() {
  if (busy.value) return
  testing.value = true
  try {
    await testSystemBackupS3({ ...form })
    feedbackType.value = 'success'
    feedback.value = t('systemSettings.dataBackup.testPassedUnsaved')
  }
  catch (error: any) {
    feedbackType.value = 'error'
    feedback.value = error?.response?.data?.msg || error?.message || t('systemSettings.dataBackup.testFailed')
  }
  finally { testing.value = false }
}
async function runNow() {
  if (runDisabledReason.value || busy.value) return
  running.value = true
  try {
    await ElMessageBox.confirm(t('systemSettings.dataBackup.runConfirm'), t('systemSettings.dataBackup.runNow'), { type: 'warning' })
    lastRecordId.value = overview.value?.records[0]?.id
    overview.value = await runSystemBackupNow()
    queued.value = true
    ElMessage.success(t('systemSettings.dataBackup.queued'))
  }
  catch (error: any) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.dataBackup.runFailed'))
  }
  finally { running.value = false }
}
defineExpose({ refresh: () => load(false) })
function beforeUnload(event: BeforeUnloadEvent) {
  if (dialogVisible.value && dirty.value) { event.preventDefault(); event.returnValue = '' }
}
function statusType(status: string) { return status === 'succeeded' ? 'success' : status === 'failed' ? 'danger' : 'warning' }
function statusLabel(status: string) { return t(`systemSettings.dataBackup.statuses.${status}`) }
function formatTime(value?: string) { return value ? new Date(value).toLocaleString() : '-' }
function formatBytes(value: number) { if (!value) return '-'; const units = ['B', 'KB', 'MB', 'GB', 'TB']; const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1); return `${(value / 1024 ** index).toFixed(index > 1 ? 1 : 0)} ${units[index]}` }

onMounted(() => { void load(); window.addEventListener('beforeunload', beforeUnload) })
onUnmounted(() => window.removeEventListener('beforeunload', beforeUnload))
</script>

<style scoped>
.system-backup-panel { display: grid; gap: 18px; }
.system-backup-panel :deep(.el-alert--info.is-light) { border: 1px solid rgba(var(--color-primary-rgb), .2); background: rgba(var(--color-primary-rgb), .07); }
.backup-status-row { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.backup-status-row > div { position: relative; display: grid; gap: 6px; overflow: hidden; padding: 14px 16px 14px 18px; border: 1px solid var(--app-shell-panel-border); border-radius: 10px; background: var(--app-shell-panel-bg-strong); box-shadow: inset 0 1px 0 var(--app-shell-panel-highlight); }
.backup-status-row > div::before { position: absolute; inset: 0 auto 0 0; width: 3px; background: rgba(var(--color-primary-rgb), .72); content: ''; }
.backup-status-row strong { color: var(--text-primary); }
.backup-status-row span, .backup-form small, .backup-history-heading p { color: var(--text-secondary); font-size: 12px; }
.backup-form { display: grid; gap: 14px; }
.backup-form-section { overflow: hidden; padding: 18px 18px 4px; border: 1px solid var(--app-shell-panel-border); border-radius: 10px; background: var(--app-shell-panel-bg-strong); box-shadow: inset 0 1px 0 var(--app-shell-panel-highlight); }
.backup-form-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 15px; }
.backup-form-heading h4, .backup-form-heading p { margin: 0; }.backup-form-heading h4 { color: var(--text-primary); font-size: 14px; }.backup-form-heading p { margin-top: 4px; color: var(--text-secondary); font-size: 12px; }
.backup-enabled-control { display: flex; align-items: center; gap: 10px; white-space: nowrap; }.backup-enabled-control span { color: var(--text-secondary); font-size: 12px; }
.backup-policy-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 0 16px; }
.backup-destination-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 0 16px; }.backup-field-wide { grid-column: span 2; }
.backup-credentials-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 16px; }
.backup-subheading { display: flex; align-items: baseline; gap: 10px; margin: 2px 0 12px; padding-top: 14px; border-top: 1px solid var(--border-light); }.backup-subheading strong { color: var(--text-primary); font-size: 12px; }.backup-subheading span { color: var(--text-secondary); font-size: 11px; }
.backup-form :deep(.el-form-item) { margin-bottom: 14px; }
.backup-form :deep(.el-form-item__label) { color: var(--text-regular); font-weight: 500; }
.backup-form :deep(.el-form-item__content) { display: flex; align-items: flex-start; flex-direction: column; }
.backup-form :deep(.el-input), .backup-form :deep(.el-select), .backup-form :deep(.el-input-number) { width: 100%; }
.backup-form :deep(.el-input__wrapper), .backup-form :deep(.el-select__wrapper) { background: var(--app-shell-panel-muted-bg); box-shadow: 0 0 0 1px var(--border-base) inset; }
.backup-form :deep(.el-input__wrapper:hover), .backup-form :deep(.el-select__wrapper:hover) { box-shadow: 0 0 0 1px rgba(var(--color-primary-rgb), .55) inset; }
.backup-form :deep(.el-input__wrapper.is-focus), .backup-form :deep(.el-select__wrapper.is-focused) { box-shadow: 0 0 0 1px var(--color-primary) inset, 0 0 0 3px rgba(var(--color-primary-rgb), .12); }
.backup-options { display: flex; gap: 16px; align-items: center; min-height: 46px; margin: 2px -18px 0; padding: 0 18px; border-top: 1px solid var(--app-shell-panel-border); background: rgba(var(--color-primary-rgb), .045); }.backup-options strong { margin-right: auto; color: var(--text-primary); font-size: 12px; }
.backup-actions { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; padding: 2px 0 4px; }
.backup-history-heading { display: flex; justify-content: space-between; align-items: center; }
.backup-history-heading h4, .backup-history-heading p { margin: 0; }
.backup-history-heading p { margin-top: 4px; }
.backup-record-detail { display: grid; gap: 8px; padding: 4px 16px 12px 48px; word-break: break-all; }
.is-success { color: var(--el-color-success); }.is-warning { color: var(--el-color-warning); }.is-error { color: var(--el-color-danger); }
@media (max-width: 900px) { .backup-status-row, .backup-policy-grid, .backup-destination-grid, .backup-credentials-grid { grid-template-columns: 1fr; }.backup-field-wide { grid-column: auto; } }
@media (max-width: 620px) { .backup-form-heading, .backup-options { align-items: flex-start; flex-direction: column; }.backup-actions { grid-template-columns: 1fr; }.backup-actions span { width: 100%; }.backup-actions :deep(.el-button) { width: 100%; margin-left: 0; } }
.backup-config-card { border: 1px solid var(--border-light); border-radius: 12px; background: var(--bg-primary); overflow: hidden; }
.backup-card-header, .backup-run-card { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 20px; }
.backup-card-title { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.backup-card-title h3, .backup-run-card h4 { margin: 0; font-size: 15px; }
.backup-card-header p, .backup-run-card p { margin: 6px 0 0; color: var(--text-secondary); font-size: 12px; overflow-wrap: anywhere; }

.backup-edit-hint { color: var(--text-secondary); font-size: 12px; margin: 0; }
.backup-actions span { margin-left: auto; font-size: 12px; color: var(--text-secondary); }
.backup-actions span.is-warning { color: var(--el-color-warning); }
.backup-run-card { border: 1px solid var(--border-light); border-radius: 12px; }
@media (max-width: 620px) { .backup-card-header, .backup-run-card { flex-wrap: wrap; padding: 16px; }.backup-config-card .backup-form { padding: 0 16px 16px; } }
.backup-config-summary { display: flex; flex-wrap: wrap; gap: 12px 24px; padding: 0 20px 20px; color: var(--text-secondary); font-size: 12px; }
.backup-actions { justify-content: flex-end; }
.backup-actions span { margin-right: auto; margin-left: 0; }
:global(.backup-config-dialog) { max-height: 90vh; display: flex; flex-direction: column; }
:global(.backup-config-dialog .el-dialog__body) { overflow-y: auto; min-height: 0; }
:global(.backup-config-dialog .el-dialog__footer) { flex-shrink: 0; border-top: 1px solid var(--border-light); }
</style>
