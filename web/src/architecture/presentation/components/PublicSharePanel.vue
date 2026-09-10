<template>
  <section class="public-share-panel">
    <div class="history-card">
      <div class="section-header">
        <div class="section-heading">
          <div class="section-title">{{ t('publicSharePanel.title') }}</div>
          <div class="section-subtitle">{{ t('publicSharePanel.subtitle') }}</div>
        </div>
        <el-button type="primary" size="small" @click="openCreateDialog">{{ t('publicSharePanel.createShare') }}</el-button>
      </div>

      <div class="form-history-toolbar">
        <el-input
          v-model="filters.keyword"
          class="history-search"
          clearable
          :prefix-icon="Search"
          :placeholder="t('publicSharePanel.searchPlaceholder')"
          @keyup.enter="load"
          @clear="load"
        />
        <el-input
          v-model="filters.createdBy"
          class="history-user-select"
          clearable
          :placeholder="t('publicSharePanel.createdBy')"
          @keyup.enter="load"
          @clear="load"
        />
        <el-select
          v-model="filters.status"
          class="history-action-select"
          clearable
          :placeholder="t('publicSharePanel.status')"
          @change="load"
        >
          <el-option :label="t('publicSharePanel.statusEnabled')" value="enabled" />
          <el-option :label="t('publicSharePanel.statusDisabled')" value="disabled" />
          <el-option :label="t('publicSharePanel.statusExpired')" value="expired" />
          <el-option :label="t('shareGovernance.exhausted')" value="exhausted" />
        </el-select>
        <el-button type="primary" plain :icon="Search" @click="load">{{ t('publicSharePanel.filter') }}</el-button>
        <el-button plain :icon="Refresh" :loading="loading" @click="resetFilters">{{ t('common.reset') }}</el-button>
      </div>

      <div class="share-bulk-toolbar">
        <el-button type="danger" plain :loading="bulkClosing" :disabled="!selectedShares.length" @click="closeSelected">{{ t('shareGovernance.closeSelected', { count: selectedShares.length }) }}</el-button>
        <span>{{ t('shareGovernance.scope') }}</span>
      </div>
      <div v-loading="loading" class="mobile-share-list">
        <el-empty v-if="shares.length === 0" :description="t('publicSharePanel.empty')" :image-size="80" />
        <article v-for="row in shares" :key="row.share_id" class="mobile-share-card">
          <el-checkbox :model-value="selectedShares.some(item => item.share_id === row.share_id)" :disabled="!row.enabled || bulkClosing" @change="toggleSelected(row)">{{ t('shareGovernance.select') }}</el-checkbox>
          <div class="mobile-share-head">
            <div class="mobile-share-title">
              <div class="title-name">{{ shareDisplayTitle(row) }}</div>
              <div v-if="row.description" class="link-description">{{ row.description }}</div>
            </div>
            <el-tag size="small" :type="statusTagType(row)" effect="light" round>
              {{ statusLabel(row) }}
            </el-tag>
          </div>

          <button class="mobile-share-url" type="button" @click="copyLink(publicLink(row))">
            {{ publicLink(row) }}
          </button>

          <div class="mobile-share-meta">
            <div>
              <span>{{ t('publicSharePanel.submissions') }}</span>
              <strong>{{ row.use_count }}</strong>
              <em>{{ usageLimitText(row.max_uses) }}</em>
            </div>
            <div>
              <span>{{ t('publicSharePanel.expiration') }}</span>
              <strong>{{ row.expires_at ? expiryHint(row.expires_at) : t('publicSharePanel.permanent') }}</strong>
              <em>{{ row.expires_at ? formatDate(row.expires_at) : t('publicSharePanel.neverExpires') }}</em>
            </div>
          </div>

          <div class="mobile-share-foot">
            <span>{{ row.created_by || '-' }} · {{ formatDate(row.created_at) }}</span>
            <div class="mobile-share-actions">
              <el-button size="small" text @click="copyLink(publicLink(row))">{{ t('publicSharePanel.copy') }}</el-button>
              <el-button size="small" text @click="openQrDialog(row)">{{ t('publicSharePanel.qrCode') }}</el-button>
              <el-button
                v-if="row.enabled"
                size="small"
                text
                type="danger"
                :loading="disablingId === row.share_id"
                @click="disableShare(row.share_id)"
              >
                {{ t('publicSharePanel.close') }}
              </el-button>
            </div>
          </div>
        </article>
      </div>

      <el-table
        v-loading="loading"
        :data="shares"
        stripe
        class="history-table"
        @selection-change="selectedShares = $event"
        :empty-text="t('publicSharePanel.empty')"
      >
        <el-table-column type="selection" width="48" :selectable="(row: PublicShareItem) => row.enabled && !bulkClosing" />
        <el-table-column :label="t('publicSharePanel.shareTitle')" min-width="220">
          <template #default="{ row }">
            <div class="title-cell">
              <div class="title-name">{{ shareDisplayTitle(row) }}</div>
              <div v-if="row.description" class="link-description">{{ row.description }}</div>
            </div>
          </template>
        </el-table-column>

        <el-table-column :label="t('publicSharePanel.publicLink')" min-width="260">
          <template #default="{ row }">
            <button class="url-cell" type="button" @click="copyLink(publicLink(row))">
              {{ publicLink(row) }}
            </button>
          </template>
        </el-table-column>

        <el-table-column :label="t('publicSharePanel.status')" width="100" align="center">
          <template #default="{ row }">
            <el-tag size="small" :type="statusTagType(row)" effect="light" round>
              {{ statusLabel(row) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column :label="t('publicSharePanel.createdBy')" width="150">
          <template #default="{ row }">
            <span class="muted-text">{{ row.created_by || '-' }}</span>
          </template>
        </el-table-column>

        <el-table-column :label="t('publicSharePanel.submissionCount')" width="130" align="center">
          <template #default="{ row }">
            <div class="count-cell">
              <div>{{ row.use_count }}</div>
              <span>{{ usageLimitText(row.max_uses) }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column :label="t('publicSharePanel.expirationTime')" width="190">
          <template #default="{ row }">
            <div class="time-cell">
              <div>{{ row.expires_at ? formatDate(row.expires_at) : t('publicSharePanel.permanent') }}</div>
              <span>{{ row.expires_at ? expiryHint(row.expires_at) : t('publicSharePanel.neverExpires') }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column :label="t('publicSharePanel.createdAt')" width="180">
          <template #default="{ row }">
            <div class="time-cell">
              <div>{{ formatDate(row.created_at) }}</div>
            </div>
          </template>
        </el-table-column>

        <el-table-column :label="t('common.operation')" width="190" align="right" fixed="right">
          <template #default="{ row }">
            <div class="action-cell">
              <el-button text @click="copyLink(publicLink(row))">{{ t('publicSharePanel.copy') }}</el-button>
              <el-button text @click="openQrDialog(row)">{{ t('publicSharePanel.qrCode') }}</el-button>
              <el-button
                v-if="row.enabled"
                text
                type="danger"
                :loading="disablingId === row.share_id"
                @click="disableShare(row.share_id)"
              >
                {{ t('publicSharePanel.close') }}
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <PublicShareCreateDialog
      v-model="dialogVisible"
      :full-code-path="fullCodePath"
      :default-title="functionDetail?.name || functionNode?.name || ''"
      @created="handleShareCreated"
    />

    <el-dialog
      v-model="qrDialogVisible"
      :title="t('publicSharePanel.qrDialogTitle')"
      :width="qrDialogWidth"
      class="public-share-qr-dialog"
    >
        <div class="qr-dialog-body">
          <div class="qr-title">{{ qrShare ? shareDisplayTitle(qrShare) : t('publicSharePanel.untitled') }}</div>
          <div v-if="qrShare?.description" class="qr-description">{{ qrShare.description }}</div>
          <div v-if="qrShare" class="qr-meta">
            <span>{{ qrShare.expires_at ? `${t('publicSharePanel.expirationTime')}：${formatDate(qrShare.expires_at)}` : t('publicSharePanel.permanent') }}</span>
            <span>{{ qrShare.max_uses > 0 ? `${t('publicSharePanel.submissionCount')}：${qrShare.use_count}/${qrShare.max_uses}` : t('publicSharePanel.unlimited') }}</span>
          </div>
        <div class="qr-box">
          <el-skeleton v-if="qrGenerating" :rows="5" animated />
          <img v-else-if="qrDataUrl" class="qr-image" :src="qrDataUrl" :alt="t('publicSharePanel.qrAlt')" />
          <el-empty v-else :description="t('publicSharePanel.qrGenerateFailed')" :image-size="80" />
        </div>
        <div class="qr-link-group">
          <div class="qr-link-label">{{ t('publicSharePanel.scanLink') }}</div>
          <button class="qr-link" type="button" @click="copyLink(currentQrLink)">
            {{ currentQrLink }}
          </button>
        </div>
      </div>

      <template #footer>
        <div class="qr-footer-actions">
          <el-button @click="copyLink(currentQrLink)">{{ t('publicSharePanel.copyLink') }}</el-button>
          <el-button :disabled="!qrDataUrl" @click="downloadQrCode">{{ t('publicSharePanel.downloadQr') }}</el-button>
        </div>
      </template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import QRCode from 'qrcode'
import { Refresh, Search } from '@element-plus/icons-vue'
import PublicShareCreateDialog from '@/architecture/presentation/components/PublicShareCreateDialog.vue'
import {
  disablePublicShare,
  listPublicShares,
  type PublicShareItem,
} from '@/architecture/presentation/context/api/publicShare'
import type { FunctionDetail, ServiceTree } from '@/architecture/domain/types'

const props = defineProps<{
  functionDetail: FunctionDetail | null
  functionNode: ServiceTree | null
}>()

const { t } = useI18n()

const loading = ref(false)
const disablingId = ref('')
const selectedShares = ref<PublicShareItem[]>([])
const bulkClosing = ref(false)
const shares = ref<PublicShareItem[]>([])
const dialogVisible = ref(false)
const qrDialogVisible = ref(false)
const qrShare = ref<PublicShareItem | null>(null)
const qrDataUrl = ref('')
const qrGenerating = ref(false)
const filters = reactive({
  keyword: '',
  createdBy: '',
  status: '',
})

const qrDialogWidth = computed(() => 'min(420px, calc(100vw - 32px))')

const fullCodePath = computed(() => {
  return props.functionNode?.full_code_path || props.functionDetail?.full_code_path || props.functionDetail?.router || ''
})

function publicLink(row: PublicShareItem) {
  return row.public_url
}

function shareDisplayTitle(row: Pick<PublicShareItem, 'title' | 'share_id'>) {
  return row.title || t('publicSharePanel.untitled')
}

function usageLimitText(maxUses: number) {
  return maxUses > 0
    ? t('publicSharePanel.maxUses', { count: maxUses })
    : t('publicSharePanel.unlimited')
}

async function load() {
  if (!fullCodePath.value) {
    return
  }
  loading.value = true
  try {
    const resp = await listPublicShares({
      full_code_path: fullCodePath.value,
      keyword: filters.keyword,
      created_by: filters.createdBy,
      status: filters.status,
    })
    shares.value = resp.items || []
    selectedShares.value = []
  } finally {
    loading.value = false
  }
}

function resetFilters() {
  filters.keyword = ''
  filters.createdBy = ''
  filters.status = ''
  load()
}

function openCreateDialog() {
  dialogVisible.value = true
}

function handleShareCreated(share: PublicShareItem) {
  shares.value = [share, ...shares.value]
}

async function disableShare(shareId: string) {
  disablingId.value = shareId
  try {
    await disablePublicShare(shareId)
    await load()
    ElMessage.success(t('publicSharePanel.closeSuccess'))
  } finally {
    disablingId.value = ''
  }
}

function toggleSelected(row: PublicShareItem) {
  selectedShares.value = selectedShares.value.some(item => item.share_id === row.share_id)
    ? selectedShares.value.filter(item => item.share_id !== row.share_id)
    : [...selectedShares.value, row]
}
async function closeSelected() {
  const selected = [...selectedShares.value]
  if (!selected.length || bulkClosing.value) return
  try { await ElMessageBox.confirm(t('shareGovernance.confirm', { count: selected.length }), t('publicSharePanel.close'), { type: 'warning' }) }
  catch { return }
  bulkClosing.value = true
  let failed = 0
  try {
    for (const row of selected) {
      try { await disablePublicShare(row.share_id) } catch { failed++ }
    }
    await load()
    if (failed) ElMessage.warning(t('shareGovernance.partial', { count: failed }))
    else ElMessage.success(t('publicSharePanel.closeSuccess'))
  } catch { ElMessage.error(t('shareGovernance.refreshFailed')) }
  finally { bulkClosing.value = false }
}

async function copyLink(link: string) {
  if (!link) {
    return
  }
  await navigator.clipboard.writeText(link)
  ElMessage.success(t('publicSharePanel.linkCopied'))
}

const currentQrLink = computed(() => {
  return qrShare.value ? publicLink(qrShare.value) : ''
})

async function openQrDialog(share: PublicShareItem) {
  qrShare.value = share
  qrDialogVisible.value = true
  qrDataUrl.value = ''
  qrGenerating.value = true
  try {
    qrDataUrl.value = await QRCode.toDataURL(publicLink(share), {
      errorCorrectionLevel: 'M',
      margin: 2,
      width: 256,
      color: {
        dark: '#111827',
        light: '#ffffff',
      },
    })
  } catch (_error) {
    ElMessage.error(t('publicSharePanel.qrGenerateFailed'))
  } finally {
    qrGenerating.value = false
  }
}

function qrFileName(share: PublicShareItem) {
  const safeName = safeQrBaseName(share)
  return `${safeName}-qrcode.png`
}

function safeQrBaseName(share: PublicShareItem) {
  return (share.title || share.share_id || 'public-share')
    .trim()
    .replace(/[\\/:*?"<>|]+/g, '-')
}

function downloadQrCode() {
  if (!qrDataUrl.value || !qrShare.value) {
    return
  }
  const link = document.createElement('a')
  link.href = qrDataUrl.value
  link.download = qrFileName(qrShare.value)
  link.click()
}

function isExpired(value?: string) {
  return !!value && new Date(value).getTime() <= Date.now()
}

function statusLabel(row: PublicShareItem) {
  if (!row.enabled) return t('publicSharePanel.statusDisabled')
  if (isExpired(row.expires_at)) return t('publicSharePanel.statusExpired')
  if (row.max_uses > 0 && row.use_count >= row.max_uses) return t('shareGovernance.exhausted')
  return t('publicSharePanel.statusEnabled')
}

function statusTagType(row: PublicShareItem) {
  if (!row.enabled) return 'info'
  if (isExpired(row.expires_at) || (row.max_uses > 0 && row.use_count >= row.max_uses)) return 'warning'
  return 'success'
}

function expiryHint(value: string) {
  const diff = new Date(value).getTime() - Date.now()
  if (diff <= 0) {
    return t('publicSharePanel.statusExpired')
  }
  const days = Math.ceil(diff / (24 * 60 * 60 * 1000))
  return t('publicSharePanel.expiresInDays', { count: days })
}

function formatDate(value: string) {
  return new Date(value).toLocaleString()
}

onMounted(load)
watch(fullCodePath, load)
</script>

<style scoped lang="scss">
.public-share-panel {
  height: 100%;
  min-height: 0;
  padding: 0;
}

.history-card {
  overflow: hidden;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 12px;
  background: var(--el-bg-color);
  box-shadow: 0 10px 24px rgba(15, 23, 42, 0.04);
}

.section-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 18px 14px;
  border-bottom: 1px solid var(--el-border-color-extra-light);
  background: var(--el-fill-color-blank);
}

.section-heading {
  min-width: 0;
}

.section-title {
  font-size: 15px;
  font-weight: 700;
  line-height: 1.4;
  color: var(--el-text-color-primary);
}

.section-subtitle {
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}

.form-history-toolbar {
  display: grid;
  grid-template-columns: minmax(260px, 1fr) minmax(150px, 190px) minmax(140px, 170px) auto auto;
  align-items: center;
  gap: 10px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--el-border-color-extra-light);
  background: var(--app-shell-panel-bg-strong, var(--el-bg-color));
}

.history-search {
  min-width: 0;
}

.history-action-select,
.history-user-select {
  width: 100%;
  min-width: 0;
}

.form-history-toolbar > .el-button {
  min-width: 78px;
  height: 36px;
  border-radius: 10px;
  font-weight: 600;
  box-shadow: none;
}

.form-history-toolbar :deep(.el-input__wrapper),
.form-history-toolbar :deep(.el-select__wrapper) {
  min-height: 36px;
  border-radius: 10px;
  background: var(--app-shell-panel-bg-strong, var(--el-bg-color));
  box-shadow: 0 0 0 1px var(--app-shell-panel-border, var(--el-border-color-light)) inset;
  transition: box-shadow 0.18s ease, background-color 0.18s ease;
}

.form-history-toolbar :deep(.el-input__wrapper:hover),
.form-history-toolbar :deep(.el-select__wrapper:hover) {
  box-shadow: 0 0 0 1px rgba(var(--el-color-primary-rgb), 0.28) inset;
}

.form-history-toolbar :deep(.el-input__wrapper.is-focus),
.form-history-toolbar :deep(.el-select__wrapper.is-focused) {
  box-shadow: 0 0 0 1px rgba(var(--el-color-primary-rgb), 0.45) inset, 0 0 0 3px rgba(var(--el-color-primary-rgb), 0.1);
}

.form-history-toolbar :deep(.el-input__inner),
.form-history-toolbar :deep(.el-select__placeholder),
.form-history-toolbar :deep(.el-select__selected-item) {
  font-size: 13px;
}

.history-table {
  width: 100%;
}

.mobile-share-list {
  display: none;
}

.history-table :deep(.el-table__inner-wrapper::before) {
  display: none;
}

.history-table :deep(.el-table__header th.el-table__cell) {
  background: var(--el-fill-color-light);
  color: var(--el-text-color-secondary);
  font-weight: 700;
}

.history-table :deep(.el-table__cell) {
  vertical-align: top;
}

.history-table :deep(.cell) {
  padding-top: 5px;
  padding-bottom: 5px;
}

.title-cell {
  min-width: 0;
}

.title-name {
  line-height: 1.25;
  color: var(--el-text-color-primary);
  font-weight: 600;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.url-cell {
  display: block;
  max-width: 100%;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--el-color-primary);
  font: inherit;
  font-size: 13px;
  line-height: 1.25;
  text-align: left;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
}

.link-description,
.muted-text,
.count-cell span,
.time-cell span {
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.link-description {
  margin-top: 2px;
  line-height: 1.25;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.count-cell,
.time-cell {
  line-height: 1.28;
  font-size: 13px;
}

.action-cell {
  display: flex;
  justify-content: flex-end;
  flex-wrap: wrap;
}

.qr-dialog-body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
}

.qr-title {
  max-width: 100%;
  color: var(--el-text-color-primary);
  font-size: 14px;
  font-weight: 700;
  line-height: 1.4;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.qr-description {
  max-width: 100%;
  color: var(--el-text-color-regular);
  font-size: 13px;
  line-height: 1.55;
  text-align: center;
  overflow-wrap: anywhere;
}

.qr-meta {
  display: flex;
  max-width: 100%;
  flex-wrap: wrap;
  justify-content: center;
  gap: 6px 14px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.5;
}

.qr-box {
  display: grid;
  place-items: center;
  width: 288px;
  min-height: 288px;
  padding: 16px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  background: #fff;
}

.qr-image {
  display: block;
  width: 256px;
  height: 256px;
}

.qr-link {
  display: block;
  width: 100%;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--el-color-primary);
  font: inherit;
  font-size: 13px;
  line-height: 1.5;
  text-align: center;
  word-break: break-all;
  cursor: pointer;
}

.qr-link-group {
  width: 100%;
}

.qr-link-label {
  margin-bottom: 4px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.4;
  text-align: center;
}

.qr-footer-actions {
  display: flex;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
}

.qr-footer-actions :deep(.el-button + .el-button) {
  margin-left: 0;
}

.custom-expire-picker,
.max-uses-input {
  display: block;
  width: 100%;
  margin-top: 12px;
}

@media (max-width: 820px) {
  .public-share-panel {
    overflow-x: hidden;
  }

  .history-card {
    border-radius: 8px;
  }

  .section-header,
  .form-history-toolbar {
    align-items: stretch;
  }

  .form-history-toolbar {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }

  .history-search {
    grid-column: 1 / -1;
  }

  .section-header {
    gap: 12px;
    padding: 14px;
  }

  .section-header .el-button {
    width: 100%;
  }

  .form-history-toolbar {
    padding: 12px;
  }

  .form-history-toolbar > .el-button {
    width: 100%;
  }

  .history-table {
    display: none;
  }

  .mobile-share-list {
    display: grid;
    gap: 10px;
    padding: 12px;
    background: var(--el-fill-color-lighter);
  }

  .mobile-share-card {
    min-width: 0;
    padding: 12px;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 8px;
    background: var(--el-bg-color);
  }

  .mobile-share-head,
  .mobile-share-foot {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 10px;
  }

  .mobile-share-title {
    min-width: 0;
    flex: 1;
  }

  .mobile-share-url {
    display: block;
    width: 100%;
    margin: 10px 0 0;
    padding: 8px;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 6px;
    background: var(--el-fill-color-light);
    color: var(--el-color-primary);
    font: inherit;
    font-size: 12px;
    line-height: 1.45;
    text-align: left;
    word-break: break-all;
    cursor: pointer;
  }

  .mobile-share-meta {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: 8px;
    margin-top: 10px;
  }

  .mobile-share-meta > div {
    min-width: 0;
    padding: 8px;
    border-radius: 6px;
    background: var(--el-fill-color-lighter);
  }

  .mobile-share-meta span,
  .mobile-share-meta em,
  .mobile-share-foot > span {
    display: block;
    color: var(--el-text-color-secondary);
    font-size: 11px;
    font-style: normal;
    line-height: 1.4;
    overflow-wrap: anywhere;
  }

  .mobile-share-meta strong {
    display: block;
    margin: 2px 0;
    color: var(--el-text-color-primary);
    font-size: 13px;
    line-height: 1.4;
    overflow-wrap: anywhere;
  }

  .mobile-share-foot {
    align-items: center;
    margin-top: 10px;
    padding-top: 8px;
    border-top: 1px solid var(--el-border-color-extra-light);
  }

  .mobile-share-actions {
    display: flex;
    justify-content: flex-end;
    flex-wrap: wrap;
    flex: 0 0 auto;
  }

  .mobile-share-actions :deep(.el-button + .el-button) {
    margin-left: 0;
  }

  .public-share-dialog :deep(.el-dialog__body),
  .public-share-qr-dialog :deep(.el-dialog__body) {
    padding: 14px 16px;
  }

  .public-share-dialog :deep(.el-dialog__footer),
  .public-share-qr-dialog :deep(.el-dialog__footer) {
    padding: 0 16px 16px;
  }

  .public-share-dialog :deep(.el-dialog__footer .el-button) {
    width: 100%;
    margin-left: 0;
  }

  .public-share-dialog :deep(.el-dialog__footer .el-button + .el-button) {
    margin-top: 8px;
  }

  .qr-box {
    width: min(288px, calc(100vw - 80px));
    min-height: min(288px, calc(100vw - 80px));
    padding: 12px;
  }

  .qr-image {
    width: min(256px, calc(100vw - 104px));
    height: min(256px, calc(100vw - 104px));
  }

  .qr-footer-actions {
    justify-content: stretch;
  }

  .qr-footer-actions .el-button {
    width: 100%;
  }
}

@media (max-width: 520px) {
  .form-history-toolbar {
    grid-template-columns: 1fr;
  }
}
</style>
