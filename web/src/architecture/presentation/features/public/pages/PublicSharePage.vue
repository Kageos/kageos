<template>
  <main class="public-share-page workspace-container">
    <section class="public-share-shell">
      <div v-if="loading" class="public-share-state">
        <el-skeleton :rows="8" animated />
      </div>

      <el-result
        v-else-if="errorMessage"
        icon="warning"
        :title="unavailableTitle"
        :sub-title="t('shareGovernance.unavailableHint')"
        class="public-share-result"
      >
        <template #extra>
          <el-button :icon="Refresh" @click="loadShare">{{ t('common.refresh') }}</el-button>
          <el-button class="submission-entry-button" round plain :icon="Clock" @click="openSubmissionDrawer">
            查看我的提交记录
          </el-button>
        </template>
      </el-result>

      <template v-else-if="functionDetail && gateway">
        <header class="public-share-header">
          <div>
            <p class="public-share-eyebrow">{{ t('publicSharePanel.publicForm') }}</p>
            <h1>{{ title }}</h1>
            <p v-if="description" class="public-share-description">{{ description }}</p>
          </div>
          <div class="public-share-header-actions">
            <div v-if="metaItems.length" class="public-share-meta">
              <span v-for="item in metaItems" :key="item">{{ item }}</span>
            </div>
            <el-button class="submission-entry-button" round plain :icon="Clock" @click="openSubmissionDrawer">
              提交记录<span v-if="submissionsTotal > 0">（{{ submissionsTotal }}）</span>
            </el-button>
          </div>
        </header>

        <div class="public-share-function-panel workspace-function-renderer public-share-renderer">
          <div class="function-runtime">
            <FormView
              :key="`public-form-${functionDetail.router}`"
              :function-detail="functionDetail"
              :form-gateway="gateway"
              :initial-data="presetValues"
              :show-submit-button="true"
              :show-public-share-button="false"
              :show-reset-button="false"
              response-display-mode="dialog"
            />
          </div>
        </div>
      </template>
    </section>

    <el-drawer
      v-model="submissionDrawerVisible"
      title="我的提交记录"
      :size="isMobile ? '100%' : '560px'"
      append-to-body
      class="public-submission-drawer"
    >
      <div class="submission-drawer-content">
        <el-alert
          title="这里只展示当前浏览器在这个表单上的提交记录"
          type="info"
          :closable="false"
          show-icon
        />

        <div class="submission-toolbar">
          <span>共 {{ submissionsTotal }} 条</span>
          <el-button
            link
            :icon="Refresh"
            :loading="submissionsLoading"
            @click="loadSubmissions(submissionsPage)"
          >
            刷新
          </el-button>
        </div>

        <el-skeleton v-if="submissionsLoading && submissions.length === 0" :rows="6" animated />

        <el-result
          v-else-if="submissionsError"
          icon="warning"
          title="提交记录加载失败"
          :sub-title="submissionsError"
        />

        <el-empty v-else-if="submissions.length === 0" description="当前还没有提交记录" />

        <template v-else>
          <el-collapse class="submission-list">
            <el-collapse-item
              v-for="(item, index) in submissions"
              :key="item.trace_id || `${item.created_at}-${index}`"
              :name="item.trace_id || `${item.created_at}-${index}`"
            >
              <template #title>
                <div class="submission-title">
                  <div class="submission-title-main">
                    <el-tag :type="submissionTagType(item.status)" size="small" effect="light">
                      {{ submissionStatusText(item.status) }}
                    </el-tag>
                    <span class="submission-summary">{{ item.summary || '表单提交' }}</span>
                  </div>
                  <time>{{ item.created_at }}</time>
                </div>
              </template>

              <div class="submission-details">
                <div v-if="item.duration_millis !== undefined" class="submission-meta-row">
                  <span>耗时</span>
                  <span>{{ item.duration_millis }} ms</span>
                </div>
                <div v-if="item.trace_id" class="submission-meta-row">
                  <span>追踪编号</span>
                  <code>{{ item.trace_id }}</code>
                </div>

                <section v-if="hasPayload(item.request_body)" class="submission-payload">
                  <h3>提交内容</h3>
                  <pre>{{ formatPayload(item.request_body) }}</pre>
                </section>

                <section v-if="hasPayload(item.response_body)" class="submission-payload">
                  <h3>处理结果</h3>
                  <pre>{{ formatPayload(item.response_body) }}</pre>
                </section>
              </div>
            </el-collapse-item>
          </el-collapse>

          <el-pagination
            v-if="submissionsTotal > submissionsPageSize"
            v-model:current-page="submissionsPage"
            class="submission-pagination"
            size="small"
            background
            layout="prev, pager, next"
            :pager-count="isMobile ? 5 : 7"
            :page-size="submissionsPageSize"
            :total="submissionsTotal"
            @current-change="loadSubmissions"
          />
        </template>
      </div>
    </el-drawer>
  </main>
