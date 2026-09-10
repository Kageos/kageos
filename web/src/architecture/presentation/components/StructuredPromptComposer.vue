<template>
  <div
    ref="rootRef"
    class="structured-prompt-composer"
    :class="{
      'is-disabled': disabled,
      'is-preview': mode === 'preview',
      'is-focused': focused,
      'is-compact': compact,
      'has-toolbar': showToolbar,
      [`mention-${mentionPanelPlacement}`]: true,
    }"
    data-testid="structured-prompt-composer"
    @click="handleRootClick"
    @focusout="handleRootFocusOut"
  >
    <div v-if="showToolbar" class="spc-toolbar">
      <div class="spc-mode-tabs" role="tablist" aria-label="输入模式">
        <button
          type="button"
          :class="['spc-mode-btn', { 'is-active': mode === 'edit' }]"
          :aria-selected="mode === 'edit'"
          role="tab"
          @click="setMode('edit')"
        >
          <el-icon :size="14"><EditPen /></el-icon>
          编辑
        </button>
        <button
          type="button"
          :class="['spc-mode-btn', { 'is-active': mode === 'preview' }]"
          :aria-selected="mode === 'preview'"
          role="tab"
          @click="setMode('preview')"
        >
          <el-icon :size="14"><View /></el-icon>
          预览
        </button>
      </div>

      <div v-if="resourceSegments.length > 0" class="spc-token-summary" aria-label="已识别资源">
        <span
          v-for="segment in resourceSegments.slice(0, 3)"
          :key="`${segment.start}-${segment.text}`"
          class="spc-token-pill"
          :title="segment.path"
        >
          {{ getResourceDisplayLabel(segment.path || segment.text) }}
        </span>
        <span v-if="resourceSegments.length > 3" class="spc-token-count">
          +{{ resourceSegments.length - 3 }}
        </span>
      </div>
    </div>

    <div
      v-show="mode === 'edit'"
      ref="editorRef"
      class="spc-editor"
      :class="{ 'is-empty': !currentText.trim() }"
      :contenteditable="disabled ? 'false' : 'true'"
      :data-placeholder="placeholder"
      :data-testid="editorTestId"
      :style="editorStyle"
      role="textbox"
      aria-multiline="true"
      :aria-label="placeholder"
      spellcheck="false"
      @input="handleEditorInput"
      @paste="handlePaste"
      @keydown="handleEditorKeydown"
      @keyup="handleEditorCursorChange"
      @click="handleEditorClick"
      @mouseup="handleEditorCursorChange"
      @focus="handleFocus"
      @blur="handleBlur"
      @compositionstart="onCompositionStart"
      @compositionend="onCompositionEnd"
    />

    <div v-show="mode === 'preview'" class="spc-preview" :style="editorStyle" data-testid="structured-prompt-preview">
      <div v-if="invocationBlocks.length > 0" class="spc-invocation-list">
        <section
          v-for="(block, index) in invocationBlocks"
          :key="`${block.tool}-${block.resourcePath}-${index}`"
          class="spc-invocation-card"
        >
          <div class="spc-invocation-head">
            <span class="spc-invocation-tool">{{ block.tool || '函数调用' }}</span>
            <span
              class="spc-invocation-resource"
              :title="block.resourcePath"
              role="button"
              tabindex="0"
              @click.stop="openInfoCardForResource(block.resourcePath, $event)"
              @keydown.enter.stop.prevent="openInfoCardForResource(block.resourcePath, $event)"
            >
              {{ getResourceDisplayLabel(block.resourcePath) }}
            </span>
          </div>
          <div v-if="block.params.length > 0" class="spc-param-row">
            <span
              v-for="param in block.params.slice(0, 4)"
              :key="`${block.tool}-${param.key}`"
              class="spc-param-chip"
              :title="`${param.key} = ${param.value}`"
            >
              {{ param.key }}{{ param.fixed ? ' 固定' : '' }}
            </span>
          </div>
        </section>
      </div>

      <div v-if="currentText.trim()" class="spc-preview-body">
        <template v-for="segment in structuredSegments" :key="`${segment.start}-${segment.end}-${segment.type}`">
          <span v-if="segment.type === 'text'" class="spc-preview-text">{{ segment.text }}</span>
          <span
            v-else-if="segment.type === 'user'"
            class="spc-user-chip"
            :title="segment.text"
            role="button"
            tabindex="0"
            @click.stop="openInfoCardForUser(segment.username, $event)"
            @keydown.enter.stop.prevent="openInfoCardForUser(segment.username, $event)"
          >
            <span class="spc-user-name">{{ getUserTokenLabel(segment.username) }}</span>
          </span>
          <span
            v-else
            class="spc-resource-chip"
            :title="segment.path"
            :data-path="segment.path"
            role="button"
            tabindex="0"
            @click.stop="openInfoCardForResource(segment.path || '', $event)"
            @keydown.enter.stop.prevent="openInfoCardForResource(segment.path || '', $event)"
          >
            <ResourceTokenIcon :meta="getResourceMeta(segment.path || '')" :path="segment.path || ''" />
            <span class="spc-resource-name">{{ getResourceDisplayLabel(segment.path || segment.text) }}</span>
          </span>
        </template>
      </div>
      <div v-else class="spc-preview-placeholder">{{ placeholder }}</div>
    </div>

    <el-popover
      :visible="mentionPanelOpen"
      trigger="manual"
      virtual-triggering
      :virtual-ref="mentionVirtualRef"
      :placement="mentionPopoverPlacement"
      :width="mentionPopoverWidth"
      :offset="8"
      :show-arrow="false"
      :teleported="true"
      :persistent="false"
      popper-class="spc-mention-popover"
    >
      <div
        class="spc-mention-panel"
        data-testid="structured-prompt-mention-panel"
        @mousedown.prevent="cancelMentionClose"
      >
        <div class="spc-mention-header">
          <span class="spc-mention-trigger">{{ mentionQuery?.trigger }}</span>
          <span>{{ mentionModeLabel }}</span>
        </div>
        <template v-if="mentionQuery?.kind === 'resource'">
          <div class="spc-resource-scopes" aria-label="资源范围">
            <button type="button" :disabled="!currentDirectory" :aria-pressed="resourceScope === 'current' && !!currentDirectory" @click="changeResourceFilter('current', resourceType)">当前目录</button>
            <button type="button" :aria-pressed="resourceScope === 'other' || !currentDirectory" @click="changeResourceFilter('other', resourceType)">{{ currentDirectory ? '其他目录' : '全部目录' }}</button>
          </div>
          <div class="spc-resource-location" :title="currentDirectory">{{ resourceScope === 'current' && currentDirectory ? `${currentDirectory} · 含子目录` : currentDirectory ? '搜索有权访问的其他目录资源' : '搜索所有有权访问的目录资源' }}</div>
          <div class="spc-resource-tabs" role="tablist" aria-label="资源类型">
            <button v-for="item in resourceTypes" :key="item.value" type="button" role="tab" :aria-selected="resourceType === item.value" @click="changeResourceFilter(resourceScope, item.value)">{{ item.label }}</button>
          </div>
        </template>
        <div v-if="mentionLoading" class="spc-mention-state">搜索中...</div>
        <div v-else-if="mentionOptions.length === 0" class="spc-mention-state">
          {{ mentionEmptyText }}
        </div>
        <div v-else class="spc-mention-list">
          <button
            v-for="(option, index) in mentionOptions"
            :key="option.key"
            type="button"
            :class="['spc-mention-option', { 'is-active': index === highlightedMentionIndex }]"
            :data-testid="`structured-prompt-mention-option-${index}`"
            @mouseenter="highlightedMentionIndex = index"
            @mousedown.prevent="applyMentionOption(option)"
          >
            <span :class="['spc-mention-icon', `is-${option.kind}`, option.iconClass]">
              <UserAvatar
                v-if="option.kind === 'user'"
                :src="option.avatar"
                :size="28"
                :alt="option.label"
                class="spc-mention-avatar"
              />
              <img
                v-else-if="option.iconSrc"
                :src="option.iconSrc"
                :alt="option.typeLabel"
                class="spc-mention-resource-img"
              />
              <component
                :is="option.iconComponent"
                v-else-if="option.iconComponent"
                :size="17"
                class="spc-mention-resource-component"
              />
              <el-icon v-else><Document /></el-icon>
            </span>
            <span class="spc-mention-main">
              <span class="spc-mention-title-row">
                <span class="spc-mention-title" :title="option.label">{{ option.label }}</span>
              </span>
              <span v-if="option.kind === 'resource'" class="spc-mention-desc spc-mention-purpose">{{ option.resourceMeta?.description || '尚未填写用途说明' }}</span>
              <span v-if="option.kind === 'resource'" class="spc-mention-desc" :title="option.value">{{ option.value }}</span>
              <span v-else-if="option.description" class="spc-mention-desc" :title="option.description">
                {{ option.description }}
              </span>
            </span>
            <span class="spc-mention-type">{{ option.typeLabel }}</span>
          </button>
        </div>
        <div class="spc-mention-footer">
          <span>↑ ↓ 选择 · Enter 插入 · Esc 关闭</span>
          <button v-if="mentionHasMore || mentionSearchError" type="button" :disabled="mentionLoading" @click="loadMoreMentions">{{ mentionSearchError ? '重试' : '加载更多' }}</button>
        </div>
      </div>
    </el-popover>

    <el-popover
      :visible="!!activeInfoCard"
      trigger="manual"
      virtual-triggering
      :virtual-ref="infoCardAnchor || undefined"
      placement="top-start"
      :offset="8"
      :width="360"
      :show-arrow="true"
      :teleported="true"
      :persistent="false"
      popper-class="spc-info-popover"
    >
    <div
      v-if="activeInfoCard"
      ref="infoCardRef"
      class="spc-info-card"
      data-testid="structured-prompt-info-card"
      role="dialog"
      :aria-label="`${activeInfoCard.title}的信息`"
      @mousedown.stop.prevent
      @click.stop
      @keydown.esc.stop="closeInfoCard"
    >
      <button type="button" class="spc-info-close" aria-label="关闭信息卡片" @click="closeInfoCard">×</button>
      <div class="spc-info-head">
        <span :class="['spc-info-icon', `is-${activeInfoCard.kind}`, activeInfoCard.iconClass]">
          <UserAvatar
            v-if="activeInfoCard.kind === 'user'"
            :src="activeInfoCard.avatar"
            :size="28"
            :alt="activeInfoCard.title"
            class="spc-info-avatar"
          />
          <img
            v-else-if="activeInfoCard.iconSrc"
            :src="activeInfoCard.iconSrc"
            :alt="activeInfoCard.subtitle"
            class="spc-info-img"
          />
          <component
            :is="activeInfoCard.iconComponent"
            v-else-if="activeInfoCard.iconComponent"
            :size="18"
            class="spc-info-component"
          />
          <el-icon v-else><Document /></el-icon>
        </span>
        <span class="spc-info-main">
          <strong>{{ activeInfoCard.title }}</strong>
          <span v-if="activeInfoCard.subtitle">{{ activeInfoCard.subtitle }}</span>
        </span>
      </div>
      <div v-if="activeInfoCard.description" class="spc-info-desc">
        {{ activeInfoCard.description }}
      </div>
      <code class="spc-info-raw">{{ activeInfoCard.raw }}</code>
    </div>
    </el-popover>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, markRaw, nextTick, onBeforeUnmount, onMounted, ref, watch, type Component } from 'vue'
