<!--
  MessageToolCalls - 连续多个工具时，每个工具一块：工具名+状态在「该块」上方，下面是对应 viewport
  - 默认只显示一行摘要；点击展开后查看参数、结果和错误
  - 每个工具：块头写工具名和耗时，下面才是该工具的 viewport
  - 输出文件仍在下方独立展示
-->
<template>
  <div v-if="hasVisibleContent" class="message-tool-calls">
    <details
      v-if="visibleToolCalls.length > 0"
      class="message-tool-calls-trace"
      :open="detailsOpen"
      @toggle="onDetailsToggle"
    >
      <summary class="message-tool-calls-summary">
        <span class="summary-main">
          <el-icon v-if="hasRunning" class="summary-icon summary-icon--running"><Loading /></el-icon>
          <el-icon v-else-if="errorCount > 0" class="summary-icon summary-icon--error"><CircleClose /></el-icon>
          <el-icon v-else class="summary-icon summary-icon--ok"><CircleCheck /></el-icon>
          <span class="summary-title">工具调用 {{ visibleToolCalls.length }} 个</span>
        </span>
        <span class="summary-tools">
          <span
            v-for="tool in summaryToolGroups"
            :key="tool.name"
            class="summary-tool-chip"
          >
            {{ tool.count > 1 ? `${tool.name} x${tool.count}` : tool.name }}
          </span>
          <span v-if="hiddenToolGroupCount > 0" class="summary-tool-chip summary-tool-chip--more">
            +{{ hiddenToolGroupCount }}
          </span>
        </span>
        <span :class="['summary-status', summaryStatusClass]">{{ summaryStatusLabel }}</span>
      </summary>
      <div class="message-tool-calls-detail">
        <template v-for="(tc, idx) in visibleToolCalls" :key="tc.id ?? `${idx}-${tc.name}`">
          <div :class="['message-tool-calls-block', { 'message-tool-calls-block--first': idx === 0 }]">
            <div class="message-tool-calls-block-head">
              <el-icon v-if="tc.status === 'ok'" class="block-head-icon block-head-icon--ok"><CircleCheck /></el-icon>
              <el-icon v-else-if="tc.status === 'error'" class="block-head-icon block-head-icon--error"><CircleClose /></el-icon>
              <el-icon v-else class="block-head-icon block-head-icon--running"><Loading /></el-icon>
              <span class="block-head-name">{{ getToolDisplayName(tc) }}</span>
              <span
                v-if="getToolDurationLabel(tc, idx)"
                :class="['block-head-duration', { 'block-head-duration--running': isToolTimerRunning(tc, idx) }]"
              >
                <span v-if="isToolTimerRunning(tc, idx)" class="block-head-duration-dot"></span>
                {{ isToolTimerRunning(tc, idx) ? getToolDurationLabel(tc, idx) : `耗时 ${getToolDurationLabel(tc, idx)}` }}
              </span>
            </div>
            <div v-if="isRenderablePrdToolCall(tc)" class="message-tool-calls-prd mini-msg-prd-preview">
              <PrdPreview
                :data="tc.result_data"
                :confirm-disabled="confirmDisabled"
                show-confirmation
                @confirm="emit('confirm-prd', $event)"
              />
            </div>
            <div
              v-else
              :class="['message-tool-calls-viewport', { 'message-tool-calls-viewport--first': idx === 0 }]"
              :ref="(el) => setViewportRef(el as HTMLElement | null, idx)"
            >
              <div class="message-tool-calls-output">
                <div
                  v-for="(line, lineIdx) in getLinesForTool(tc)"
                  :key="lineIdx"
                  :class="['output-line', line.type]"
                >
                  {{ line.text }}
                </div>
                <div
                  v-if="hasRunning && idx === visibleToolCalls.length - 1"
                  class="output-line output-line--cursor"
                >
                  ▌
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>
    </details>
    <OutputFilesDisplay
      v-if="fileGroups.length > 0"
      :file-groups="fileGroups"
      deletable
      class="message-output-files"
    />
    <OutputDisplayFields
      v-if="displayFields.length > 0"
      :fields="displayFields"
      class="message-output-display-fields"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Loading, CircleCheck, CircleClose } from '@element-plus/icons-vue'