</template>

<script setup lang="ts">
import { Clock, Refresh } from '@element-plus/icons-vue'
import { useMediaQuery } from '@vueuse/core'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import FormView from '@/architecture/presentation/views/FormView.vue'
import {
  PublicShareFormGateway,
  createPublicShareFunctionDetail,
  getPublicShareView,
  listPublicShareSubmissions,
  type PublicShareSubmissionItem,
  type PublicShareView,
} from '@/architecture/presentation/context/api/publicShare'
import { getErrorMessage } from '@/architecture/shared/apiError'
import type { FunctionDetail } from '@/architecture/domain/types'
import { lockFunctionDetailPresetFields } from '@/architecture/domain/utils/publicSharePreset'

const route = useRoute()
const { t } = useI18n()
const loading = ref(true)
const errorMessage = ref('')
const unavailableTitle = computed(() => {
  if (errorMessage.value.includes('已关闭')) return t('publicSharePanel.statusDisabled')
  if (errorMessage.value.includes('已过期')) return t('publicSharePanel.statusExpired')
  if (errorMessage.value.includes('上限')) return t('shareGovernance.exhausted')
  return t('shareGovernance.unavailable')
})
const view = ref<PublicShareView | null>(null)
const functionDetail = ref<FunctionDetail | null>(null)
const gateway = ref<PublicShareFormGateway | null>(null)
const submissionDrawerVisible = ref(false)
const submissions = ref<PublicShareSubmissionItem[]>([])
const submissionsLoading = ref(false)
const submissionsError = ref('')
const submissionsTotal = ref(0)
const submissionsPage = ref(1)
const submissionsPageSize = 20
const isMobile = useMediaQuery('(max-width: 700px)')
let submissionRefreshTimer: number | undefined

const shareId = computed(() => String(route.params.shareId || route.params.share_id || ''))
const title = computed(() => view.value?.title || '表单')
const description = computed(() => view.value?.description || '')
const presetValues = computed(() => view.value?.preset_values || {})
const metaItems = computed(() => {
  const pieces: string[] = []
  if (view.value?.expires_at) {
    pieces.push(`有效期至 ${new Date(view.value.expires_at).toLocaleString()}`)
  }
  if (view.value?.remaining_uses !== undefined) {
    pieces.push(`剩余 ${view.value.remaining_uses} 次`)
  }
  return pieces
})

async function loadShare() {
  loading.value = true
  errorMessage.value = ''
  try {
    const nextView = await getPublicShareView(shareId.value)
    view.value = nextView
    functionDetail.value = lockFunctionDetailPresetFields(
      createPublicShareFunctionDetail(nextView),
      nextView.preset_values || {}
    )
    gateway.value = new PublicShareFormGateway(nextView.share_id, scheduleSubmissionReload)
    document.title = `${nextView.title} - ${t('publicSharePanel.publicForm')}`
    void loadSubmissions(1)
  } catch (error) {
    errorMessage.value = getErrorMessage(error, '公开表单暂时不可用')
  } finally {
    loading.value = false
  }
}

async function loadSubmissions(page = submissionsPage.value) {
  if (!shareId.value) return
  submissionsLoading.value = true
  submissionsError.value = ''
  try {
    const result = await listPublicShareSubmissions(shareId.value, {
      page,
      page_size: submissionsPageSize,
    })
    submissions.value = result.items || []
    submissionsTotal.value = result.total || 0
    submissionsPage.value = result.page || page
  } catch (error) {
    submissionsError.value = getErrorMessage(error, '暂时无法读取提交记录')
  } finally {
    submissionsLoading.value = false
  }
}

function openSubmissionDrawer() {
  submissionDrawerVisible.value = true
  void loadSubmissions(submissionsPage.value)
}

function scheduleSubmissionReload() {
  if (submissionRefreshTimer !== undefined) {
    window.clearTimeout(submissionRefreshTimer)
  }
  submissionRefreshTimer = window.setTimeout(() => {
    void loadSubmissions(1)
  }, 700)
}