import { Document, EditPen, View } from '@element-plus/icons-vue'
import type { UserInfo } from '@/architecture/domain/types'
import { getUserInfo } from '@/architecture/presentation/context/api/auth'
import { getUsersByUsernames, searchUsersFuzzy } from '@/architecture/presentation/context/api/user'
import {
  getServiceTreeDetail,
  searchResources,
  searchFunctions,
  type ResourceSearchResult,
  type ServiceTreeDetailResp,
} from '@/architecture/presentation/context/api/service-tree'
import UserAvatar from '@/architecture/presentation/shared/components/UserAvatar.vue'
import ChartIcon from '@/architecture/presentation/shared/components/icons/ChartIcon.vue'
import TableIcon from '@/architecture/presentation/shared/components/icons/TableIcon.vue'
import {
  isWorkspaceToolResourcePath,
  insertWorkspaceResourceTokensAtOffset,
  parseWorkspaceInvocationBlocks,
  parseWorkspacePromptSegments,
  resolveWorkspaceResourcePath,
  workspaceToolName,
  wrapWorkspaceResourcePath,
  type WorkspacePromptSegment,
} from './utils/workspaceInvocationSnippet'
import {
  findMiniComposerMentionQuery,
  replaceMiniComposerMention,
  type MiniComposerMentionKind,
  type MiniComposerMentionQuery,
} from './utils/miniComposerMention'

type ComposerMode = 'edit' | 'preview'
type MentionPanelPlacement = 'above' | 'below'

interface StructuredPromptTextSegment {
  type: 'text'
  text: string
  start: number
  end: number
}

interface StructuredPromptUserSegment {
  type: 'user'
  text: string
  username: string
  start: number
  end: number
}

type StructuredPromptResourceSegment = WorkspacePromptSegment & { type: 'resource' }
type StructuredPromptSegment =
  | StructuredPromptTextSegment
  | StructuredPromptUserSegment
  | StructuredPromptResourceSegment

interface StructuredMentionOption {
  key: string
  kind: MiniComposerMentionKind
  label: string
  value: string
  description: string
  typeLabel: string
  resourceType?: ResourceSearchResult['type']
  avatar?: string
  iconSrc?: string
  iconComponent?: Component
  iconClass?: string
  metaItems: string[]
  userMeta?: PromptUserMeta
  resourceMeta?: PromptResourceMeta
}

interface PromptUserMeta {
  username: string
  label: string
  description?: string
  avatar?: string
  metaItems: string[]
}

interface PromptResourceMeta {
  path: string
  label: string
  description?: string
  typeLabel: string
  resourceType?: ResourceSearchResult['type'] | ServiceTreeDetailResp['type'] | string
  templateType?: string
  iconSrc?: string
  iconComponent?: Component
  iconClass?: string
  metaItems: string[]
}

interface PromptInfoCard {
  kind: 'user' | 'resource'
  title: string
  subtitle: string
  description: string
  raw: string
  avatar?: string
  iconSrc?: string
  iconComponent?: Component
  iconClass?: string
}

const SYSTEM_USER_META: PromptUserMeta = {
  username: 'system',
  label: '系统',
  description: '系统内置身份',
  metaItems: ['@system'],
}
const RAW_TABLE_ICON = markRaw(TableIcon)
const RAW_CHART_ICON = markRaw(ChartIcon)

const ResourceTokenIcon = defineComponent({
  name: 'ResourceTokenIcon',
  props: {
    meta: {
      type: Object as () => PromptResourceMeta | undefined,
      default: undefined,
    },
    path: {
      type: String,
      default: '',
    },
  },
  setup(iconProps) {
    return () => {
      const meta = iconProps.meta
      if (meta?.iconSrc) {
        return h('img', {
          class: 'spc-resource-icon-img',
          src: meta.iconSrc,
          alt: meta.typeLabel || '',
        })
      }
      if (meta?.iconComponent) {
        return h(meta.iconComponent as Component, {
          class: 'spc-resource-icon-component',
          size: 15,
        })
      }
      return h('span', {
        class: ['spc-resource-icon-fallback', `is-${resourceKind(iconProps.path)}`],
      })
    }
  },
})

const POST_COMPOSITION_ENTER_SUPPRESS_MS = 260

const props = withDefaults(defineProps<{
  modelValue: string
  placeholder?: string
  disabled?: boolean
  minRows?: number
  maxRows?: number
  showToolbar?: boolean
  compact?: boolean
  submitOnEnter?: boolean
  enableMentions?: boolean
  mentionPanelPlacement?: MentionPanelPlacement
  readonlyPreview?: boolean
  editorTestId?: string
  fullCodePath?: string
}>(), {
  placeholder: '输入任务，可粘贴 </path> 或 <./path> 资源引用',
  disabled: false,
  minRows: 4,
  maxRows: 12,
  showToolbar: true,
  compact: false,
  submitOnEnter: false,
  enableMentions: true,
  mentionPanelPlacement: 'below',
  readonlyPreview: false,
  editorTestId: 'structured-prompt-editor',
  fullCodePath: '',
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'focus'): void
  (e: 'blur'): void
  (e: 'enter', event: KeyboardEvent): void
}>()

const rootRef = ref<HTMLElement | null>(null)
const editorRef = ref<HTMLElement | null>(null)
const mode = ref<ComposerMode>(props.readonlyPreview ? 'preview' : 'edit')
const focused = ref(false)
const currentText = ref(props.modelValue)
const mentionQuery = ref<MiniComposerMentionQuery | null>(null)
const rawMentionOptions = ref<StructuredMentionOption[]>([])
const resourceScope = ref<'current' | 'other'>('current')
const resourceType = ref('all')
const resourceTypes = [
  { value: 'all', label: '全部' }, { value: 'docs', label: '文档' },
  { value: 'package', label: '目录' }, { value: 'table', label: '数据表' },
  { value: 'form', label: '表单' }, { value: 'chart', label: '图表' },
  { value: 'other', label: '其他' },
]
const currentDirectory = computed(() => {
  const path = props.fullCodePath.replace(/\/$/, '')
  return /\.(?:docs|table|form|chart|function|workflow|page)$/.test(path) ? path.slice(0, path.lastIndexOf('/')) : path
})
const mentionSearchError = ref(false)
const mentionHasMore = ref(false)
let mentionPage = 1
let mentionRetryPage = 1
const mentionOptions = computed(() => rawMentionOptions.value.filter(option => {
  if (option.kind === 'user') return true
  const local = !!currentDirectory.value && (option.value === currentDirectory.value || option.value.startsWith(`${currentDirectory.value}/`))
  if (currentDirectory.value && (resourceScope.value === 'current' ? !local : local)) return false
  const kind = option.resourceType === 'function'
    ? (option.resourceMeta?.templateType || option.value.split('.').pop() || '')
    : option.resourceType
  return resourceType.value === 'all' || (resourceType.value === 'other'
    ? !['docs', 'package', 'table', 'form', 'chart'].includes(kind || '')
    : kind === resourceType.value)
}))
const mentionLoading = ref(false)
const highlightedMentionIndex = ref(0)
const mentionAnchorRect = ref<DOMRect>(createVirtualRect())
const composing = ref(false)
const userMetaByUsername = ref<Record<string, PromptUserMeta>>({ system: SYSTEM_USER_META })
const resourceMetaByPath = ref<Record<string, PromptResourceMeta>>({})
const activeInfoCard = ref<PromptInfoCard | null>(null)
const infoCardRef = ref<HTMLElement | null>(null)
const infoCardAnchor = ref<HTMLElement | null>(null)
let renderTimer: ReturnType<typeof setTimeout> | null = null
let mentionSearchTimer: ReturnType<typeof setTimeout> | null = null
let mentionCloseTimer: ReturnType<typeof setTimeout> | null = null
let metadataHydrateTimer: ReturnType<typeof setTimeout> | null = null
let mentionSearchSeq = 0
let activeMentionSearchKey = ''
let pendingMentionCommitKey = ''
let metadataHydrateSeq = 0
let rendering = false
let lastCompositionEndAt = 0
let lastCaretOffset = props.modelValue.length

const structuredSegments = computed(() => parseStructuredPromptSegments(currentText.value))
const resourceSegments = computed(() => structuredSegments.value.filter((segment): segment is StructuredPromptResourceSegment => segment.type === 'resource'))
const invocationBlocks = computed(() => parseWorkspaceInvocationBlocks(currentText.value, props.fullCodePath))
const mentionPanelOpen = computed(() => props.enableMentions && mode.value === 'edit' && !!mentionQuery.value)
const mentionPopoverPlacement = computed(() => props.mentionPanelPlacement === 'above' ? 'top-start' : 'bottom-start')
const mentionPopoverWidth = computed(() => {
  const rootWidth = rootRef.value?.getBoundingClientRect().width || 360
  return mentionQuery.value?.kind === 'resource' ? Math.min(720, window.innerWidth - 24) : Math.max(300, Math.min(rootWidth, 520))
})
const mentionVirtualRef = {
  getBoundingClientRect: () => mentionAnchorRect.value,
  get contextElement() {
    return editorRef.value || rootRef.value || undefined
  },
}
const mentionModeLabel = computed(() => mentionQuery.value?.kind === 'user' ? '选择用户' : '选择资源')
const mentionEmptyText = computed(() => {
  const query = mentionQuery.value
  if (!query) return ''
  if (mentionSearchError.value) return '加载失败，请重试'
  if (mentionHasMore.value) return '已加载的资源中暂无匹配项，可加载更多或继续输入名称搜索'
  if (!query.query.trim() && query.kind === 'user') return '未获取到当前用户，请输入用户名搜索'
  return query.kind === 'user' ? '没有匹配的用户' : '没有匹配的资源'
})
const editorStyle = computed(() => {
  const lineHeight = props.compact ? 20 : 24
  const verticalPadding = props.compact ? 8 : 28
  const minRows = Math.max(1, props.minRows)
  const maxRows = Math.max(minRows, props.maxRows)
  return {
    minHeight: `${minRows * lineHeight + verticalPadding}px`,
    maxHeight: `${maxRows * lineHeight + verticalPadding}px`,
    textAlign: 'left' as const,
  }
})