import OutputFilesDisplay from './OutputFilesDisplay.vue'
import OutputDisplayFields from './OutputDisplayFields.vue'
import PrdPreview from './PrdPreview.vue'
import type { WorkspaceChatToolCallSummary } from '@/architecture/presentation/context/api/workspace'
import type { OutputFileGroup } from '@/architecture/presentation/composables/useOutputFileGroups'
import { extractAllDisplayFields } from '@/architecture/presentation/composables/useOutputDisplayFields'
import {
  formatWorkspaceToolCallResultPreview,
  getVisibleWorkspaceToolCalls,
  getWorkspaceToolCallDisplayName
} from '@/architecture/presentation/utils/workspaceRoleDisplay'

const props = withDefaults(defineProps<{
  toolCalls: WorkspaceChatToolCallSummary[]
  fileGroups: OutputFileGroup[]
  confirmDisabled?: boolean
}>(), {
  confirmDisabled: false,
})

const emit = defineEmits<{
  (e: 'confirm-prd', payload: { remark: string; prd: unknown }): void
}>()

const visibleToolCalls = computed(() => getVisibleWorkspaceToolCalls(props.toolCalls))
const displayFields = computed(() => extractAllDisplayFields(visibleToolCalls.value))
const hasVisibleContent = computed(() =>
  visibleToolCalls.value.length > 0 || props.fileGroups.length > 0 || displayFields.value.length > 0
)
const allSummaryToolGroups = computed(() => {
  const groups: Array<{ name: string; count: number }> = []
  const byName = new Map<string, { name: string; count: number }>()
  for (const call of visibleToolCalls.value) {
    const name = getToolDisplayName(call)
    const existing = byName.get(name)
    if (existing) {
      existing.count += 1
      continue
    }
    const group = { name, count: 1 }
    byName.set(name, group)
    groups.push(group)
  }
  return groups
})
const summaryToolGroups = computed(() => allSummaryToolGroups.value.slice(0, 4))
const hiddenToolGroupCount = computed(() => Math.max(0, allSummaryToolGroups.value.length - summaryToolGroups.value.length))
const errorCount = computed(() => visibleToolCalls.value.filter((t) => t.status === 'error').length)
const hasInlinePreviewToolCall = computed(() =>
  visibleToolCalls.value.some((tc) => isRenderablePrdToolCall(tc))
)
const detailsOpen = ref(hasInlinePreviewToolCall.value)

/** 每个工具一个 viewport，用数组存 DOM，滚动时滚最后一个 */
const viewportRefs = ref<(HTMLElement | null)[]>([])

function setViewportRef(el: HTMLElement | null, idx: number) {
  const arr = viewportRefs.value
  if (el) {
    while (arr.length <= idx) arr.push(null)
    arr[idx] = el
    // 不在这里 trim，避免 Vue ref 回调乱序时把后面已设置的 ref 清掉导致错乱
  } else {
    viewportRefs.value = arr.slice(0, idx)
  }
}

const hasRunning = computed(() =>
  visibleToolCalls.value.some((t) => t.status === 'streaming' || t.status === 'running')
)
const summaryStatusLabel = computed(() => {
  if (hasRunning.value) return '执行中'
  if (errorCount.value > 0) return `${errorCount.value} 失败`
  return '完成'
})
const summaryStatusClass = computed(() => {
  if (hasRunning.value) return 'summary-status--running'
  if (errorCount.value > 0) return 'summary-status--error'
  return 'summary-status--ok'
})

interface RuntimeTimer {
  startedAt: number
  completedAt?: number
}

const toolTimers = new Map<string, RuntimeTimer>()
const timerNow = ref(Date.now())
let timerInterval: ReturnType<typeof setInterval> | null = null

function isToolPending(status: string): boolean {
  return status === 'streaming' || status === 'running'
}

function isToolDone(status: string): boolean {
  return status === 'ok' || status === 'error'
}

