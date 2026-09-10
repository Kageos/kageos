<template>
  <div v-loading="loading" class="storage-assets-panel">
    <div class="asset-topbar">
      <el-alert
        :title="t('systemSettings.resources.assets.coverageTitle')"
        :description="t('systemSettings.resources.assets.coverageDesc')"
        type="info"
        :closable="false"
        show-icon
      />
    </div>

    <template v-if="result?.metadata_available">
      <div class="asset-summary">
        <article><span>{{ t('systemSettings.resources.assets.activeFiles') }}</span><strong>{{ formatCount(result.summary.active_files) }}</strong><small>{{ t('systemSettings.resources.assets.trackedFiles') }}</small></article>
        <article><span>{{ t('systemSettings.resources.assets.activeSize') }}</span><strong>{{ formatBytes(result.summary.active_bytes) }}</strong><small>{{ t('systemSettings.resources.assets.physicalObjectSize') }}</small></article>
        <article><span>{{ t('systemSettings.resources.assets.workspaceCount') }}</span><strong>{{ formatCount(assetWorkspaceCount) }}</strong><small>{{ selectedWorkspaceLabel }}</small></article>
        <article><span>{{ t('systemSettings.resources.assets.auditExceptions') }}</span><strong :class="{ danger: result.summary.failed_files > 0 }">{{ formatCount(result.summary.failed_files) }}</strong><small>{{ t('systemSettings.resources.assets.deletedFiles', { count: formatCount(result.summary.deleted_files) }) }}</small></article>
      </div>

      <section v-if="workspaceUsage.length" class="workspace-usage-card">
        <div class="workspace-usage-heading">
          <div><h4>{{ t('systemSettings.resources.assets.workspaceUsageTitle') }}</h4><p>{{ t('systemSettings.resources.assets.workspaceUsageDesc') }}</p></div>
          <span>{{ t('systemSettings.resources.assets.workspaceUsageTotal', { count: formatCount(workspaceUsage.length) }) }}</span>
        </div>
        <div class="workspace-usage-list">
          <button v-for="item in pagedWorkspaceUsage" :key="item.path" type="button" :class="{ selected: workspacePath === item.path }" @click="selectWorkspaceUsage(item.path)">
            <span class="workspace-usage-main"><strong>{{ workspaceName(item.path) }}</strong><small>/{{ item.path }}</small></span>
            <span class="workspace-usage-metric"><strong>{{ formatBytes(item.size_bytes) }}</strong><small>{{ t('systemSettings.resources.assets.workspaceFiles', { count: formatCount(item.file_count) }) }}</small></span>
            <span class="workspace-usage-track"><i :style="{ width: `${workspaceUsagePercent(item.size_bytes)}%` }" /></span>
          </button>
        </div>
        <footer v-if="workspaceUsage.length > workspaceUsagePageSize" class="workspace-usage-footer">
          <el-pagination v-model:current-page="workspaceUsagePage" small background layout="prev, pager, next" :page-size="workspaceUsagePageSize" :total="workspaceUsage.length" />
        </footer>
      </section>

      <section class="asset-table-card">
        <div class="asset-filters">
          <el-select v-model="workspacePath" clearable filterable :placeholder="t('systemSettings.resources.assets.allWorkspaces')" @change="handleWorkspaceChange">
            <el-option v-for="workspace in workspaceOptions" :key="workspace.path" :label="workspace.label" :value="workspace.path" />
          </el-select>
          <el-select v-model="directoryPath" clearable filterable :placeholder="t('systemSettings.resources.assets.allDirectories')" @change="resetAndLoad">
            <el-option v-for="directory in serviceDirectoryOptions" :key="directory.path" :label="directoryLabel(directory)" :value="directory.path" />
          </el-select>
          <el-select v-model="status" @change="resetAndLoad">
            <el-option :label="t('systemSettings.resources.assets.statusAll')" value="all" />
            <el-option :label="t('systemSettings.resources.assets.statusCompleted')" value="completed" />
            <el-option :label="t('systemSettings.resources.assets.statusDeleted')" value="deleted" />
            <el-option :label="t('systemSettings.resources.assets.statusFailed')" value="failed_all" />
          </el-select>
          <el-input v-model="keyword" clearable :placeholder="t('systemSettings.resources.assets.searchPlaceholder')" @clear="resetAndLoad" @keyup.enter="resetAndLoad" />
          <el-button :icon="Refresh" @click="load">{{ t('systemSettings.resources.assets.refresh') }}</el-button>
        </div>

        <div class="asset-view-heading">
          <h4>{{ t('systemSettings.resources.assets.file') }} <span>{{ result.total.toLocaleString() }}</span></h4>
          <el-radio-group v-model="assetView" size="small">
            <el-radio-button value="list">{{ t('systemSettings.resources.dashboard.filesList') }}</el-radio-button>
            <el-radio-button value="grid">{{ t('systemSettings.resources.dashboard.filesGrid') }}</el-radio-button>
          </el-radio-group>
        </div>
        <div v-if="assetView === 'grid' && result.list.length" class="asset-gallery">
          <article v-for="asset in result.list" :key="asset.ref" class="gallery-file">
            <button class="gallery-cover" :aria-label="asset.file_name" @click="asset.previewable && asset.status === 'completed' ? preview(asset) : openDetail(asset)">
              <el-image v-if="asset.thumbnail_url" :src="asset.thumbnail_url" fit="cover"><template #error><component :is="fileIcon(asset)" /></template></el-image>
              <component :is="fileIcon(asset)" v-else />
              <el-tag size="small" :type="statusType(asset.status)">{{ statusLabel(asset.status) }}</el-tag>
            </button>
            <div class="gallery-copy">
              <button class="file-name-button" :title="asset.file_name" @click="openDetail(asset)">{{ asset.file_name }}</button>
              <small>{{ formatBytes(asset.file_size) }} · {{ workspaceName(workspacePathFromRouter(asset.router)) }}</small>
              <div class="gallery-footer"><span>{{ formatTime(asset.uploaded_at) }}</span><el-button v-if="asset.status === 'completed'" link :icon="Download" :aria-label="t('systemSettings.resources.assets.download')" @click="download(asset)" /></div>
            </div>
          </article>
        </div>
        <el-empty v-else-if="assetView === 'grid'" />
        <el-table v-if="assetView === 'list'" :data="result.list" size="small" stripe class="asset-table" @row-dblclick="openDetail">
          <el-table-column :label="t('systemSettings.resources.assets.file')" min-width="220">
            <template #default="{ row }">
              <div class="file-cell">
                <button type="button" class="asset-cover" :class="{ previewable: row.previewable }" :aria-label="row.previewable ? t('systemSettings.resources.assets.previewFile', { name: row.file_name }) : row.file_name" @click="row.previewable ? preview(row) : openDetail(row)">
                  <el-image v-if="row.thumbnail_url" :src="row.thumbnail_url" fit="cover" class="asset-cover-image">
                    <template #error><component :is="fileIcon(row)" /></template>
                  </el-image>
                  <component :is="fileIcon(row)" v-else />
                  <span v-if="row.previewable" class="cover-hover"><View /></span>
                </button>
                <div class="file-copy">
                  <button type="button" class="file-name-button" :title="row.file_name" @click="openDetail(row)">{{ row.file_name || fileNameFromKey(row.file_key) }}</button>
                  <small>{{ row.content_type || t('systemSettings.resources.assets.unknownType') }} · {{ formatBytes(row.file_size) }}</small>
                  <span v-if="row.description" class="file-description">{{ row.description }}</span>
                </div>
              </div>
            </template>
          </el-table-column>
          <el-table-column :label="t('systemSettings.resources.assets.ownership')" min-width="190">
            <template #default="{ row }">
              <strong class="plain-value">{{ workspaceName(workspacePathFromRouter(row.router)) }}</strong>
              <code class="asset-path" :title="row.router">/{{ relativeServicePath(row.router) }}</code>
              <small class="cell-subtext">/{{ workspacePathFromRouter(row.router) }}<template v-if="serviceFunctionName(row.router)"> · {{ serviceFunctionName(row.router) }}</template></small>
            </template>
          </el-table-column>
          <el-table-column :label="t('systemSettings.resources.assets.audit')" min-width="170">
            <template #default="{ row }">
              <strong class="plain-value">{{ t('systemSettings.resources.assets.auditCounts', { previews: formatCount(row.preview_count), downloads: formatCount(row.download_count) }) }}</strong>
              <small class="cell-subtext">{{ row.last_accessed_at ? formatTime(row.last_accessed_at) : t('systemSettings.resources.assets.neverAccessed') }}</small>
              <small class="cell-subtext">{{ t('systemSettings.resources.assets.uploader') }}：{{ row.username || '-' }} · {{ formatTime(row.uploaded_at) }}</small>
            </template>
          </el-table-column>
          <el-table-column fixed="right" :label="t('systemSettings.resources.assets.actions')" width="178">
            <template #default="{ row }">
              <div class="status-actions">
                <el-tag size="small" effect="plain" :type="statusType(row.status)">{{ statusLabel(row.status) }}</el-tag>
                <span>
                  <el-button v-if="row.status === 'completed' && row.previewable" link type="primary" :icon="View" @click="preview(row)">{{ t('systemSettings.resources.assets.preview') }}</el-button>
                  <el-button v-if="row.status === 'completed'" link type="primary" :icon="Download" @click="download(row)">{{ t('systemSettings.resources.assets.download') }}</el-button>
                  <el-dropdown v-if="row.status === 'completed' || row.status === 'delete_failed'" trigger="click" @command="remove(row)">
                    <el-button link :icon="MoreFilled" aria-label="更多操作" />
                    <template #dropdown><el-dropdown-menu><el-dropdown-item command="delete" :icon="Delete">{{ t('systemSettings.resources.assets.delete') }}</el-dropdown-item></el-dropdown-menu></template>
                  </el-dropdown>
                  <span v-if="row.status === 'deleted'" class="deleted-by">{{ row.deleted_by || '-' }}</span>
                </span>
              </div>
            </template>
          </el-table-column>
        </el-table>

        <footer class="asset-table-footer">
          <span>{{ t('systemSettings.resources.assets.pageHint', { count: formatCount(result.total) }) }}</span>
          <el-pagination v-if="result.total > pageSize" v-model:current-page="page" background layout="prev, pager, next" :page-size="pageSize" :total="result.total" @current-change="load" />
        </footer>
      </section>
    </template>
    <el-empty v-else-if="result" :description="t('systemSettings.resources.assets.metadataUnavailable')" />

    <el-dialog v-model="previewVisible" :title="previewAsset?.file_name || t('systemSettings.resources.assets.preview')" width="min(920px, 92vw)" destroy-on-close append-to-body>
      <div v-loading="previewLoading" class="asset-preview-stage">
        <img v-if="previewAsset?.preview_kind === 'image' && previewURL" :src="previewURL" :alt="previewAsset.file_name" />
        <video v-else-if="previewAsset?.preview_kind === 'video' && previewURL" :src="previewURL" controls playsinline />
        <iframe v-else-if="previewAsset?.preview_kind === 'pdf' && previewURL" :src="previewURL" :title="previewAsset.file_name" />
        <el-empty v-else-if="!previewLoading" :description="t('systemSettings.resources.assets.previewUnavailable')" />
      </div>
      <template #footer><el-button @click="previewVisible = false">{{ t('common.close') }}</el-button><el-button v-if="previewAsset" type="primary" :icon="Download" @click="download(previewAsset)">{{ t('systemSettings.resources.assets.download') }}</el-button></template>
    </el-dialog>

    <el-drawer v-model="detailVisible" :title="detailAsset?.file_name || t('systemSettings.resources.assets.fileDetail')" size="min(720px, 92vw)" append-to-body destroy-on-close>
      <template v-if="detailAsset">
        <div class="detail-overview">
          <button v-if="detailAsset.previewable" type="button" class="detail-cover" @click="preview(detailAsset)">
            <el-image v-if="detailAsset.thumbnail_url" :src="detailAsset.thumbnail_url" fit="cover" /><component :is="fileIcon(detailAsset)" v-else />
            <span><View /> {{ t('systemSettings.resources.assets.preview') }}</span>
          </button>
          <div class="detail-metadata">
            <div><span>{{ t('systemSettings.resources.assets.workspace') }}</span><strong>{{ workspaceName(workspacePathFromRouter(detailAsset.router)) }}</strong></div>
            <div><span>{{ t('systemSettings.resources.assets.serviceDirectory') }}</span><code>/{{ relativeServicePath(detailAsset.router) }}</code></div>
            <div><span>{{ t('systemSettings.resources.assets.objectRef') }}</span><code>{{ detailAsset.ref }}</code></div>
            <div><span>{{ t('systemSettings.resources.assets.uploader') }}</span><strong>{{ detailAsset.username || '-' }}</strong></div>
            <div><span>{{ t('systemSettings.resources.assets.sizeAndType') }}</span><strong>{{ formatBytes(detailAsset.file_size) }} · {{ detailAsset.content_type || '-' }}</strong></div>
            <div><span>{{ t('systemSettings.resources.assets.uploadedAt') }}</span><strong>{{ formatTime(detailAsset.uploaded_at) }}</strong></div>
          </div>
        </div>
        <div class="detail-actions"><el-button v-if="detailAsset.previewable && detailAsset.status === 'completed'" :icon="View" @click="preview(detailAsset)">{{ t('systemSettings.resources.assets.preview') }}</el-button><el-button v-if="detailAsset.status === 'completed'" type="primary" :icon="Download" @click="download(detailAsset)">{{ t('systemSettings.resources.assets.download') }}</el-button></div>
        <section class="audit-history">
          <div class="audit-heading"><div><h4>{{ t('systemSettings.resources.assets.accessHistory') }}</h4><p>{{ t('systemSettings.resources.assets.accessHistoryDesc') }}</p></div><el-button :icon="Refresh" link @click="loadAudits(detailAsset)">{{ t('common.refresh') }}</el-button></div>
          <el-table v-loading="auditsLoading" :data="audits" size="small">
            <el-table-column :label="t('systemSettings.resources.assets.auditAction')" width="100"><template #default="{ row }"><el-tag size="small" effect="plain">{{ auditActionLabel(row.action) }}</el-tag></template></el-table-column>
            <el-table-column prop="username" :label="t('systemSettings.resources.assets.operator')" min-width="110" />
            <el-table-column prop="ip_address" label="IP" min-width="125" />
            <el-table-column :label="t('systemSettings.resources.assets.accessedAt')" min-width="160"><template #default="{ row }">{{ formatTime(row.accessed_at) }}</template></el-table-column>
          </el-table>
          <el-empty v-if="!auditsLoading && !audits.length" :description="t('systemSettings.resources.assets.noAuditHistory')" :image-size="64" />
        </section>
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Document, Download, MoreFilled, Picture, Refresh, VideoCamera, View } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import { getAppList } from '@/architecture/presentation/context/api/app'
import type { App } from '@/architecture/domain/types'
import {
  deleteFileRefs, getSystemStorageAssetAccessURL, listSystemStorageAssetAudits, listSystemStorageAssets,
  type SystemStorageAsset, type SystemStorageAssetAudit, type SystemStorageAssetsResp,
} from '@/architecture/presentation/context/api/storage'