watch(() => props.modelValue, (value) => {
  if (value === currentText.value) return
  currentText.value = value
  if (rendering) return
  const editor = editorRef.value
  if (!editor) return
  if (serializeEditorContent(editor) === value) return
  renderEditorContentPreservingCaret(value)
  scheduleMetadataHydration()
})

watch(() => props.readonlyPreview, (readonlyPreview) => {
  if (readonlyPreview) {
    mode.value = 'preview'
  } else if (mode.value === 'preview' && !props.showToolbar) {
    mode.value = 'edit'
  }
})

onMounted(() => {
  renderEditorContent(currentText.value)
  scheduleMetadataHydration()
  document.addEventListener('mousedown', handleDocumentMouseDown)
})

onBeforeUnmount(() => {
  clearRenderTimer()
  resetMentionSearch()
  cancelMentionClose()
  clearMetadataHydration()
  document.removeEventListener('mousedown', handleDocumentMouseDown)
})

function setMode(nextMode: ComposerMode) {
  if (props.readonlyPreview && nextMode === 'edit') {
    return
  }
  mode.value = nextMode
  closeMentionPanel()
  if (nextMode === 'edit') {
    void nextTick(() => focus())
  }
}

function handleRootClick(event: MouseEvent) {
  if (props.disabled || mode.value !== 'edit') return
  const target = event.target as HTMLElement
  if (target.closest('button') || target.closest('a') || target.closest('.spc-toolbar') || target.closest('.spc-user-chip') || target.closest('.spc-resource-chip')) {
    return
  }
  if (target.closest('.spc-editor')) {
    return
  }
  focus()
}

function handleEditorInput(event?: Event) {
  if (rendering || props.disabled) return
  if (composing.value || isComposingInputEvent(event)) return
  const editor = editorRef.value
  if (!editor) return
  syncTrailingLineBreak(editor)
  commitText(serializeEditorContent(editor))
  rememberCaretPosition(editor)
  keepCaretVisible(editor)
  closeInfoCard()
  updateMentionFromEditor()
  scheduleTokenRender()
  scheduleMetadataHydration()
}

function handlePaste(event: ClipboardEvent) {
  const text = event.clipboardData?.getData('text/plain')
  if (!text) return
  event.preventDefault()
  insertTextAtCaret(text)
}

function handleEditorKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && activeInfoCard.value) {
    event.preventDefault()
    event.stopPropagation()
    closeInfoCard()
    return
  }
  if (props.disabled) {
    return
  }
  if (isIMEComposing(event)) {
    return
  }
  if (shouldSuppressPostCompositionEnter(event)) {
    updateMentionFromEditor()
    commitMentionSelectionOrQueue()
    event.preventDefault()
    return
  }

  if (isMentionCommitKey(event)) {
    updateMentionFromEditor()
    if (commitMentionSelectionOrQueue()) {
      event.preventDefault()
      return
    }
  }

  if (mentionPanelOpen.value) {
    if (event.key === 'Escape') {
      event.preventDefault()
      closeMentionPanel()
      return
    }
    if (mentionOptions.value.length > 0) {
      if (event.key === 'ArrowDown') {
        event.preventDefault()
        moveMentionHighlight(1)
        return
      }
      if (event.key === 'ArrowUp') {
        event.preventDefault()
        moveMentionHighlight(-1)
        return
      }
    }
    if (isMentionCommitKey(event)) {
      event.preventDefault()
      commitMentionSelectionOrQueue()
      return
    }
  }

  if (event.key === 'Enter') {
    if (props.submitOnEnter && !event.shiftKey) {
      event.preventDefault()
      emit('enter', event)
      return
    }
    event.preventDefault()
    insertTextAtCaret('\n')
  }
}

function handleEditorCursorChange(event?: Event) {
  if (composing.value) {
    return
  }
  if (
    event instanceof KeyboardEvent
    && (
      isIMEComposing(event)
      || (
        mentionPanelOpen.value
        && ['ArrowDown', 'ArrowUp', 'Enter', 'Tab', 'Escape'].includes(event.key)
      )
    )
  ) {
    return
  }
  rememberCaretPosition()
  updateMentionFromEditor()
}

function handleEditorClick(event: MouseEvent) {
  handleEditorCursorChange(event)
  const token = event.target instanceof HTMLElement
    ? event.target.closest('.spc-editor-token')
    : null
  if (!(token instanceof HTMLElement) || !editorRef.value?.contains(token)) {
    closeInfoCard()
    return
  }

  const username = token.dataset.username || ''
  const path = token.dataset.path || ''
  if (username) {
    openInfoCardForUser(username, event)
  } else if (path) {
    openInfoCardForResource(path, event)
  }
}

function handleFocus() {
  focused.value = true
  cancelMentionClose()
  emit('focus')
  void nextTick(() => updateMentionFromEditor())
}

function handleBlur() {
  rememberCaretPosition()
  focused.value = false
  scheduleMentionClose()
  renderEditorContent(currentText.value)
  if (!isFocusInsideRoot()) {
    closeInfoCard()
  }
  emit('blur')
}

function handleRootFocusOut(event: FocusEvent) {
  const root = rootRef.value
  const nextTarget = event.relatedTarget
  if (nextTarget instanceof Node && (root?.contains(nextTarget) || infoCardRef.value?.contains(nextTarget))) {
    return
  }
  closeInfoCard()
}

function isFocusInsideRoot() {
  const root = rootRef.value
  const active = document.activeElement
  return active instanceof Node && (!!root?.contains(active) || !!infoCardRef.value?.contains(active))
}

function onCompositionStart() {
  composing.value = true
  clearRenderTimer()
}

function onCompositionEnd() {
  composing.value = false
  lastCompositionEndAt = getNow()
  void nextTick(() => {
    if (!composing.value) {
      handleEditorInput()
    }
  })
}

function isIMEComposing(event: KeyboardEvent) {
  return composing.value
    || event.isComposing
    || event.key === 'Process'
    || (event as KeyboardEvent & { keyCode?: number }).keyCode === 229
}

function isComposingInputEvent(event?: Event) {
  return !!event && 'isComposing' in event && Boolean((event as InputEvent).isComposing)
}

function shouldSuppressPostCompositionEnter(event: KeyboardEvent) {
  if (event.key !== 'Enter') return false
  if (!lastCompositionEndAt) return false
  return getNow() - lastCompositionEndAt < POST_COMPOSITION_ENTER_SUPPRESS_MS
}

function isMentionCommitKey(event: KeyboardEvent) {
  return event.key === 'Enter' || event.key === 'Tab'
}

function getNow() {
  return typeof performance !== 'undefined' ? performance.now() : Date.now()
}

function commitText(value: string) {
  currentText.value = value
  emit('update:modelValue', value)
}

function scheduleTokenRender() {
  clearRenderTimer()
  renderTimer = setTimeout(() => {
    const editor = editorRef.value
    if (!editor || focused.value === false) return
    if (composing.value) return
    if (mentionQuery.value) return
    if (window.getSelection()?.isCollapsed === false) return
    const offset = getCaretTextOffset(editor)
    const text = serializeEditorContent(editor)
    currentText.value = text
    if (!needsTokenRender(editor, text)) return
    const scrollTop = editor.scrollTop
    renderEditorContent(text)
    restoreCaretTextOffset(editor, offset)
    editor.scrollTop = scrollTop
    updateMentionFromEditor()
  }, 260)
}

function needsTokenRender(editor: HTMLElement, text: string) {
  const renderedTokens = Array.from(editor.querySelectorAll<HTMLElement>('.spc-editor-token'))
    .map(token => token.dataset.tokenRaw || '')
    .filter(Boolean)
  const parsedTokens = parseStructuredPromptSegments(text)
    .filter(segment => segment.type !== 'text')
    .map(segment => segment.text)
  return renderedTokens.length !== parsedTokens.length
    || renderedTokens.some((token, index) => token !== parsedTokens[index])
}

function clearRenderTimer() {
  if (renderTimer) {
    clearTimeout(renderTimer)
    renderTimer = null
  }
}

function renderEditorContent(text: string) {
  const editor = editorRef.value
  if (!editor) return

  rendering = true
  editor.replaceChildren(...buildEditorNodes(text))
  syncTrailingLineBreak(editor)
  rendering = false
}

function renderEditorContentPreservingCaret(text: string) {
  const editor = editorRef.value
  if (!editor) return
  const offset = focused.value ? getActiveOrRememberedCaretOffset(editor, text.length) : null
  // Do not destroy a non-collapsed selection or an IME composition for display-only metadata.
  if (composing.value) return
  const scrollTop = editor.scrollTop
  const selection = window.getSelection()
  if (focused.value && selection && !selection.isCollapsed) return
  renderEditorContent(text)
  if (offset !== null) {
    lastCaretOffset = offset
    restoreCaretTextOffset(editor, offset)
  }
  editor.scrollTop = scrollTop
}