function getToolTimerKey(tc: WorkspaceChatToolCallSummary, idx: number): string {
  return tc.id ? `id:${tc.id}` : `idx:${idx}:${tc.name}`
}

function hasActiveToolTimer(): boolean {
  for (const timer of toolTimers.values()) {
    if (timer.completedAt == null) return true
  }
  return false
}

function syncTimerInterval() {
  if (hasActiveToolTimer()) {
    if (timerInterval == null) {
      timerInterval = setInterval(() => {
        timerNow.value = Date.now()
      }, 250)
    }
    return
  }
  if (timerInterval != null) {
    clearInterval(timerInterval)
    timerInterval = null
  }
}

function syncToolTimers() {
  const now = Date.now()
  const visibleKeys = new Set<string>()
  let changed = false

  visibleToolCalls.value.forEach((tc, idx) => {
    const key = getToolTimerKey(tc, idx)
    visibleKeys.add(key)
    const existing = toolTimers.get(key)

    if (isToolPending(tc.status)) {
      if (!existing) {
        toolTimers.set(key, { startedAt: now })
        changed = true
      } else if (existing.completedAt != null) {
        delete existing.completedAt
        changed = true
      }
      return
    }

    if (isToolDone(tc.status) && existing && existing.completedAt == null) {
      existing.completedAt = now
      changed = true
    }
  })

  for (const key of toolTimers.keys()) {
    if (!visibleKeys.has(key)) {
      toolTimers.delete(key)
      changed = true
    }
  }

  if (changed) timerNow.value = now
  syncTimerInterval()
}

function formatDuration(ms: number): string {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (hours > 0) return `${hours}时${minutes}分${seconds}秒`
  if (minutes > 0) return `${minutes}分${seconds}秒`
  return `${seconds}秒`
}

function getToolDurationLabel(tc: WorkspaceChatToolCallSummary, idx: number): string {
  const timer = toolTimers.get(getToolTimerKey(tc, idx))
  if (!timer) return ''
  const end = timer.completedAt ?? timerNow.value
  return formatDuration(end - timer.startedAt)
}

function isToolTimerRunning(tc: WorkspaceChatToolCallSummary, idx: number): boolean {
  const timer = toolTimers.get(getToolTimerKey(tc, idx))
  return !!timer && timer.completedAt == null && isToolPending(tc.status)
}

function onDetailsToggle(event: Event) {
  detailsOpen.value = (event.currentTarget as HTMLDetailsElement).open
}

function isRenderablePrdToolCall(tc: WorkspaceChatToolCallSummary): boolean {
  return tc.name === 'write_prd' && tc.status === 'ok' && tc.result_data != null
}

function getToolDisplayName(tc: WorkspaceChatToolCallSummary): string {
  return getWorkspaceToolCallDisplayName(tc)
}

/** 把字符串里字面的 \n、\r 转成真实换行，这样嵌套 JSON 里的换行能正确展示 */
function unescapeNewlines(s: string): string {
  return s.replace(/\\n/g, '\n').replace(/\\r/g, '\r')
}

/** 结果/错误：仅还原转义的换行，不截断，完整内容在视口内滚动查看 */
function formatResultOrError(s: string): string {
  if (!s || !s.trim()) return ''
  return unescapeNewlines(s.trim())
}

function formatResultData(val: unknown): string {
  if (val == null) return ''
  try {
    return JSON.stringify(val, null, 2)
  } catch {
    return String(val)
  }
}

/** 参数：多行展示（pretty JSON），仅还原换行，不截断，完整内容在视口内滚动查看 */
function formatArgsPreview(argsStr: string | undefined): string {
  if (!argsStr || !argsStr.trim()) return ''
  const trimmed = argsStr.trim()
  let multiLine: string
  try {
    const obj = JSON.parse(trimmed)
    multiLine = JSON.stringify(obj, null, 2)
    multiLine = unescapeNewlines(multiLine)
  } catch {
    multiLine = unescapeNewlines(trimmed)
  }
  return multiLine
}