function submissionTagType(status: string): 'success' | 'danger' | 'info' {
  if (status === 'success') return 'success'
  if (status === 'failed') return 'danger'
  return 'info'
}

function submissionStatusText(status: string): string {
  if (status === 'success') return '成功'
  if (status === 'failed') return '失败'
  return status || '未知'
}

function hasPayload(value: unknown): boolean {
  return value !== undefined && value !== null
}

function formatPayload(value: unknown): string {
  if (typeof value === 'string') {
    try {
      return JSON.stringify(JSON.parse(value), null, 2)
    } catch {
      return value
    }
  }
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

onMounted(loadShare)
onBeforeUnmount(() => {
  if (submissionRefreshTimer !== undefined) {
    window.clearTimeout(submissionRefreshTimer)
  }
})
</script>

<style scoped lang="scss">
.public-share-page {
  height: 100vh;
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  padding: 16px 18px 18px;
  overflow: hidden;
  background: var(--app-shell-bg, var(--el-bg-color-page));
  background-attachment: fixed;
  color: var(--el-text-color-primary);
}

.public-share-shell {
  flex: 1;
  min-height: 0;
  width: min(1180px, 100%);
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 14px;
  overflow: hidden;
}

.public-share-state,
.public-share-result {
  padding: 48px;
}

.public-share-header {
  flex: 0 0 auto;
  display: flex;
  justify-content: space-between;
  gap: 24px;
  padding: 28px 32px;
  border: 1px solid var(--app-shell-panel-border, var(--el-border-color-lighter));
  border-radius: 24px;
  background: var(--app-shell-panel-bg, var(--el-bg-color));
  box-shadow: var(--app-shell-panel-shadow-soft, var(--box-shadow-base));
  position: relative;
  overflow: hidden;
}

.public-share-header::before,
.public-share-function-panel::before {
  content: '';
  position: absolute;
  top: 0;
  left: 28px;
  right: 28px;
  height: 1px;
  background: var(--app-shell-panel-highlight, rgba(255, 255, 255, 0.7));
  opacity: 0.7;
  pointer-events: none;
}

.public-share-eyebrow {
  margin: 0 0 8px;
  font-size: 12px;
  font-weight: 700;
  color: var(--el-color-primary);
  text-transform: uppercase;
}

.public-share-header h1 {
  margin: 0;
  font-size: 28px;
  line-height: 1.25;
  letter-spacing: 0;
  color: var(--el-text-color-primary);
}

.public-share-description {
  margin: 10px 0 0;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}

.public-share-meta {
  flex: 0 0 auto;
  max-width: 260px;
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 6px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.5;
  text-align: right;
}

.public-share-meta span {
  padding: 4px 9px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 999px;
  background: var(--el-fill-color-light);
}

.public-share-header-actions {
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  justify-content: center;
  gap: 12px;
}

.public-share-renderer {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.public-share-function-panel {
  position: relative;
  border: 1px solid var(--app-shell-panel-border, var(--el-border-color-lighter));
  border-radius: 24px;
  background: var(--app-shell-panel-bg, var(--el-bg-color));
  box-shadow: var(--app-shell-panel-shadow, var(--box-shadow-base));
  overflow: hidden;
}

.public-share-renderer .function-runtime {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 28px;
  -webkit-overflow-scrolling: touch;
}

.submission-drawer-content {
  display: flex;
  width: 100%;
  min-height: 100%;
  min-width: 0;
  flex-direction: column;
  gap: 18px;
}

.submission-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.submission-list {
  min-width: 0;
  border-top: none;
}

.submission-list :deep(.el-collapse-item__header) {
  height: auto;
  min-height: 64px;
  min-width: 0;
  padding: 10px 0;
  line-height: 1.4;
}

.submission-list :deep(.el-collapse-item__arrow) {
  flex: 0 0 auto;
  margin-left: 8px;
}

.submission-list :deep(.el-collapse-item__wrap),
.submission-list :deep(.el-collapse-item__content) {
  min-width: 0;
}

.submission-title {
  min-width: 0;
  width: 100%;
  padding-right: 12px;
  overflow: hidden;
}

.submission-title-main {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 8px;
  font-weight: 600;
}

.submission-title-main :deep(.el-tag) {
  flex: 0 0 auto;
  margin-top: 1px;
}

.submission-summary {
  min-width: 0;
  overflow: hidden;
  overflow-wrap: anywhere;
  word-break: break-word;
}

.submission-title time {
  display: block;
  margin-top: 7px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  font-weight: 400;
}

.submission-details {
  display: flex;
  width: 100%;
  min-width: 0;
  flex-direction: column;
  gap: 12px;
  padding: 4px 0 14px;
}

.submission-meta-row {
  display: grid;
  grid-template-columns: 72px minmax(0, 1fr);
  gap: 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.submission-meta-row code {
  overflow-wrap: anywhere;
  color: var(--el-text-color-regular);
}

.submission-payload h3 {
  margin: 0 0 8px;
  color: var(--el-text-color-regular);
  font-size: 13px;
}

.submission-payload {
  width: 100%;
  min-width: 0;
  max-width: 100%;
}

.submission-payload pre {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  max-width: 100%;
  max-height: 320px;
  margin: 0;
  padding: 12px;
  overflow: auto;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-primary);
  font-family: var(--app-font-mono, 'JetBrains Mono', monospace);
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.submission-pagination {
  max-width: 100%;
  justify-content: center;
  margin-top: auto;
  padding-top: 6px;
}

:global(.public-submission-drawer .el-drawer__body) {
  overflow-x: hidden;
}

@media (max-width: 700px) {
  .public-share-page {
    height: auto;
    min-height: 100svh;
    align-items: center;
    padding:
      max(18px, env(safe-area-inset-top))
      max(12px, env(safe-area-inset-right))
      calc(28px + env(safe-area-inset-bottom))
      max(12px, env(safe-area-inset-left));
    overflow-y: auto;
    overflow-x: hidden;
  }

  .public-share-shell {
    width: min(460px, 100%);
    flex: 0 0 auto;
    gap: 12px;
    overflow: visible;
  }

  .public-share-header {
    display: block;
    padding: 8px 4px 4px;
    border: none;
    border-radius: 0;
    background: transparent;
    box-shadow: none;
    text-align: left;
  }

  .public-share-header::before,
  .public-share-function-panel::before {
    display: none;
  }

  .public-share-eyebrow {
    margin-bottom: 6px;
    font-size: 11px;
  }

  .public-share-header h1 {
    font-size: 22px;
    line-height: 1.35;
  }

  .public-share-description {
    margin-top: 8px;
    font-size: 14px;
    line-height: 1.55;
  }

  .public-share-meta {
    margin-top: 10px;
    max-width: none;
    justify-content: flex-start;
    text-align: left;
    font-size: 12px;
  }

  .public-share-header-actions {
    align-items: stretch;
    gap: 10px;
  }

  .submission-entry-button {
    width: 100%;
    min-height: 44px;
    padding-right: 18px;
    padding-left: 18px;
  }

  .public-share-state,
  .public-share-result {
    width: 100%;
    padding: 32px 12px;
  }

  .public-share-renderer {
    min-height: 0;
  }

  .public-share-function-panel {
    border: 0;
    border-radius: 0;
    background: transparent;
    box-shadow: none;
    overflow: visible;
  }

  .public-share-renderer .function-runtime {
    overflow: visible;
    padding: 0;
  }

  .public-share-renderer :deep(.form-view-main) {
    padding: 20px 18px 22px;
    border-radius: 16px;
  }

  .public-share-renderer :deep(.section-title) {
    margin-bottom: 18px;
    font-size: 18px;
    text-align: center;
  }

  .public-share-renderer :deep(.form-actions-row) {
    display: grid;
    grid-template-columns: 1fr;
  }

  .public-share-renderer :deep(.form-actions-row .el-button) {
    width: 100%;
    margin-left: 0;
  }

  .submission-summary {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 3;
  }

  .submission-toolbar :deep(.el-button) {
    min-width: 44px;
    min-height: 44px;
  }

  .submission-pagination :deep(.btn-prev),
  .submission-pagination :deep(.btn-next),
  .submission-pagination :deep(.el-pager li) {
    min-width: 36px;
    height: 36px;
    line-height: 36px;
  }

  :global(.public-submission-drawer .el-drawer__header) {
    margin-bottom: 18px;
    padding-top: max(20px, env(safe-area-inset-top));
    padding-right: max(20px, env(safe-area-inset-right));
    padding-left: max(20px, env(safe-area-inset-left));
  }

  :global(.public-submission-drawer .el-drawer__body) {
    padding-right: max(20px, env(safe-area-inset-right));
    padding-bottom: max(20px, env(safe-area-inset-bottom));
    padding-left: max(20px, env(safe-area-inset-left));
  }
}
</style>