// A trailing newline needs a final BR for the browser to paint its empty line/caret.
// It is presentation-only and must never become part of the prompt.
function keepCaretVisible(editor: HTMLElement) {
  const selection = window.getSelection()
  if (!selection?.rangeCount || !selection.isCollapsed) return
  const range = selection.getRangeAt(0)
  if (!isRangeInside(editor, range)) return
  let rect = Array.from(range.getClientRects?.() || []).find(rect => rect.height > 0)
  if (!rect && getCaretTextOffset(editor) === serializeEditorContent(editor).length) {
    rect = editor.querySelector('[data-caret-trailing-break]')?.getBoundingClientRect()
  }
  if (!rect) return
  const bounds = editor.getBoundingClientRect()
  if (rect.bottom > bounds.bottom - 4) editor.scrollTop += rect.bottom - bounds.bottom + 4
  else if (rect.top < bounds.top + 4) editor.scrollTop -= bounds.top + 4 - rect.top
}

function syncTrailingLineBreak(editor: HTMLElement) {
  const marker = editor.querySelector('[data-caret-trailing-break]')
  const needsBreak = serializeEditorContent(editor).endsWith('\n')
  if (!needsBreak) { marker?.remove(); return }
  if (marker === editor.lastChild) return
  marker?.remove()
  const br = document.createElement('br')
  br.dataset.caretTrailingBreak = 'true'
  editor.appendChild(br)
}

function buildEditorNodes(text: string): Node[] {
  const nodes: Node[] = []
  parseStructuredPromptSegments(text).forEach((segment) => {
    if (segment.type === 'resource') {
      nodes.push(createResourceTokenNode(segment))
    } else if (segment.type === 'user') {
      nodes.push(createUserTokenNode(segment))
    } else if (segment.text) {
      nodes.push(document.createTextNode(segment.text))
    }
  })
  if (nodes.length === 0) {
    nodes.push(document.createTextNode(''))
  }
  return nodes
}

function createResourceTokenNode(segment: StructuredPromptResourceSegment) {
  const chip = document.createElement('span')
  chip.className = `spc-editor-token is-resource is-${resourceKind(segment.path || '')}`
  chip.contentEditable = 'false'
  chip.dataset.tokenRaw = segment.text
  chip.dataset.path = segment.path || ''
  const meta = getResourceMeta(segment.path || '')
  const icon = createResourceIconElement(meta, segment.path || '')
  const label = document.createElement('span')
  label.className = 'spc-editor-token-label'
  label.textContent = getResourceDisplayLabel(segment.path || segment.text)
  chip.replaceChildren(icon, label)
  return chip
}

function createUserTokenNode(segment: StructuredPromptUserSegment) {
  const chip = document.createElement('span')
  chip.className = 'spc-editor-token is-user'
  chip.contentEditable = 'false'
  chip.dataset.tokenRaw = segment.text
  chip.dataset.username = segment.username
  chip.textContent = getUserTokenLabel(segment.username)
  return chip
}

function serializeEditorContent(root: HTMLElement): string {
  let text = ''
  root.childNodes.forEach((node) => {
    text += serializeNode(node)
  })
  return text
}

function serializeNode(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) {
    return node.textContent || ''
  }
  if (node instanceof HTMLElement) {
    if (node.dataset.tokenRaw) {
      return node.dataset.tokenRaw
    }
    let text = ''
    node.childNodes.forEach((child) => {
      text += serializeNode(child)
    })
    return text
  }
  return node.textContent || ''
}

function insertTextAtCaret(text: string, preferredOffset?: number) {
  if (props.disabled) return
  mode.value = 'edit'
  const editor = editorRef.value
  if (!editor) return
  const source = serializeEditorContent(editor)
  const fallbackOffset = Math.min(preferredOffset ?? lastCaretOffset, source.length)
  editor.focus()

  const selection = window.getSelection()
  const range = selection?.rangeCount ? selection.getRangeAt(0) : null
  if (!selection || !range || !isRangeInside(editor, range)) {
    const value = `${source.slice(0, fallbackOffset)}${text}${source.slice(fallbackOffset)}`
    lastCaretOffset = fallbackOffset + text.length
    commitText(value)
    renderEditorContent(value)
    void nextTick(() => {
      editor.focus()
      restoreCaretTextOffset(editor, lastCaretOffset)
      updateMentionFromEditor()
      scheduleTokenRender()
    })
    return
  }

  range.deleteContents()
  const node = document.createTextNode(text)
  range.insertNode(node)
  range.setStart(node, text.length)
  range.collapse(true)
  selection.removeAllRanges()
  selection.addRange(range)
  lastCaretOffset = getCaretTextOffset(editor)
  handleEditorInput()
}

function insertWorkspaceResources(paths: string[]) {
  if (props.disabled || paths.length === 0) return
  const editor = editorRef.value
  const source = editor ? serializeEditorContent(editor) : currentText.value
  const offset = editor ? getActiveOrRememberedCaretOffset(editor, source.length) : source.length
  const result = insertWorkspaceResourceTokensAtOffset(source, paths, offset, props.fullCodePath)
  if (!result.insertedText) return
  insertTextAtCaret(result.insertedText, offset)
}

function isRangeInside(root: HTMLElement, range: Range) {
  const container = range.commonAncestorContainer
  return container === root || root.contains(container)
}

function getCaretTextOffset(root: HTMLElement): number {
  const selection = window.getSelection()
  if (!selection || selection.rangeCount === 0) return serializeEditorContent(root).length
  const range = selection.getRangeAt(0)
  if (!isRangeInside(root, range)) return serializeEditorContent(root).length
  const preRange = range.cloneRange()
  preRange.selectNodeContents(root)
  preRange.setEnd(range.endContainer, range.endOffset)
  const fragment = preRange.cloneContents()
  const container = document.createElement('div')
  container.appendChild(fragment)
  return serializeEditorContent(container).length
}

function getActiveOrRememberedCaretOffset(root: HTMLElement, textLength: number) {
  const selection = window.getSelection()
  const range = selection?.rangeCount ? selection.getRangeAt(0) : null
  if (range && isRangeInside(root, range)) {
    return Math.min(getCaretTextOffset(root), textLength)
  }
  return Math.min(lastCaretOffset, textLength)
}

function rememberCaretPosition(root = editorRef.value) {
  if (!root) return
  const selection = window.getSelection()
  const range = selection?.rangeCount ? selection.getRangeAt(0) : null
  if (range && isRangeInside(root, range)) {
    lastCaretOffset = getCaretTextOffset(root)
  }
}

function createVirtualRect(left = 0, top = 0, width = 1, height = 24): DOMRect {
  if (typeof DOMRect !== 'undefined') {
    return new DOMRect(left, top, width, height)
  }
  return {
    x: left,
    y: top,
    left,
    top,
    width,
    height,
    right: left + width,
    bottom: top + height,
    toJSON: () => ({}),
  } as DOMRect
}

function normalizeClientRect(rect: DOMRect | DOMRectReadOnly): DOMRect {
  return createVirtualRect(
    rect.left,
    rect.top,
    Math.max(1, rect.width || 1),
    Math.max(20, rect.height || 20)
  )
}

function isUsableClientRect(rect: DOMRect | DOMRectReadOnly | undefined): rect is DOMRect | DOMRectReadOnly {
  return !!rect
    && Number.isFinite(rect.left)
    && Number.isFinite(rect.top)
    && (rect.width > 0 || rect.height > 0 || rect.left > 0 || rect.top > 0)
}

function getMentionAnchorRect(editor: HTMLElement): DOMRect {
  const selection = window.getSelection()
  if (selection?.rangeCount) {
    const range = selection.getRangeAt(0)
    if (isRangeInside(editor, range)) {
      const collapsedRange = range.cloneRange()
      collapsedRange.collapse(true)
      const clientRect = Array.from(collapsedRange.getClientRects()).find(isUsableClientRect)
        || collapsedRange.getBoundingClientRect()
      if (isUsableClientRect(clientRect)) {
        return normalizeClientRect(clientRect)
      }
    }
  }

  const editorRect = editor.getBoundingClientRect()
  const fallbackTop = props.mentionPanelPlacement === 'above'
    ? editorRect.top
    : Math.min(editorRect.bottom, editorRect.top + 44)
  return createVirtualRect(editorRect.left + 12, fallbackTop, 1, 24)
}

function restoreCaretTextOffset(root: HTMLElement, targetOffset: number) {
  const range = document.createRange()
  const selection = window.getSelection()
  let consumed = 0
  let placed = false

  const place = (node: Node, offset: number) => {
    range.setStart(node, offset)
    range.collapse(true)
    placed = true
  }

  root.childNodes.forEach((node) => {
    if (placed) return
    const tokenRaw = node instanceof HTMLElement ? node.dataset.tokenRaw : ''
    const length = tokenRaw ? tokenRaw.length : (node.textContent || '').length
    if (targetOffset <= consumed + length) {
      if (node.nodeType === Node.TEXT_NODE) {
        place(node, Math.max(0, targetOffset - consumed))
      } else if (targetOffset - consumed <= Math.floor(length / 2)) {
        range.setStartBefore(node)
        range.collapse(true)
        placed = true
      } else {
        range.setStartAfter(node)
        range.collapse(true)
        placed = true
      }
    }
    consumed += length
  })

  if (!placed) {
    range.selectNodeContents(root)
    range.collapse(false)
  }

  selection?.removeAllRanges()
  selection?.addRange(range)
}

function updateMentionFromEditor() {
  cancelMentionClose()
  if (!props.enableMentions || props.disabled || mode.value !== 'edit') {
    closeMentionPanel()
    return
  }

  const editor = editorRef.value
  if (!editor) {
    closeMentionPanel()
    return
  }

  const text = serializeEditorContent(editor)
  const query = findMiniComposerMentionQuery(text, getCaretTextOffset(editor))
  const previousSearchKey = mentionQuery.value ? getMentionSearchKey(mentionQuery.value) : ''
  const nextSearchKey = query ? getMentionSearchKey(query) : ''
  if (query?.kind === 'resource' && mentionQuery.value?.kind !== 'resource') {
    resourceScope.value = 'current'
    resourceType.value = 'all'
  }
  if (query) {
    mentionAnchorRect.value = getMentionAnchorRect(editor)
  }
  mentionQuery.value = query
  if (previousSearchKey !== nextSearchKey) {
    highlightedMentionIndex.value = 0
  }

  if (!query) {
    pendingMentionCommitKey = ''
    resetMentionSearch()
    return
  }

  scheduleMentionSearch(query)
}