/** 单个工具的输出行（工具名+状态已在块头展示，这里只含参数/结果/错误） */
function getLinesForTool(tc: WorkspaceChatToolCallSummary): { text: string; type: string }[] {
  const lines: { text: string; type: string }[] = []
  if (tc.arguments) {
    const argsPreview = formatArgsPreview(tc.arguments)
    if (argsPreview) lines.push({ text: `参数: ${argsPreview}`, type: 'args' })
  }
  const resultPreview = formatWorkspaceToolCallResultPreview(tc)
  if (resultPreview) {
    lines.push({ text: resultPreview, type: 'result' })
  } else if (tc.result) {
    const preview = formatResultOrError(tc.result)
    if (preview) lines.push({ text: preview, type: 'result' })
  } else if (tc.result_data != null) {
    const preview = formatResultData(tc.result_data)
    if (preview) lines.push({ text: preview, type: 'result' })
  }
  if (tc.error) {
    const preview = formatResultOrError(tc.error)
    if (preview) lines.push({ text: `错误: ${preview}`, type: 'error' })
  }
  return lines
}

/** 只用 scrollTop 滚 viewport 自身，不用 scrollIntoView 避免带动外层容器跳动 */
let _vpRafId = 0
function scrollAllViewportsToBottom() {
  if (_vpRafId) return
  _vpRafId = requestAnimationFrame(() => {
    _vpRafId = 0
    const arr = viewportRefs.value
    for (const vp of arr) {
      if (vp) vp.scrollTop = vp.scrollHeight - vp.clientHeight
    }
  })
}

watch(
  () => visibleToolCalls.value.map((t) => {
    const resultDataLen = t.result_data == null ? 0 : formatResultData(t.result_data).length
    return (t.arguments?.length ?? 0) + (t.result?.length ?? 0) + resultDataLen + (t.error?.length ?? 0)
  }),
  () => scrollAllViewportsToBottom(),
  { deep: true }
)
watch(
  () => visibleToolCalls.value.length,
  (len) => {
    const arr = viewportRefs.value
    if (arr.length > len) viewportRefs.value = arr.slice(0, len)
    scrollAllViewportsToBottom()
  }
)
watch(hasRunning, (running) => {
  if (running && detailsOpen.value) scrollAllViewportsToBottom()
})
watch(hasInlinePreviewToolCall, (shouldOpen) => {
  if (shouldOpen) detailsOpen.value = true
})
watch(
  () => visibleToolCalls.value.map((t, idx) => `${getToolTimerKey(t, idx)}:${t.status}`).join('|'),
  syncToolTimers,
  { immediate: true }
)
onBeforeUnmount(() => {
  if (timerInterval != null) clearInterval(timerInterval)
})
</script>

<style scoped lang="scss">
.message-tool-calls {
  width: 100%;
  margin-top: 6px;
}

.message-tool-calls-trace {
  width: 100%;
}

.message-tool-calls-summary {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  min-height: 28px;
  padding: 4px 8px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  background: transparent;
  color: var(--el-text-color-secondary);
  cursor: pointer;
  list-style: none;
  font-size: 11px;
}

.message-tool-calls-summary::-webkit-details-marker {
  display: none;
}

.message-tool-calls-summary::before {
  content: '›';
  flex-shrink: 0;
  color: var(--el-text-color-placeholder);
  transition: transform 0.16s ease;
}

.message-tool-calls-trace[open] .message-tool-calls-summary {
  border-radius: 8px 8px 0 0;
}

.message-tool-calls-trace[open] .message-tool-calls-summary::before {
  transform: rotate(90deg);
}

.summary-main,
.summary-tools,
.summary-status {
  display: inline-flex;
  align-items: center;
}

.summary-main {
  gap: 5px;
  flex-shrink: 0;
  color: var(--el-text-color-primary);
}

.summary-icon {
  font-size: 13px;
}

.summary-icon--running {
  color: var(--el-color-primary);
  animation: think-spin 0.8s linear infinite;
}

.summary-icon--ok {
  color: var(--el-color-success);
}

.summary-icon--error {
  color: var(--el-color-danger);
}

.summary-title {
  font-weight: 600;
  white-space: nowrap;
}