const { t } = useI18n()
const loading = ref(false)
const assetView = ref('list')
const page = ref(1)
const pageSize = 20
const workspacePath = ref('')
const directoryPath = ref('')
const status = ref('completed')
const keyword = ref('')
const result = ref<SystemStorageAssetsResp | null>(null)
const workspaceUsagePage = ref(1)
const workspaceUsagePageSize = 6
const workspaces = ref<App[]>([])
const previewVisible = ref(false)
const previewLoading = ref(false)
const previewURL = ref('')
const previewAsset = ref<SystemStorageAsset | null>(null)
const detailVisible = ref(false)
const detailAsset = ref<SystemStorageAsset | null>(null)
const auditsLoading = ref(false)
const audits = ref<SystemStorageAssetAudit[]>([])

const derivedWorkspacePaths = computed(() => [...new Set([
  ...(result.value?.workspaces || []).map(item => item.path),
  ...(result.value?.directories || []).map(item => workspacePathFromRouter(item.router)),
].filter(Boolean))])
const derivedWorkspaceUsage = computed(() => {
  const grouped = new Map<string, { path: string; file_count: number; size_bytes: number }>()
  for (const item of result.value?.directories || []) {
    const path = workspacePathFromRouter(item.router)
    if (!path) continue
    const current = grouped.get(path) || { path, file_count: 0, size_bytes: 0 }
    current.file_count += item.file_count
    current.size_bytes += item.size_bytes
    grouped.set(path, current)
  }
  return [...grouped.values()].sort((a, b) => b.size_bytes - a.size_bytes || a.path.localeCompare(b.path))
})
const workspaceUsage = computed(() => result.value?.workspaces?.length ? result.value.workspaces : derivedWorkspaceUsage.value)
const assetWorkspaceCount = computed(() => workspaceUsage.value.length)
const pagedWorkspaceUsage = computed(() => workspaceUsage.value.slice((workspaceUsagePage.value - 1) * workspaceUsagePageSize, workspaceUsagePage.value * workspaceUsagePageSize))
const workspaceUsagePeak = computed(() => Math.max(1, ...workspaceUsage.value.map(item => item.size_bytes)))
const workspaceOptions = computed(() => {
  const map = new Map<string, { path: string; label: string }>()
  for (const app of workspaces.value) {
    const path = `${app.user}/${app.code}`
    map.set(path, { path, label: `${app.name || app.code} · /${path}` })
  }
  for (const path of derivedWorkspacePaths.value) if (!map.has(path)) map.set(path, { path, label: `${t('systemSettings.resources.assets.unregisteredWorkspace')} · /${path}` })
  return [...map.values()].sort((a, b) => a.label.localeCompare(b.label))
})
const workspaceNameMap = computed(() => new Map(workspaceOptions.value.map(item => [item.path, item.label.split(' · ')[0] || item.path])))
const directoryOptions = computed(() => {
  const grouped = new Map<string, { path: string; file_count: number; size_bytes: number }>()
  for (const item of result.value?.directories || []) {
    const path = serviceDirectoryPath(item.router)
    if (workspacePath.value && path !== workspacePath.value && !path.startsWith(`${workspacePath.value}/`)) continue
    const current = grouped.get(path) || { path, file_count: 0, size_bytes: 0 }
    current.file_count += item.file_count
    current.size_bytes += item.size_bytes
    grouped.set(path, current)
  }
  return [...grouped.values()].sort((a, b) => b.size_bytes - a.size_bytes)
})
const serviceDirectoryOptions = computed(() => directoryOptions.value.filter(item => item.path !== workspacePath.value))
const selectedWorkspaceLabel = computed(() => workspacePath.value ? workspaceName(workspacePath.value) : t('systemSettings.resources.assets.allWorkspaces'))