function resetMentionSearch() {
  if (mentionSearchTimer) {
    clearTimeout(mentionSearchTimer)
    mentionSearchTimer = null
  }
  mentionSearchSeq += 1
  activeMentionSearchKey = ''
  pendingMentionCommitKey = ''
  mentionLoading.value = false
  mentionHasMore.value = false
  mentionSearchError.value = false
  rawMentionOptions.value = []
}

function getMentionSearchKey(query: MiniComposerMentionQuery) {
  return `${query.kind}:${query.query.trim()}:${query.kind === 'resource' ? `${currentDirectory.value}:${resourceScope.value}:${resourceType.value}` : ''}`
}

function scheduleMentionSearch(query: MiniComposerMentionQuery) {
  const searchKey = getMentionSearchKey(query)
  if (searchKey === activeMentionSearchKey) {
    return
  }

  activeMentionSearchKey = searchKey
  if (mentionSearchTimer) {
    clearTimeout(mentionSearchTimer)
  }
  mentionLoading.value = true
  mentionHasMore.value = false
  mentionPage = 1
  pendingMentionCommitKey = ''
  rawMentionOptions.value = []
  mentionSearchTimer = setTimeout(() => {
    void runMentionSearch(query, searchKey)
  }, 220)
}

async function runMentionSearch(query: MiniComposerMentionQuery, searchKey: string, page = 1) {
  const currentSeq = ++mentionSearchSeq
  mentionLoading.value = true
  mentionSearchError.value = false

  try {
    if (query.kind === 'user') {
      const keyword = query.query.trim()
      const users = keyword ? (await searchUsersFuzzy(keyword, 8)).users || [] : [await getUserInfo()]
      if (currentSeq !== mentionSearchSeq || activeMentionSearchKey !== searchKey) return
      rawMentionOptions.value = users.filter(user => !!user?.username).map(user => ({
        ...mapUserMentionOption(user),
        ...(!keyword ? { typeLabel: '我自己' } : {}),
      }))
    } else {
      const scopePath = resourceScope.value === 'current' ? currentDirectory.value : ''
      const functionType = ['table', 'form', 'chart'].includes(resourceType.value)
      const response = functionType
        ? await searchFunctions({ user: '', app: '', keyword: query.query.trim(), full_code_path: scopePath,
          template_type: resourceType.value, page, page_size: 100 }).then(result => ({
          items: (result.functions || []).map(item => ({ ...item, type: 'function' as const })),
        }))
        : await searchResources({
          keyword: query.query.trim(),
          full_code_path: scopePath,
          resource_type: resourceType.value === 'docs' || resourceType.value === 'package' ? resourceType.value : resourceType.value === 'all' ? 'all' : 'function',
          page, page_size: 100,
        })
      if (currentSeq !== mentionSearchSeq || activeMentionSearchKey !== searchKey) return
      const options = (response.items || []).map(mapResourceMentionOption)
      rawMentionOptions.value = page === 1 ? options : Array.from(new Map([...rawMentionOptions.value, ...options].map(option => [option.key, option])).values())
      mentionPage = page
      mentionHasMore.value = (response.items || []).length === 100
    }
  } catch {
    if (currentSeq === mentionSearchSeq && activeMentionSearchKey === searchKey) {
      mentionSearchError.value = true
      mentionRetryPage = page
    }
  } finally {
    if (currentSeq === mentionSearchSeq && activeMentionSearchKey === searchKey) {
      mentionLoading.value = false
      highlightedMentionIndex.value = 0
      applyPendingMentionCommit(searchKey)
    }
  }
}

function changeResourceFilter(scope: 'current' | 'other', type: string) {
  resourceScope.value = scope
  resourceType.value = type
  highlightedMentionIndex.value = 0
  if (mentionQuery.value) scheduleMentionSearch(mentionQuery.value)
}

function loadMoreMentions() {
  if (!mentionQuery.value || mentionLoading.value) return
  void runMentionSearch(mentionQuery.value, getMentionSearchKey(mentionQuery.value), mentionSearchError.value ? mentionRetryPage : mentionPage + 1)
}

watch(currentDirectory, () => {
  if (mentionQuery.value?.kind === 'resource') changeResourceFilter('current', 'all')
})

function queueMentionCommit() {
  const query = mentionQuery.value
  if (!query) {
    return false
  }
  pendingMentionCommitKey = getMentionSearchKey(query)
  return true
}

function commitMentionSelectionOrQueue() {
  const query = mentionQuery.value
  if (!query) {
    return false
  }
  const option = mentionOptions.value[highlightedMentionIndex.value] || mentionOptions.value[0]
  if (option) {
    applyMentionOption(option)
    return true
  }
  return queueMentionCommit()
}

function applyPendingMentionCommit(searchKey: string) {
  if (pendingMentionCommitKey !== searchKey) {
    return
  }
  pendingMentionCommitKey = ''
  const firstOption = mentionOptions.value[0]
  if (firstOption) {
    applyMentionOption(firstOption)
  }
}

function mapUserMentionOption(user: UserInfo): StructuredMentionOption {
  const meta = mapUserInfoToMeta(user)
  const username = meta.username

  return {
    key: `user:${username}`,
    kind: 'user',
    label: meta.label,
    value: username,
    description: meta.description || '',
    typeLabel: '用户',
    avatar: user.avatar,
    metaItems: meta.metaItems,
    userMeta: meta,
  }
}

function mapResourceMentionOption(resource: ResourceSearchResult): StructuredMentionOption {
  const path = resource.full_code_path || ''
  const description = cleanMentionText(resource.description)
    || cleanMentionText(resource.snippet)
    || getReadablePath(path)
  const meta = mapResourceSearchResultToMeta(resource)

  return {
    key: `resource:${resource.type}:${resource.id}:${path}`,
    kind: 'resource',
    label: meta.label,
    value: path,
    description,
    typeLabel: meta.typeLabel,
    resourceType: resource.type,
    metaItems: meta.metaItems,
    resourceMeta: meta,
    iconSrc: meta.iconSrc,
    iconComponent: meta.iconComponent,
    iconClass: meta.iconClass,
  }
}

function moveMentionHighlight(delta: number) {
  const count = mentionOptions.value.length
  if (count === 0) return
  highlightedMentionIndex.value = (highlightedMentionIndex.value + delta + count) % count
  void nextTick(scrollHighlightedMentionIntoView)
}

function scrollHighlightedMentionIntoView() {
  const panel = document.querySelector<HTMLElement>('[data-testid="structured-prompt-mention-panel"]')
  const list = panel?.querySelector<HTMLElement>('.spc-mention-list')
  const option = panel?.querySelector<HTMLElement>('.spc-mention-option.is-active')
  if (!list || !option) return

  const optionTop = option.offsetTop
  const optionBottom = optionTop + option.offsetHeight
  if (optionTop < list.scrollTop) {
    list.scrollTop = optionTop
  } else if (optionBottom > list.scrollTop + list.clientHeight) {
    list.scrollTop = optionBottom - list.clientHeight
  }
}

function applyMentionOption(option: StructuredMentionOption | undefined) {
  if (!option || !mentionQuery.value) {
    return
  }

  const replacement = option.kind === 'user'
    ? `@${option.value}`
    : wrapWorkspaceResourcePath(option.value)
  const result = replaceMiniComposerMention(currentText.value, mentionQuery.value, replacement)

  if (option.userMeta?.username) {
    userMetaByUsername.value = {
      ...userMetaByUsername.value,
      [option.userMeta.username]: option.userMeta,
    }
  }
  if (option.resourceMeta?.path) {
    resourceMetaByPath.value = {
      ...resourceMetaByPath.value,
      [option.resourceMeta.path]: option.resourceMeta,
    }
  }

  commitText(result.value)
  renderEditorContent(result.value)
  closeMentionPanel()
  scheduleMetadataHydration()

  void nextTick(() => {
    const editor = editorRef.value
    if (!editor) return
    editor.focus()
    restoreCaretTextOffset(editor, result.cursor)
  })
}

function closeMentionPanel() {
  mentionQuery.value = null
  resetMentionSearch()
}

function scheduleMentionClose() {
  cancelMentionClose()
  mentionCloseTimer = setTimeout(() => {
    closeMentionPanel()
  }, 160)
}

function cancelMentionClose() {
  if (mentionCloseTimer) {
    clearTimeout(mentionCloseTimer)
    mentionCloseTimer = null
  }
}

function getUserTokenLabel(username: string) {
  const normalized = normalizeMentionUsername(username)
  const meta = getUserMeta(normalized)
  const label = normalizeUserDisplayLabel(normalized, meta?.label || '')
  return label
    ? `@${normalized}(${label})`
    : `@${normalized}`
}

function getUserMeta(username: string): PromptUserMeta | undefined {
  const normalized = normalizeMentionUsername(username)
  return userMetaByUsername.value[normalized] || (normalized === 'system' ? SYSTEM_USER_META : undefined)
}

function getResourceMeta(path: string): PromptResourceMeta | undefined {
  const normalized = normalizeResourcePathForMeta(path)
  return normalized ? resourceMetaByPath.value[normalized] : undefined
}

function getResourceDisplayLabel(pathOrToken: string) {
  const path = normalizeResourcePathForMeta(pathOrToken)
  const meta = path ? getResourceMeta(path) : undefined
  return meta?.label || resourceDisplayName(pathOrToken)
}

function createResourceIconElement(meta: PromptResourceMeta | undefined, path: string) {
  if (meta?.iconSrc) {
    const img = document.createElement('img')
    img.className = 'spc-resource-icon-img'
    img.src = meta.iconSrc
    img.alt = meta.typeLabel || ''
    return img
  }

  const icon = document.createElement('span')
  icon.className = `spc-resource-icon-fallback is-${resourceKind(path)} ${meta?.iconClass || ''}`.trim()
  return icon
}

function scheduleMetadataHydration() {
  if (import.meta.env.MODE === 'test') {
    return
  }
  clearMetadataHydration()
  metadataHydrateTimer = setTimeout(() => {
    void hydrateVisibleMetadata()
  }, 320)
}

function clearMetadataHydration() {
  if (metadataHydrateTimer) {
    clearTimeout(metadataHydrateTimer)
    metadataHydrateTimer = null
  }
}