.summary-tools {
  gap: 4px;
  min-width: 0;
  overflow: hidden;
}

.summary-tool-chip {
  max-width: 120px;
  overflow: hidden;
  padding: 1px 5px;
  border: 1px solid var(--el-border-color-extra-light);
  border-radius: 999px;
  color: var(--el-text-color-secondary);
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.summary-tool-chip--more {
  flex-shrink: 0;
  color: var(--el-text-color-placeholder);
}

.summary-status {
  flex-shrink: 0;
  margin-left: auto;
  font-weight: 600;
  white-space: nowrap;
}

.summary-status--running {
  color: var(--el-color-primary);
}

.summary-status--ok {
  color: var(--el-color-success);
}

.summary-status--error {
  color: var(--el-color-danger);
}

.message-tool-calls-detail {
  padding: 6px;
  border: 1px solid var(--el-border-color-lighter);
  border-top: none;
  border-radius: 0 0 8px 8px;
  background: transparent;
}

@keyframes think-spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* 每个工具一块：块头（工具名+状态）+ viewport */
.message-tool-calls-block {
  margin-top: 6px;
}
.message-tool-calls-block--first {
  margin-top: 0;
}

.message-tool-calls-block-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 8px;
  background: var(--el-fill-color-lighter);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: var(--el-border-radius-base) var(--el-border-radius-base) 0 0;
  font-size: 12px;
}

.block-head-icon {
  font-size: 14px;
  flex-shrink: 0;
}
.block-head-icon--ok {
  color: var(--el-color-success);
}
.block-head-icon--error {
  color: var(--el-color-danger);
}
.block-head-icon--running {
  color: var(--el-color-primary);
  animation: think-spin 0.8s linear infinite;
}

.block-head-name {
  min-width: 0;
  overflow: hidden;
  font-weight: 500;
  color: var(--el-text-color-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.block-head-duration {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  margin-left: auto;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1;
}

.block-head-duration--running {
  color: var(--el-color-primary);
}

.block-head-duration-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  box-shadow: 0 0 8px rgba(64, 158, 255, 0.42);
}

/* 每个工具一块 viewport，约 4-5 行高度，内容可滚动 */
.message-tool-calls-viewport {
  max-height: 6.5em;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 7px 10px;
  background: var(--el-fill-color-blank);
  border: 1px solid var(--el-border-color-lighter);
  border-top: none;
  border-radius: 0 0 var(--el-border-radius-base) var(--el-border-radius-base);
  line-height: 1.5;
  font-size: 11px;
  color: var(--el-text-color-regular);
}
.message-tool-calls-viewport--first {
  /* 紧接在块头下方，样式已由 border-top: none 体现 */
}

.message-tool-calls-prd {
  :deep(.prd-preview) {
    margin-top: 0;
    border-top: none;
    border-radius: 0 0 var(--el-border-radius-base) var(--el-border-radius-base);
  }
}

.message-tool-calls-output {
  white-space: pre-wrap;
  word-break: break-word;
}

.output-line {
  margin-bottom: 2px;
  white-space: pre-wrap; /* 保留参数/结果中的换行，避免整段挤在一行 */
  word-break: break-word;

  &:last-child {
    margin-bottom: 0;
  }
}

.output-line.tool {
  color: var(--el-text-color-primary);
  font-weight: 500;
}

.output-line.args {
  color: var(--el-text-color-secondary);
  padding-left: 10px;
  border-left: 2px solid var(--el-color-primary-light-5);
  font-size: 11px;
}

.output-line.result {
  color: var(--el-text-color-regular);
  padding-left: 10px;
  border-left: 2px solid var(--el-border-color-lighter);
}

.output-line.error {
  color: var(--el-color-danger);
  padding-left: 10px;
  border-left: 2px solid var(--el-color-danger-light-5);
}

.output-line--cursor {
  display: inline-block;
  animation: think-blink 0.8s step-end infinite;
  color: var(--el-color-primary);
}
@keyframes think-blink {
  50% { opacity: 0; }
}

.message-output-files {
  margin-top: 10px;
}

.message-output-display-fields {
  margin-top: 10px;
}
</style>