async function load() {
  loading.value = true
  try {
    result.value = await listSystemStorageAssets({ page: page.value, page_size: pageSize, router_prefix: directoryPath.value || workspacePath.value || undefined, status: status.value, keyword: keyword.value.trim() || undefined })
  } catch (error: any) { ElMessage.error(error?.message || t('systemSettings.resources.assets.loadFailed')) }
  finally { loading.value = false }
}
async function loadWorkspaces() { try { workspaces.value = await getAppList(500, undefined, true) } catch { workspaces.value = [] } }
function resetAndLoad() { page.value = 1; void load() }
function handleWorkspaceChange() { directoryPath.value = ''; resetAndLoad() }
function selectWorkspaceUsage(path: string) { workspacePath.value = workspacePath.value === path ? '' : path; handleWorkspaceChange() }
function workspaceUsagePercent(value: number) { return Math.max(value > 0 ? 3 : 0, Math.min(100, value / workspaceUsagePeak.value * 100)) }
function directoryLabel(item: { path: string; file_count: number; size_bytes: number }) { return `${workspacePath.value ? relativeToWorkspace(item.path) : `/${item.path}`} · ${formatCount(item.file_count)} · ${formatBytes(item.size_bytes)}` }
function workspacePathFromRouter(router: string) { return String(router || '').replace(/^\/+|\/+$/g, '').split('/').filter(Boolean).slice(0, 2).join('/') }
function serviceDirectoryPath(router: string) { const parts = String(router || '').replace(/^\/+|\/+$/g, '').split('/').filter(Boolean); if (parts.length > 2 && /\.(form|table|chart)$/i.test(parts.at(-1) || '')) parts.pop(); return parts.join('/') }
function relativeToWorkspace(path: string) { const prefix = workspacePath.value ? `${workspacePath.value}/` : ''; return `/${prefix && path.startsWith(prefix) ? path.slice(prefix.length) : path}` }
function relativeServicePath(router: string) { const workspace = workspacePathFromRouter(router); const directory = serviceDirectoryPath(router); return directory.startsWith(`${workspace}/`) ? directory.slice(workspace.length + 1) : directory }
function serviceFunctionName(router: string) { const last = String(router || '').replace(/\/+$/g, '').split('/').at(-1) || ''; return /\.(form|table|chart)$/i.test(last) ? last : '' }
function workspaceName(path: string) { return workspaceNameMap.value.get(path) || path || '-' }
function fileNameFromKey(key: string) { return key.split('/').pop() || key }
function fileIcon(row: SystemStorageAsset) { return row.preview_kind === 'image' ? Picture : row.preview_kind === 'video' ? VideoCamera : Document }
function formatCount(value?: number | string) { return Number(value || 0).toLocaleString() }
function formatBytes(value: number) { if (!value) return '0 B'; const units = ['B', 'KB', 'MB', 'GB', 'TB']; const index = Math.min(units.length - 1, Math.floor(Math.log(value) / Math.log(1024))); return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}` }
function formatTime(value: string) { return new Date(value).toLocaleString() }
function statusType(value: string) { return value === 'completed' ? 'success' : value === 'deleted' ? 'info' : value.includes('failed') ? 'danger' : 'warning' }
function statusLabel(value: string) { const key = value === 'completed' ? 'statusCompleted' : value === 'deleted' ? 'statusDeleted' : value.includes('failed') ? 'statusFailed' : 'statusPending'; return t(`systemSettings.resources.assets.${key}`) }
function auditActionLabel(action: string) { return t(`systemSettings.resources.assets.${action === 'preview' ? 'auditPreview' : 'auditDownload'}`) }

async function preview(row: SystemStorageAsset) {
  previewAsset.value = row; previewURL.value = ''; previewVisible.value = true; previewLoading.value = true
  try { const response = await getSystemStorageAssetAccessURL(row.ref, 'preview'); previewURL.value = response.url; row.preview_count += 1; if (detailAsset.value?.ref === row.ref) void loadAudits(row) }
  catch (error: any) { ElMessage.error(error?.message || t('systemSettings.resources.assets.previewFailed')) }
  finally { previewLoading.value = false }
}
async function download(row: SystemStorageAsset) {
  try { const response = await getSystemStorageAssetAccessURL(row.ref, 'download'); window.open(response.url, '_blank', 'noopener,noreferrer'); row.download_count += 1; if (detailAsset.value?.ref === row.ref) void loadAudits(row) }
  catch (error: any) { ElMessage.error(error?.message || t('systemSettings.resources.assets.downloadFailed')) }
}
async function openDetail(row: SystemStorageAsset) { detailAsset.value = row; detailVisible.value = true; await loadAudits(row) }
async function loadAudits(row: SystemStorageAsset) { auditsLoading.value = true; try { audits.value = (await listSystemStorageAssetAudits(row.ref, 30)).list || [] } catch { audits.value = [] } finally { auditsLoading.value = false } }
async function remove(row: SystemStorageAsset) {
  try {
    await ElMessageBox.confirm(t('systemSettings.resources.assets.deleteConfirmDesc', { name: row.file_name, path: `/${row.router}` }), t('systemSettings.resources.assets.deleteConfirmTitle'), { type: 'warning', confirmButtonText: t('systemSettings.resources.assets.confirmDelete'), cancelButtonText: t('common.cancel') })
    const response = await deleteFileRefs([row.ref]); const item = response.results[0]; if (!item || item.status === 'failed') throw new Error(item?.error || t('systemSettings.resources.assets.deleteFailed'))
    ElMessage.success(t('systemSettings.resources.assets.deleteSucceeded', { size: formatBytes(item.released_bytes || row.file_size) })); detailVisible.value = false; await load()
  } catch (error: any) { if (error === 'cancel' || error === 'close') return; ElMessage.error(error?.message || t('systemSettings.resources.assets.deleteFailed')) }
}

onMounted(() => { void Promise.all([load(), loadWorkspaces()]) })
</script>

<style scoped>
.storage-assets-panel { display: grid; gap: 16px; }
.asset-topbar { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 12px; }
.asset-summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.asset-summary article { display: grid; gap: 5px; min-width: 0; padding: 15px 16px; border: 1px solid var(--border-light); border-radius: var(--border-radius-lg); background: var(--bg-tertiary); }
.asset-summary span, .asset-summary small { overflow: hidden; color: var(--text-secondary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.asset-summary strong { color: var(--text-primary); font-size: 22px; }.asset-summary strong.danger { color: var(--el-color-danger); }
.workspace-usage-card { overflow: hidden; border: 1px solid var(--border-light); border-radius: var(--border-radius-lg); background: var(--bg-tertiary); }
.workspace-usage-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 14px 16px 12px; border-bottom: 1px solid var(--border-light); }
.workspace-usage-heading h4, .workspace-usage-heading p { margin: 0; }.workspace-usage-heading h4 { color: var(--text-primary); font-size: 14px; }.workspace-usage-heading p, .workspace-usage-heading > span { margin-top: 4px; color: var(--text-secondary); font-size: 12px; }
.workspace-usage-list { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
.workspace-usage-list button { position: relative; display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 3px 16px; min-width: 0; padding: 13px 16px 16px; border: 0; border-bottom: 1px solid var(--border-light); background: transparent; color: inherit; text-align: left; cursor: pointer; }
.workspace-usage-list button:nth-child(odd) { border-right: 1px solid var(--border-light); }.workspace-usage-list button:hover { background: rgba(129, 140, 248, .08); }.workspace-usage-list button.selected { background: rgba(129, 140, 248, .14); box-shadow: inset 3px 0 0 rgba(129, 140, 248, .82); }
.workspace-usage-main, .workspace-usage-metric { display: grid; min-width: 0; }.workspace-usage-main strong, .workspace-usage-main small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.workspace-usage-main strong, .workspace-usage-metric strong { color: var(--text-primary); font-size: 13px; }.workspace-usage-main small, .workspace-usage-metric small { margin-top: 3px; color: var(--text-secondary); font-size: 11px; }.workspace-usage-metric { justify-items: end; }
.workspace-usage-track { grid-column: 1 / -1; height: 3px; margin-top: 7px; overflow: hidden; border-radius: 999px; background: var(--bg-secondary); }.workspace-usage-track i { display: block; height: 100%; border-radius: inherit; background: var(--el-color-primary); }
.workspace-usage-footer { display: flex; justify-content: flex-end; padding: 8px 12px; }
.asset-table-card { overflow: hidden; border: 1px solid var(--border-light); border-radius: var(--border-radius-lg); background: var(--bg-tertiary); }
.asset-filters { display: grid; grid-template-columns: minmax(190px, 1fr) minmax(210px, 1.15fr) 120px minmax(200px, 1fr) auto; gap: 10px; padding: 14px; border-bottom: 1px solid var(--border-light); }
.asset-table { width: 100%; }.file-cell { display: flex; align-items: center; gap: 11px; min-width: 0; }
.asset-cover { position: relative; display: grid; flex: 0 0 46px; width: 46px; height: 46px; overflow: hidden; place-items: center; padding: 0; border: 1px solid var(--border-light); border-radius: 9px; background: var(--bg-secondary); color: var(--text-secondary); }
.asset-cover.previewable { cursor: pointer; }.asset-cover > :deep(svg) { width: 20px; }.asset-cover-image { width: 100%; height: 100%; }.asset-cover-image :deep(.el-image__inner) { width: 100%; height: 100%; }
.cover-hover { position: absolute; inset: 0; display: grid; place-items: center; background: rgba(14, 18, 31, .65); color: white; opacity: 0; transition: opacity .15s ease; }.cover-hover :deep(svg) { width: 16px; }.asset-cover:hover .cover-hover, .asset-cover:focus-visible .cover-hover { opacity: 1; }
.file-copy { display: grid; min-width: 0; gap: 3px; }.file-name-button { overflow: hidden; padding: 0; border: 0; background: transparent; color: var(--el-color-primary); font: inherit; font-size: 13px; font-weight: 600; text-align: left; text-overflow: ellipsis; white-space: nowrap; cursor: pointer; }.file-copy small, .cell-subtext { color: var(--text-secondary); font-size: 11px; }.file-description { overflow: hidden; color: var(--text-secondary); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.plain-value, .cell-subtext { display: block; }.asset-path { display: block; overflow: hidden; color: var(--text-secondary); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }.deleted-by { color: var(--text-secondary); font-size: 12px; }
.status-actions { display: grid; justify-items: start; gap: 5px; }.status-actions > span { display: flex; align-items: center; white-space: nowrap; }
.asset-table-footer { display: flex; align-items: center; justify-content: space-between; min-height: 48px; padding: 0 14px; border-top: 1px solid var(--border-light); color: var(--text-secondary); font-size: 12px; }
.asset-preview-stage { display: grid; min-height: 420px; max-height: 70vh; place-items: center; overflow: hidden; border-radius: 10px; background: #0d101a; }.asset-preview-stage img, .asset-preview-stage video { max-width: 100%; max-height: 70vh; object-fit: contain; }.asset-preview-stage iframe { width: 100%; height: 68vh; border: 0; background: white; }
.detail-overview { display: grid; grid-template-columns: 160px minmax(0, 1fr); gap: 18px; }.detail-cover { position: relative; display: grid; width: 160px; height: 130px; overflow: hidden; place-items: center; border: 1px solid var(--border-light); border-radius: 10px; background: var(--bg-tertiary); color: var(--text-secondary); cursor: pointer; }.detail-cover :deep(.el-image) { width: 100%; height: 100%; }.detail-cover > :deep(svg) { width: 32px; }.detail-cover > span { position: absolute; inset: auto 0 0; display: flex; align-items: center; justify-content: center; gap: 5px; padding: 7px; background: rgba(14, 18, 31, .76); color: white; font-size: 12px; }.detail-cover > span :deep(svg) { width: 14px; }
.detail-metadata { display: grid; gap: 10px; }.detail-metadata div { display: grid; grid-template-columns: 92px minmax(0, 1fr); gap: 10px; }.detail-metadata span { color: var(--text-secondary); font-size: 12px; }.detail-metadata strong, .detail-metadata code { overflow-wrap: anywhere; color: var(--text-primary); font-size: 12px; }.detail-actions { display: flex; justify-content: flex-end; gap: 8px; margin: 16px 0 22px; }
.audit-history { padding-top: 18px; border-top: 1px solid var(--border-light); }.audit-heading { display: flex; align-items: flex-start; justify-content: space-between; margin-bottom: 12px; }.audit-heading h4, .audit-heading p { margin: 0; }.audit-heading h4 { color: var(--text-primary); font-size: 14px; }.audit-heading p { margin-top: 4px; color: var(--text-secondary); font-size: 12px; }
@media (max-width: 1150px) { .asset-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }.asset-filters { grid-template-columns: 1fr 1fr 120px; } }
@media (max-width: 760px) { .asset-topbar, .asset-summary, .asset-filters, .detail-overview, .workspace-usage-list { grid-template-columns: 1fr; }.workspace-usage-list button:nth-child(odd) { border-right: 0; }.workspace-usage-heading { flex-direction: column; }.detail-cover { width: 100%; }.asset-table-footer { align-items: flex-start; flex-direction: column; gap: 8px; padding-block: 10px; } }

.asset-view-heading { display:flex; align-items:center; justify-content:space-between; gap:12px; padding:20px 0 16px; }
.asset-view-heading h4 { margin:0; font-size:16px; color:var(--text-primary); }
.asset-view-heading h4 span { margin-left:8px; font-size:12px; font-weight:500; color:var(--text-secondary); }
.asset-gallery { display:grid; grid-template-columns:repeat(auto-fill,minmax(210px,1fr)); gap:18px; }
.gallery-file { min-width:0; overflow:hidden; border:1px solid var(--border-light); border-radius:14px; background:var(--bg-primary); transition:box-shadow .2s,transform .2s; }
.gallery-file:hover { transform:translateY(-2px); box-shadow:0 8px 24px rgba(20,35,60,.08); }
.gallery-cover { position:relative; border:0; width:100%; height:148px; display:grid; place-items:center; padding:0; background:var(--bg-tertiary); color:var(--text-secondary); cursor:pointer; }
.gallery-cover > svg { width:44px; height:44px; opacity:.65; }
.gallery-cover .el-image { width:100%; height:100%; }
.gallery-cover .el-tag { position:absolute; top:10px; right:10px; }
.gallery-copy { display:grid; gap:8px; padding:16px; }
.gallery-copy > small { color:var(--text-secondary); font-size:11px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.gallery-footer { display:flex; align-items:center; justify-content:space-between; color:var(--text-secondary); font-size:10px; }
.asset-summary { gap:16px; }
.asset-summary article { padding:22px; border-radius:14px; background:var(--bg-primary); border-top:3px solid var(--el-color-primary); gap:10px; }
.asset-summary article:nth-child(2) { border-top-color:#35a89a; }
.asset-summary article:nth-child(3) { border-top-color:#9a83da; }
.asset-summary article:nth-child(4) { border-top-color:#d49b4f; }
.asset-summary strong { font-size:30px; font-variant-numeric:tabular-nums; letter-spacing:-.8px; }
.workspace-usage-card { border-radius:14px; background:var(--bg-primary); }
.asset-table :deep(.el-table__cell) { padding:16px 0; }
.asset-topbar :deep(.el-alert) { background:var(--bg-tertiary); }
@media (prefers-reduced-motion:reduce) { .gallery-file { transition:none; } .gallery-file:hover { transform:none; } }
@media (max-width:760px) { .asset-summary { grid-template-columns:repeat(2,minmax(0,1fr)); gap:10px; } .asset-summary article { padding:16px; } .asset-summary strong { font-size:26px; } }
</style>