async function hydrateVisibleMetadata() {
  const currentSeq = ++metadataHydrateSeq
  const userNames = Array.from(new Set(
    structuredSegments.value
      .filter((segment): segment is StructuredPromptUserSegment => segment.type === 'user')
      .map((segment) => segment.username)
      .filter((username) => username && !getUserMeta(username))
  )).slice(0, 40)
  const resourcePaths = Array.from(new Set(
    structuredSegments.value
      .filter((segment): segment is StructuredPromptResourceSegment => segment.type === 'resource')
      .map((segment) => normalizeResourcePathForMeta(segment.path || segment.text))
      .filter((path) => path && isHydratableResourcePath(path) && !resourceMetaByPath.value[path])
  )).slice(0, 20)

  await Promise.all([
    hydrateUserMetadata(userNames, currentSeq),
    hydrateResourceMetadata(resourcePaths, currentSeq),
  ])
}

async function hydrateUserMetadata(usernames: string[], seq: number) {
  if (usernames.length === 0) return
  try {
    const response = await getUsersByUsernames(usernames)
    if (seq !== metadataHydrateSeq) return
    const next = { ...userMetaByUsername.value }
    const users = response.users || []
    users.forEach((user) => {
      const meta = mapUserInfoToMeta(user)
      if (meta.username) {
        next[meta.username] = meta
      }
    })
    userMetaByUsername.value = next
    if (!focused.value) renderEditorContentPreservingCaret(currentText.value)
  } catch {
    // Metadata is display-only. Keep raw tokens if lookup fails.
  }
}

async function hydrateResourceMetadata(paths: string[], seq: number) {
  if (paths.length === 0) return
  try {
    const details = await Promise.all(paths.map(async (path) => {
      try {
        return await getServiceTreeDetail(path)
      } catch {
        return null
      }
    }))
    if (seq !== metadataHydrateSeq) return
    const next = { ...resourceMetaByPath.value }
    details.forEach((detail, index) => {
      const fallbackPath = paths[index] || ''
      const meta = detail
        ? mapResourceDetailToMeta(detail)
        : createFallbackResourceMeta(fallbackPath)
      next[meta.path] = meta
    })
    resourceMetaByPath.value = next
    if (!focused.value) renderEditorContentPreservingCaret(currentText.value)
  } catch {
    // Metadata is display-only. Keep raw tokens if lookup fails.
  }
}

function mapUserInfoToMeta(user: UserInfo): PromptUserMeta {
  const username = user.username || ''
  const department = user.department_full_name_path || user.department_name || ''
  const signature = cleanMentionText(user.signature)
  const email = cleanMentionText(user.email)
  const label = normalizeUserDisplayLabel(username, user.nickname || '')
  return {
    username,
    label: label || username,
    description: email || signature || username,
    avatar: user.avatar,
    metaItems: compactMetaItems([
      username ? `@${username}` : '',
      department,
    ]),
  }
}

function mapResourceSearchResultToMeta(resource: ResourceSearchResult): PromptResourceMeta {
  const path = normalizeResourcePathForMeta(resource.full_code_path || '')
  return {
    path,
    label: resource.name || resource.code || getPathTail(path) || path,
    description: cleanMentionText(resource.description) || cleanMentionText(resource.snippet),
    typeLabel: getResourceTypeLabel(resource),
    resourceType: resource.type,
    templateType: resource.template_type,
    metaItems: compactMetaItems([
      getReadablePath(path),
      resource.match_source ? `命中 ${getMatchSourceLabel(resource.match_source)}` : '',
      shouldShowResourceHeat(resource) ? `${formatCompactCount(resource.run_count || 0)} 次运行` : '',
    ]),
    ...getResourceIconMeta(resource),
  }
}

function mapResourceDetailToMeta(detail: ServiceTreeDetailResp): PromptResourceMeta {
  const path = normalizeResourcePathForMeta(detail.full_code_path || '')
  const resourceLike: ResourceSearchResult = {
    id: detail.id,
    name: detail.name,
    code: detail.code,
    type: detail.type,
    full_code_path: path,
    description: detail.description,
    tags: detail.tags,
    template_type: detail.template_type,
    app_id: detail.app_id,
    run_count: detail.run_count,
  }
  return {
    ...mapResourceSearchResultToMeta(resourceLike),
    metaItems: compactMetaItems([
      getReadablePath(path),
      detail.owner ? `Owner ${detail.owner}` : '',
      detail.run_count ? `${formatCompactCount(detail.run_count || 0)} 次运行` : '',
    ]),
  }
}

function createFallbackResourceMeta(path: string): PromptResourceMeta {
  const normalized = normalizeResourcePathForMeta(path)
  const resourceLike: ResourceSearchResult = {
    id: 0,
    name: resourceDisplayName(normalized),
    code: getPathTail(normalized),
    type: normalized.includes('/docs/') || normalized.endsWith('.docs') ? 'docs' : normalized.endsWith('/') ? 'package' : 'function',
    full_code_path: normalized,
    template_type: normalized.endsWith('.form') ? 'form' : normalized.endsWith('.table') ? 'table' : normalized.endsWith('.chart') ? 'chart' : '',
  }
  return mapResourceSearchResultToMeta(resourceLike)
}

function normalizeResourcePathForMeta(pathOrToken: string) {
  return resolveWorkspaceResourcePath(pathOrToken, props.fullCodePath)
}

function openInfoCardForUser(username: string, event: MouseEvent | KeyboardEvent) {
  setInfoCardAnchor(event)
  const normalized = normalizeMentionUsername(username)
  const meta = getUserMeta(normalized) || {
    username: normalized,
    label: normalized,
    description: '',
    metaItems: [`@${normalized}`],
  }
  activeInfoCard.value = {
    kind: 'user',
    title: getUserTokenLabel(normalized),
    subtitle: meta.metaItems.filter((item) => item !== `@${normalized}`).join(' / '),
    description: meta.description || '',
    raw: `@${normalized}`,
    avatar: meta.avatar,

  }
}

function openInfoCardForResource(pathOrToken: string, event: MouseEvent | KeyboardEvent) {
  setInfoCardAnchor(event)
  const path = normalizeResourcePathForMeta(pathOrToken)
  const meta = getResourceMeta(path) || createFallbackResourceMeta(path)
  activeInfoCard.value = {
    kind: 'resource',
    title: meta.label,
    subtitle: meta.typeLabel,
    description: meta.description || '正在加载用途说明…',
    raw: wrapWorkspaceResourcePath(path),
    iconSrc: meta.iconSrc,
    iconComponent: meta.iconComponent,
    iconClass: meta.iconClass,

  }
  const card = activeInfoCard.value
  void getServiceTreeDetail(path).then(detail => {
    const loaded = mapResourceDetailToMeta(detail)
    resourceMetaByPath.value = { ...resourceMetaByPath.value, [path]: loaded }
    if (activeInfoCard.value !== card) return
    Object.assign(card, {
      title: loaded.label, subtitle: loaded.typeLabel,
      description: loaded.description || '这个资源尚未填写用途说明。',
      iconSrc: loaded.iconSrc, iconComponent: loaded.iconComponent, iconClass: loaded.iconClass,
    })
  }).catch(() => {
    if (activeInfoCard.value === card && card && !meta.description) {
      card.description = '暂时无法加载用途说明，请稍后重新点击查看。'
    }
  })
}

function setInfoCardAnchor(event: MouseEvent | KeyboardEvent) {
  const target = event.target instanceof Element ? event.target : null
  infoCardAnchor.value = target?.closest<HTMLElement>('.spc-editor-token, .spc-user-chip, .spc-resource-chip')
    || (event.currentTarget instanceof HTMLElement ? event.currentTarget : rootRef.value)
  closeMentionPanel()
}

function closeInfoCard() {
  activeInfoCard.value = null
}

function handleDocumentMouseDown(event: MouseEvent) {
  const root = rootRef.value
  if (!root || !(event.target instanceof Node) || root.contains(event.target) || infoCardRef.value?.contains(event.target)) {
    return
  }
  closeInfoCard()
}

function parseStructuredPromptSegments(text: string): StructuredPromptSegment[] {
  const source = String(text || '')
  const tokens: Array<StructuredPromptResourceSegment | StructuredPromptUserSegment> = []

  parseWorkspacePromptSegments(source, props.fullCodePath).forEach((segment) => {
    if (segment.type === 'resource') {
      tokens.push(segment as StructuredPromptResourceSegment)
    }
  })

  const userMatcher = /(^|\s)(@[^\s<>]+)/g
  let userMatch: RegExpExecArray | null
  while ((userMatch = userMatcher.exec(source)) !== null) {
    const prefix = userMatch[1] || ''
    const raw = userMatch[2] || ''
    const parsedMention = parseUserMentionToken(raw)
    const start = userMatch.index + prefix.length
    const end = start + raw.length
    const nextChar = source[end] || ''
    if (!parsedMention || (nextChar && !/\s/.test(nextChar))) {
      continue
    }
    tokens.push({
      type: 'user',
      text: parsedMention.canonical,
      username: parsedMention.username,
      start,
      end,
    })
  }

  const orderedTokens = tokens
    .sort((a, b) => a.start - b.start || a.end - b.end)
    .filter((token, index, list) => {
      const previous = list[index - 1]
      return !previous || token.start >= previous.end
    })

  const segments: StructuredPromptSegment[] = []
  let cursor = 0
  orderedTokens.forEach((token) => {
    if (token.start > cursor) {
      segments.push({
        type: 'text',
        text: source.slice(cursor, token.start),
        start: cursor,
        end: token.start,
      })
    }
    segments.push(token)
    cursor = token.end
  })

  if (cursor < source.length) {
    segments.push({
      type: 'text',
      text: source.slice(cursor),
      start: cursor,
      end: source.length,
    })
  }

  if (segments.length === 0) {
    segments.push({
      type: 'text',
      text: source,
      start: 0,
      end: source.length,
    })
  }

  return segments
}

function parseUserMentionToken(raw: string): { canonical: string; username: string } | null {
  const body = String(raw || '').trim().replace(/^@/, '')
  const username = normalizeMentionUsername(body)
  if (!username) return null
  return {
    canonical: `@${username}`,
    username,
  }
}

function normalizeMentionUsername(username: string) {
  const trimmed = String(username || '').trim().replace(/^@/, '')
  const parenIndex = trimmed.indexOf('(')
  const normalized = parenIndex >= 0 ? trimmed.slice(0, parenIndex) : trimmed
  return normalized.trim()
}

function normalizeUserDisplayLabel(username: string, label: string) {
  const normalizedUsername = normalizeMentionUsername(username)
  const normalizedLabel = String(label || '').trim().replace(/^@/, '')
  if (!normalizedLabel || normalizedLabel === normalizedUsername) {
    return normalizedUsername === 'system' ? '系统' : ''
  }
  if (normalizedUsername === 'system') {
    const systemInner = extractParenLabel(normalizedLabel)
    return systemInner || (normalizedLabel.includes('system') ? '系统' : normalizedLabel)
  }
  if (normalizedLabel.startsWith(`${normalizedUsername}(`) && normalizedLabel.endsWith(')')) {
    return extractParenLabel(normalizedLabel)
  }
  return normalizedLabel
}

function extractParenLabel(value: string) {
  const start = value.indexOf('(')
  const end = value.lastIndexOf(')')
  if (start < 0 || end <= start) return ''
  return value.slice(start + 1, end).trim()
}

function getPathTail(path: string) {
  if (isWorkspaceToolResourcePath(path)) {
    return workspaceToolName(path)
  }
  const parts = path.split('/').filter(Boolean)
  return parts[parts.length - 1] || ''
}

function getReadablePath(path: string) {
  if (isWorkspaceToolResourcePath(path)) {
    return `内置工具 / ${workspaceToolName(path) || path}`
  }
  const parts = path.split('/').filter(Boolean)
  if (parts.length <= 3) return path
  return `/${parts.slice(-3).join('/')}`
}

function isHydratableResourcePath(path: string) {
  return String(path || '').startsWith('/') && !isWorkspaceToolResourcePath(path)
}

function cleanMentionText(value?: string) {
  return String(value || '')
    .replace(/<[^>]*>/g, '')
    .replace(/\s+/g, ' ')
    .trim()
}

function compactMetaItems(items: Array<string | undefined | null>) {
  return items
    .map(item => cleanMentionText(item || ''))
    .filter((item, index, list) => item && list.indexOf(item) === index)
}

function getResourceTypeLabel(resource: ResourceSearchResult) {
  if (isWorkspaceToolResourcePath(resource.full_code_path || '')) return '内置工具'
  if (resource.type === 'package') return '服务目录'
  if (resource.type === 'docs') return '文档'
  if (resource.template_type === 'table') return '数据表'
  if (resource.template_type === 'form') return '表单'
  if (resource.template_type === 'chart') return '图表'
  return '工具'
}

function getMatchSourceLabel(matchSource: string) {
  const normalized = matchSource.toLowerCase()
  if (normalized.includes('description')) return '描述'
  if (normalized.includes('tag')) return '标签'
  if (normalized.includes('code')) return '编码'
  if (normalized.includes('path')) return '路径'
  if (normalized.includes('name')) return '名称'
  return matchSource
}

function shouldShowResourceHeat(resource: ResourceSearchResult) {
  return resource.type === 'function' && !!resource.run_count
}

function formatCompactCount(count: number) {
  if (count >= 10000) return `${(count / 10000).toFixed(count >= 100000 ? 0 : 1)}w`
  if (count >= 1000) return `${(count / 1000).toFixed(count >= 10000 ? 0 : 1)}k`
  return String(count)
}

function getResourceIconMeta(resource: ResourceSearchResult): Pick<StructuredMentionOption, 'iconSrc' | 'iconComponent' | 'iconClass'> {
  if (isWorkspaceToolResourcePath(resource.full_code_path || '')) {
    return { iconClass: 'tool-icon' }
  }
  if (resource.type === 'package') {
    return { iconSrc: '/service-tree/custom-folder.svg', iconClass: 'package-icon-img' }
  }
  if (resource.type === 'docs') {
    return { iconSrc: '/文档.svg', iconClass: 'docs-icon-img' }
  }
  if (resource.template_type === 'form') {
    return { iconSrc: '/service-tree/编辑.svg', iconClass: 'form-icon-img' }
  }
  if (resource.template_type === 'table') {
    return { iconComponent: RAW_TABLE_ICON, iconClass: 'table-icon' }
  }
  if (resource.template_type === 'chart') {
    return { iconComponent: RAW_CHART_ICON, iconClass: 'chart-icon' }
  }
  return { iconClass: 'function-icon' }
}

function resourceKind(path: string) {
  if (isWorkspaceToolResourcePath(path)) return 'tool'
  if (path.endsWith('.form')) return 'form'
  if (path.endsWith('.table')) return 'table'
  if (path.endsWith('.chart')) return 'chart'
  if (path.endsWith('.docs') || path.includes('/docs/')) return 'docs'
  return 'directory'
}

function resourceDisplayName(path: string) {
  const normalized = String(path || '').replace(/[<>]/g, '')
  if (isWorkspaceToolResourcePath(normalized)) {
    return workspaceToolName(normalized) || normalized
  }
  const parts = normalized.split('/').filter(Boolean)
  return parts[parts.length - 1] || normalized || '资源'
}

function focus() {
  if (props.readonlyPreview) {
    return
  }
  mode.value = 'edit'
  void nextTick(() => {
    const editor = editorRef.value
    if (!editor) return
    editor.focus()
    const selection = window.getSelection()
    const range = selection?.rangeCount ? selection.getRangeAt(0) : null
    if (document.activeElement === editor && (!range || !isRangeInside(editor, range))) {
      restoreCaretTextOffset(editor, Math.min(lastCaretOffset, currentText.value.length))
    }
  })
}

function focusAtEnd() {
  if (props.readonlyPreview) {
    return
  }
  mode.value = 'edit'
  void nextTick(() => {
    const editor = editorRef.value
    if (!editor) return
    editor.focus()
    restoreCaretTextOffset(editor, currentText.value.length)
    lastCaretOffset = currentText.value.length
  })
}

function blur() {
  editorRef.value?.blur()
}

defineExpose({
  focus,
  focusAtEnd,
  blur,
  insertTextAtCaret,
  insertWorkspaceResources,
  getElement: () => editorRef.value,
})
</script>

<style scoped>
.structured-prompt-composer {
  --spc-bg: rgba(9, 15, 28, 0.9);
  --spc-border: rgba(125, 146, 183, 0.28);
  --spc-text: #e6f0ff;
  --spc-muted: #8da0bd;
  --spc-accent: #60e7ff;
  position: relative;
  border: 1px solid var(--spc-border);
  border-radius: 10px;
  background: linear-gradient(180deg, rgba(13, 20, 35, 0.92), rgba(8, 13, 24, 0.88));
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.05);
}

.structured-prompt-composer.is-focused {
  border-color: var(--spc-border);
  box-shadow: none;
}

.structured-prompt-composer.is-disabled {
  opacity: 0.68;
}

.structured-prompt-composer:not(.is-disabled) {
  cursor: text;
}

.structured-prompt-composer * {
  cursor: auto;
}

.spc-toolbar {
  min-height: 42px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 8px 6px 10px;
  border-bottom: 1px solid rgba(125, 146, 183, 0.16);
}

.spc-mode-tabs {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px;
  border: 1px solid rgba(125, 146, 183, 0.18);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.04);
}

.spc-mode-btn {
  height: 28px;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border: 0;
  border-radius: 6px;
  padding: 0 9px;
  background: transparent;
  color: var(--spc-muted);
  cursor: pointer;
  font-size: 12px;
  font-weight: 700;
}

.spc-mode-btn.is-active {
  background: rgba(96, 231, 255, 0.14);
  color: var(--spc-accent);
}

.spc-token-summary {
  min-width: 0;
  display: flex;
  justify-content: flex-end;
  gap: 5px;
  overflow: hidden;
}

.spc-token-pill,
.spc-token-count {
  max-width: 150px;
  display: inline-flex;
  align-items: center;
  overflow: hidden;
  border: 1px solid rgba(96, 231, 255, 0.18);
  border-radius: 7px;
  padding: 3px 7px;
  color: rgba(216, 248, 255, 0.86);
  background: rgba(96, 231, 255, 0.08);
  font-size: 11px;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.spc-editor,
.spc-preview {
  flex: 1;
  box-sizing: border-box;
  overflow-y: auto;
  padding: 12px;
  color: var(--spc-text);
  font-size: 14px;
  line-height: 24px;
  text-align: left;
}

.spc-editor {
  flex: 1;
  width: 100%;
  outline: none;
  direction: ltr;
  unicode-bidi: plaintext;
  white-space: pre-wrap;
  word-break: break-word;
}

.is-compact .spc-editor,
.is-compact .spc-preview {
  padding: 4px 0;
  font-size: 14px;
  line-height: 20px;
}

.spc-editor.is-empty::before {
  content: attr(data-placeholder);
  display: block;
  width: 100%;
  color: var(--text-placeholder, rgba(141, 160, 189, 0.64));
  -webkit-text-fill-color: var(--text-placeholder, rgba(141, 160, 189, 0.64));
  font-weight: 400;
  text-align: left;
  pointer-events: none;
}

.spc-preview {
  flex: 1;
}

.spc-preview-body {
  white-space: pre-wrap;
  word-break: break-word;
}

.spc-preview-placeholder {
  color: rgba(141, 160, 189, 0.72);
}

:deep(.spc-editor-token),
.spc-resource-chip,
.spc-user-chip {
  max-width: min(420px, 100%);
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 0 2px;
  border: 1px solid rgba(96, 231, 255, 0.22);
  border-radius: 7px;
  padding: 1px 7px;
  background: rgba(96, 231, 255, 0.1);
  color: #baf7ff;
  font-size: 12px;
  font-weight: 700;
  line-height: 20px;
  vertical-align: baseline;
  white-space: nowrap;
  cursor: pointer;
}

:deep(.spc-editor-token.is-user),
.spc-user-chip {
  border-color: rgba(124, 255, 196, 0.2);
  background: rgba(124, 255, 196, 0.08);
  color: #b9ffd9;
}

:deep(.spc-editor-token.is-table),
.spc-resource-chip:has(.spc-resource-icon-fallback.is-table) {
  border-color: rgba(85, 60, 206, 0.24);
}

:deep(.spc-editor-token.is-tool),
.spc-resource-chip:has(.spc-resource-icon-fallback.is-tool) {
  border-color: rgba(37, 99, 235, 0.24);
}

:deep(.spc-editor-token-label),
.spc-user-name,
.spc-resource-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.spc-resource-icon-img,
:deep(.spc-resource-icon-img),
.spc-resource-icon-component,
:deep(.spc-resource-icon-component) {
  width: 15px;
  height: 15px;
  flex-shrink: 0;
  object-fit: contain;
}

.spc-resource-icon-fallback,
:deep(.spc-resource-icon-fallback) {
  width: 9px;
  height: 9px;
  flex-shrink: 0;
  border-radius: 3px;
  background: #6366f1;
  box-shadow: none;
}

.spc-resource-icon-fallback.is-table,
:deep(.spc-resource-icon-fallback.is-table) {
  background: #553CCE;
  box-shadow: none;
}

.spc-resource-icon-fallback.is-chart,
:deep(.spc-resource-icon-fallback.is-chart) {
  background: #8b5cf6;
  box-shadow: none;
}

.spc-resource-icon-fallback.is-docs,
:deep(.spc-resource-icon-fallback.is-docs) {
  background: #f59e0b;
  box-shadow: none;
}

.spc-resource-icon-fallback.is-tool,
:deep(.spc-resource-icon-fallback.is-tool) {
  background: #2563eb;
  box-shadow: none;
}

.spc-invocation-list {
  display: grid;
  gap: 8px;
  margin-bottom: 10px;
}

.spc-invocation-card {
  border: 1px solid rgba(96, 231, 255, 0.18);
  border-radius: 8px;
  padding: 9px;
  background: rgba(96, 231, 255, 0.06);
}

.spc-invocation-head {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.spc-invocation-tool {
  flex-shrink: 0;
  border-radius: 6px;
  padding: 2px 6px;
  background: rgba(139, 92, 246, 0.16);
  color: #c8b6ff;
  font-size: 12px;
  font-weight: 800;
}

.spc-invocation-resource {
  min-width: 0;
  overflow: hidden;
  color: #d8f8ff;
  font-size: 13px;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.spc-param-row {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-top: 8px;
}

.spc-param-chip {
  border-radius: 6px;
  padding: 2px 6px;
  background: rgba(255, 255, 255, 0.06);
  color: rgba(230, 240, 255, 0.78);
  font-size: 11px;
}

:global(.spc-info-popover.el-popover.el-popper) {
  z-index: var(--aos-z-global-overlay, 10040) !important;
  max-width: calc(100vw - 24px);
  padding: 0;
  border: 0;
  background: transparent;
}
.spc-info-close { position: absolute; right: 8px; top: 6px; border: 0; background: transparent; color: inherit; cursor: pointer; font-size: 20px; }
.spc-info-card {
  position: relative;
  width: 100%;
  max-height: min(300px, calc(100vh - 24px));
  overflow-y: auto;
  box-sizing: border-box;
  border: 1px solid rgba(96, 231, 255, 0.22);
  border-radius: 10px;
  padding: 10px;
  background:
    linear-gradient(145deg, rgba(5, 17, 31, 0.98), rgba(9, 30, 48, 0.96)),
    rgba(4, 12, 24, 0.98);
  box-shadow: 0 18px 44px rgba(0, 0, 0, 0.34), 0 0 24px rgba(34, 211, 238, 0.12);
}

.spc-info-head {
  display: grid;
  grid-template-columns: 38px minmax(0, 1fr);
  gap: 10px;
  align-items: center;
}

.spc-info-icon {
  width: 38px;
  height: 38px;
  border: 1px solid rgba(96, 231, 255, 0.18);
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: rgba(34, 211, 238, 0.08);
  color: #22d3ee;
}

.spc-info-icon.is-user {
  overflow: hidden;
  color: #7cffc4;
  background: rgba(124, 255, 196, 0.08);
  border-color: rgba(124, 255, 196, 0.2);
}

.spc-info-avatar {
  width: 36px;
  height: 36px;
  flex-shrink: 0;
}

.spc-info-img,
.spc-info-component {
  width: 22px;
  height: 22px;
  object-fit: contain;
}

.spc-info-main {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.spc-info-main strong {
  min-width: 0;
  overflow: hidden;
  color: #d8f8ff;
  font-size: 13px;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.spc-info-main span {
  min-width: 0;
  overflow: hidden;
  color: rgba(184, 225, 235, 0.66);
  font-size: 12px;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.spc-info-desc {
  margin-top: 9px;
  color: rgba(230, 240, 255, 0.76);
  font-size: 12px;
  line-height: 1.5;
}

.spc-info-raw {
  display: block;
  margin-top: 9px;
  overflow: hidden;
  border: 1px solid rgba(96, 231, 255, 0.13);
  border-radius: 7px;
  padding: 6px 7px;
  background: rgba(255, 255, 255, 0.04);
  color: rgba(216, 248, 255, 0.8);
  font-size: 11px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.spc-mention-panel {
  width: 100%;
  max-height: min(520px, 65vh);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border-light, var(--el-border-color-light));
  border-radius: 10px;
  background: var(--el-bg-color-overlay, var(--bg-secondary));
  box-shadow: var(--el-box-shadow-light, 0 12px 32px rgba(15, 23, 42, 0.14));
}

:global(.spc-mention-popover.el-popover.el-popper) {
  padding: 0;
  border: 0;
  border-radius: 10px;
  background: transparent;
  box-shadow: none;
  min-width: min(300px, calc(100vw - 24px));
  max-width: calc(100vw - 24px);
}

.spc-resource-scopes, .spc-resource-tabs, .spc-mention-footer {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  flex-shrink: 0;
}
.spc-resource-tabs { overflow-x: auto; border-bottom: 1px solid var(--el-border-color-lighter); }
.spc-resource-scopes button, .spc-resource-tabs button, .spc-mention-footer button {
  border: 0;
  border-radius: 6px;
  padding: 6px 10px;
  background: transparent;
  color: var(--el-text-color-secondary);
  white-space: nowrap;
  cursor: pointer;
}
.spc-resource-scopes button[aria-pressed="true"], .spc-resource-tabs button[aria-selected="true"] {
  color: var(--el-color-primary);
  background: rgba(var(--color-primary-rgb), 0.14);
  font-weight: 600;
}
.spc-resource-scopes button:disabled { opacity: .4; cursor: default; }
.spc-resource-location { padding: 0 12px 6px; font-size: 11px; color: var(--el-text-color-secondary); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; flex-shrink: 0; }
.spc-mention-footer { justify-content: space-between; border-top: 1px solid var(--el-border-color-lighter); font-size: 11px; color: var(--el-text-color-secondary); }
.spc-mention-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 38px;
  padding: 8px 10px;
  border-bottom: 1px solid var(--border-light, var(--el-border-color-lighter));
  color: var(--text-secondary, var(--el-text-color-secondary));
  font-size: 12px;
  font-weight: 700;
}

.spc-mention-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border: 1px solid rgba(var(--color-primary-rgb), 0.22);
  border-radius: 5px;
  background: rgba(var(--color-primary-rgb), 0.1);
  color: var(--color-primary, var(--el-color-primary));
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.spc-mention-state {
  min-height: 72px;
  display: flex;
  align-items: center;
  padding: 16px 14px;
  color: var(--text-secondary, var(--el-text-color-secondary));
  font-size: 12px;
}

.spc-mention-list {
  min-height: 0;
  overscroll-behavior: contain;
  overflow-y: auto;
  padding: 7px;
}

.spc-mention-list::-webkit-scrollbar {
  width: 7px;
}

.spc-mention-list::-webkit-scrollbar-thumb {
  border-radius: 999px;
  background: rgba(var(--color-primary-rgb), 0.2);
}

.spc-mention-option {
  width: 100%;
  min-width: 0;
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
  min-height: 62px;
  height: auto;
  border: 1px solid transparent;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  cursor: pointer;
  padding: 8px 9px;
  text-align: left;
  transition: border-color 0.16s ease, background 0.16s ease;
}

.spc-mention-option:hover,
.spc-mention-option.is-active {
  border-color: rgba(var(--color-primary-rgb), 0.3);
  background: rgba(var(--color-primary-rgb), 0.09);
}

.spc-mention-option.is-active {
  box-shadow: inset 3px 0 0 var(--color-primary, var(--el-color-primary));
}

.spc-mention-icon {
  width: 34px;
  height: 34px;
  box-sizing: border-box;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(var(--color-primary-rgb), 0.18);
  background: rgba(var(--color-primary-rgb), 0.08);
  color: var(--color-primary, var(--el-color-primary));
}

.spc-mention-icon.is-user {
  overflow: hidden;
  color: var(--color-primary, var(--el-color-primary));
  background: rgba(var(--color-primary-rgb), 0.08);
  border-color: rgba(var(--color-primary-rgb), 0.18);
}

.spc-mention-icon.table-icon {
  color: var(--color-primary, var(--el-color-primary));
}

.spc-mention-icon.form-icon-img {
  color: #3b82f6;
}

.spc-mention-icon.chart-icon {
  color: #9377e0;
}

.spc-mention-icon.function-icon {
  color: #6366f1;
}

.spc-mention-icon.tool-icon {
  color: #2563eb;
}

.spc-mention-icon.package-icon-img,
.spc-mention-icon.docs-icon-img,
.spc-mention-icon.form-icon-img {
  background: rgba(255, 255, 255, 0.04);
}

.spc-mention-avatar {
  width: 30px;
  height: 30px;
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 700;
}

.spc-mention-resource-img {
  width: 20px;
  height: 20px;
  object-fit: contain;
  flex-shrink: 0;
  opacity: 0.94;
}

.spc-mention-resource-component {
  width: 20px;
  height: 20px;
  flex-shrink: 0;
}

.spc-mention-main,
.spc-mention-title-row,
.spc-mention-title,
.spc-mention-desc {
  min-width: 0;
}

.spc-mention-main {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 3px;
  overflow: hidden;
}

.spc-mention-title-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 20px;
}

.spc-mention-title {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-primary, var(--el-text-color-primary));
  font-size: 13px;
  font-weight: 700;
}

.spc-mention-type {
  flex-shrink: 0;
  border: 1px solid var(--border-light, var(--el-border-color-light));
  border-radius: 6px;
  padding: 2px 5px;
  color: var(--text-secondary, var(--el-text-color-secondary));
  font-size: 11px;
  line-height: 1.2;
  background: var(--el-fill-color-light);
}

.spc-mention-desc {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-secondary, var(--el-text-color-secondary));
  font-size: 12px;
}

</style>
