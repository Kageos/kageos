<template>
  <div
    v-if="attachedFiles.length > 0"
    :class="['mini-ws-files', { 'mini-ws-files--compact': compactWindow }]"
    data-testid="mini-workstation-attached-files"
  >
    <div class="mini-ws-files__summary">
      <el-icon :size="14"><Paperclip /></el-icon>
      <span>{{ t('miniWorkstation.attachedFiles', { count: attachedFiles.length }) }}</span>
    </div>
    <div class="mini-ws-files__list">
      <span
        v-for="(file, idx) in attachedFiles"
        :key="`${file.ref || file.name}-${idx}`"
        class="mini-ws-file-chip"
        :title="file.source_name || file.name"
      >
        <el-icon :size="14"><Document /></el-icon>
        <span class="mini-ws-file-chip__name">{{ file.source_name || file.name }}</span>
        <button
          type="button"
          class="mini-ws-file-chip__remove"
          :title="t('miniWorkstation.removeAttachedFile', { name: file.source_name || file.name })"
          :aria-label="t('miniWorkstation.removeAttachedFile', { name: file.source_name || file.name })"
          @click.stop="removeFile(idx)"
        >
          <el-icon :size="12"><Close /></el-icon>
        </button>
      </span>
    </div>
  </div>

  <div
    class="mini-ws-input"
    :class="{ 'mini-ws-input--schedule': variant === 'schedule', 'mini-ws-input--compact-window': compactWindow }"
    data-testid="mini-workstation-composer"
    @click="handleContainerClick"
  >
    <div class="mini-composer-left-actions">
      <el-upload
        :auto-upload="false"
        :show-file-list="false"
        :on-change="onFileChange"
        :disabled="uploading || blocked"
        class="mini-upload-btn"
      >
        <el-button :icon="Paperclip" link :loading="uploading" size="small" :title="t('miniWorkstation.uploadFile')" />
      </el-upload>
      <slot name="left-actions" />
    </div>

    <div class="mini-input-wrap" :class="{ 'is-blocked': blocked }">
      <MiniWorkstationResourceIdentity
        v-if="variant !== 'schedule'"
        class="mini-path-pill"
        :name="displayPath"
        :full-code-path="fullCodePath"
        :resource-type="resourceType"
        :resource-template-type="resourceTemplateType"
      />
      <span v-if="blocked" class="mini-blocked-pill" :title="blockedLabel || t('miniWorkstation.blockingGeneric')">
        {{ blockedLabel || t('miniWorkstation.blockingGeneric') }}
      </span>
      <StructuredPromptComposer
        ref="structuredInputRef"
        class="mini-structured-input"
        :model-value="inputText"
        :placeholder="composerPlaceholder"
        :disabled="blocked"
        :submit-on-enter="composerSubmitOnEnter"
        :show-toolbar="variant === 'schedule'"
        :compact="variant !== 'schedule'"
        :min-rows="variant === 'schedule' ? 6 : 4"
        :max-rows="variant === 'schedule' ? 14 : 4"
        :full-code-path="fullCodePath"
        :mention-panel-placement="mentionPanelPlacement"
        editor-test-id="mini-workstation-input"
        @update:model-value="emitInput"
        @enter="handleComposerEnter"
      />
    </div>

    <div v-if="variant !== 'schedule'" class="mini-action-stack">
      <el-select
        :model-value="selectedLLMConfigId"
        :placeholder="t('miniWorkstation.defaultModel')"
        filterable
        :loading="llmLoading"
        teleported
        fit-input-width
        :persistent="false"
        placement="top-start"
        :popper-options="modelSelectPopperOptions"
        popper-class="mini-ws-model-select-popper"
        class="mini-ws-model-select"
        @update:model-value="emit('update:selectedLLMConfigId', Number($event))"
        @visible-change="onLLMSelectVisibleChange"
      >
        <el-option :label="t('miniWorkstation.defaultModel')" :value="0" />
        <el-option
          v-for="llm in llmList"
          :key="llm.id"
          :label="llmOptionLabel(llm)"
          :value="llm.id"
        />
      </el-select>
      <div class="mini-action-row">
        <el-button
          v-if="sending || sessionRunning"
          type="danger"
          size="small"
          :loading="stopping"
          data-testid="mini-workstation-stop"
          class="mini-stop-btn"
          @click="$emit('stop')"
        >
          <el-icon><VideoPause /></el-icon>
          {{ t('miniWorkstation.stop') }}
        </el-button>
        <el-button
          v-else
          type="primary"
          size="small"
          :disabled="blocked || !fullCodePath || (!inputText.trim() && attachedFiles.length === 0)"
          data-testid="mini-workstation-send"
          class="mini-send-btn"
          @click="$emit('send')"
        >
          {{ t('miniWorkstation.send') }}
        </el-button>
        <el-tooltip
          :content="miniHideShortcutHint"
          placement="top"
          effect="light"
          popper-class="mini-workstation-tooltip-popper"
        >
          <button
            type="button"
            class="mini-hide-btn"
            :title="miniHideShortcutHint"
            data-testid="mini-workstation-collapse"
            @click="$emit('collapse')"
          >
            <el-icon><Close /></el-icon>
          </button>
        </el-tooltip>
      </div>
    </div>
  </div>

</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Close, Document, Paperclip, VideoPause } from '@element-plus/icons-vue'
import type { LLMInfo } from '@/architecture/presentation/context/api/agent'
import type { WorkspaceChatMessageFile } from '@/architecture/presentation/context/api/workspace'
import StructuredPromptComposer from './StructuredPromptComposer.vue'
import MiniWorkstationResourceIdentity from './MiniWorkstationResourceIdentity.vue'

interface FocusableInput {
  focus: () => void
  focusAtEnd?: () => void
  insertWorkspaceResources?: (paths: string[]) => void
}

const props = withDefaults(defineProps<{
  fullCodePath: string
  dirName?: string
  resourceType?: string
  resourceTemplateType?: string
  attachedFiles: WorkspaceChatMessageFile[]
  uploading: boolean
  inputText: string
  sending: boolean
  sessionRunning?: boolean
  stopping: boolean
  selectedLLMConfigId: number
  llmList: LLMInfo[]
  llmLoading: boolean
  queuedCount: number
  blocked?: boolean
  blockedLabel?: string
  blockedPlaceholder?: string
  registerInputRef: (el: FocusableInput | null) => void
  onLLMSelectVisibleChange: (visible: boolean) => void
  onFileChange: (uploadFileObj: { raw?: File }) => void | Promise<void>
  removeFile: (index: number) => void
  onInputEnter: (event: KeyboardEvent) => void
  toggleShortcutLabel?: string
  variant?: 'chat' | 'schedule'
  placeholder?: string
  mentionPanelPlacement?: 'above' | 'below'
  compactWindow?: boolean
}>(), {
  variant: 'chat',
  resourceType: '',
  resourceTemplateType: '',
  placeholder: '',
  mentionPanelPlacement: 'above',
  compactWindow: false,
})

const emit = defineEmits<{
  (e: 'update:inputText', value: string): void
  (e: 'update:selectedLLMConfigId', value: number): void
  (e: 'send'): void
  (e: 'stop'): void
  (e: 'collapse'): void
}>()

const { t } = useI18n()
const structuredInputRef = ref<InstanceType<typeof StructuredPromptComposer> | null>(null)

const modelSelectPopperOptions = {
  strategy: 'fixed' as const,
}

function llmProtocolLabel(protocol: string) {
  switch ((protocol || '').trim()) {
    case 'openai_responses':
      return 'Responses'
    case 'anthropic_messages':
      return 'Messages'
    default:
      return 'Chat'
  }
}

function llmEndpointLabel(llm: LLMInfo) {
  const endpoint = (llm.endpoint_path || '').trim()
  const base = (llm.api_base || '').trim().replace(/\/+$/, '')
  if (!base) {
    return endpoint
  }
  try {
    const url = new URL(base)
    const basePath = url.pathname.replace(/\/+$/, '')
    const endpointPath = endpoint ? (endpoint.startsWith('/') ? endpoint : `/${endpoint}`) : ''
    return `${url.host}${basePath}${endpointPath}`
  } catch {
    if (endpoint) {
      return `${base}${endpoint.startsWith('/') ? endpoint : `/${endpoint}`}`.replace(/^https?:\/\//, '')
    }
    return base.replace(/^https?:\/\//, '')
  }
}

function llmOptionLabel(llm: LLMInfo) {
  const endpoint = llmEndpointLabel(llm)
  const suffix = endpoint ? ` · ${endpoint}` : ''
  return `#${llm.id} ${llm.name} (${llm.model} · ${llmProtocolLabel(llm.protocol)}${suffix})`
}

const miniHideShortcutHint = computed(() => {
  const toggleShortcut = (props.toggleShortcutLabel || '').trim()
  return toggleShortcut
    ? t('miniWorkstation.closeBackgroundHintWithShortcut', { shortcut: toggleShortcut })
    : t('miniWorkstation.closeBackgroundHint')
})

const composerPlaceholder = computed(() => {
  if (props.blocked) {
    return props.blockedPlaceholder || t('miniWorkstation.blockedPlaceholder')
  }
  const placeholder = props.placeholder.trim()
  if (placeholder) {
    return placeholder
  }
  if (props.variant === 'schedule') {
    return t('miniWorkstation.schedulePlaceholder')
  }
  return t('miniWorkstation.chatPlaceholder')
})

const composerSubmitOnEnter = computed(() => props.variant !== 'schedule')

const displayPath = computed(() => {
  const label = (props.dirName || '').trim()
  if (label) return label
  if (!props.fullCodePath) return t('miniWorkstation.noDirectorySelected')
  const parts = props.fullCodePath.split('/').filter(Boolean)
  return parts[parts.length - 1] || props.fullCodePath
})

watch(structuredInputRef, () => {
  registerStructuredInputRef()
}, { flush: 'post' })

onMounted(() => {
  void nextTick(registerStructuredInputRef)
})

onBeforeUnmount(() => {
  props.registerInputRef(null)
})

function registerStructuredInputRef() {
  props.registerInputRef(structuredInputRef.value)
}

function emitInput(value: string) {
  if (props.blocked) return
  emit('update:inputText', value)
}

function handleComposerEnter(event: KeyboardEvent) {
  if (props.blocked) return
  props.onInputEnter(event)
}

function handleContainerClick(event: MouseEvent) {
  if (props.blocked) return
  const target = event.target as HTMLElement
  if (target.closest('button') || target.closest('a') || target.closest('.mini-path-pill') || target.closest('.el-select') || target.closest('.el-upload')) {
    return
  }
  if (target.closest('.spc-editor')) {
    return
  }
  structuredInputRef.value?.focus()
}

</script>

<style scoped>
.mini-ws-files {
  min-width: 0;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border-top: 1px solid var(--border-light);
  border-bottom: 1px solid rgba(var(--color-primary-rgb), 0.1);
  background: rgba(var(--color-primary-rgb), 0.035);
}

.mini-ws-files__summary {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--color-primary);
  font-size: 11px;
  font-weight: 700;
  white-space: nowrap;
}

.mini-ws-files__list {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  overflow-x: auto;
  scrollbar-width: thin;
  scrollbar-color: rgba(var(--color-primary-rgb), 0.2) transparent;
}

.mini-ws-file-chip {
  flex: 0 1 auto;
  min-width: 0;
  max-width: 240px;
  height: 30px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 5px 0 8px;
  border: 1px solid rgba(var(--color-primary-rgb), 0.2);
  border-radius: 8px;
  background: rgba(var(--color-primary-rgb), 0.08);
  color: var(--text-primary);
  font-size: 12px;
}

.mini-ws-file-chip > .el-icon {
  flex: 0 0 auto;
  color: var(--color-primary);
}

.mini-ws-file-chip__name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mini-ws-file-chip__remove {
  flex: 0 0 auto;
  width: 22px;
  height: 22px;
  display: inline-grid;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
}

.mini-ws-file-chip__remove:hover {
  background: rgba(var(--color-danger-rgb), 0.1);
  color: var(--color-danger);
}

.mini-ws-files--compact {
  grid-template-columns: auto minmax(0, 1fr);
  gap: 8px;
  padding: 6px 8px;
}

.mini-ws-files--compact .mini-ws-file-chip {
  max-width: 180px;
  height: 28px;
}

.mini-ws-input {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: start;
  gap: 12px;
  box-sizing: border-box;
  min-height: 50px;
  padding: 4px 10px;
  position: relative;
  border: 1px solid transparent;
  border-radius: 14px;
  background: var(--el-fill-color-light);
  box-shadow: none;
  backdrop-filter: none;
  transition: background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}

.mini-ws-input:hover {
  border-color: var(--border-light);
  background: var(--el-fill-color);
}

.mini-ws-input:focus-within {
  border-color: transparent;
  background: var(--el-fill-color-light);
  box-shadow: none;
}

.mini-ws-input--schedule {
  grid-template-columns: auto minmax(0, 1fr);
  min-height: 180px;
  align-items: stretch;
  background: var(--el-fill-color-light);
  border: 1px solid var(--border-light);
  box-shadow: none;
  backdrop-filter: none;
  border-radius: var(--border-radius-lg);
  transition: all 0.25s cubic-bezier(0.25, 0.8, 0.25, 1);

  &:hover {
    border-color: var(--border-base);
    background: var(--el-fill-color-light);
  }
  
  &:focus-within {
    background: var(--el-fill-color-light);
    border-color: var(--color-primary);
    box-shadow: 0 0 0 3px rgba(var(--color-primary-rgb), 0.12);
  }
}

html.dark .mini-ws-input--schedule {
  background: var(--el-fill-color-light);
  border-color: var(--border-light);
}

.mini-ws-input--schedule .mini-structured-input :deep(.spc-editor),
.mini-ws-input--schedule .mini-structured-input :deep(.spc-preview) {
  color: var(--text-primary);
  -webkit-text-fill-color: var(--text-primary);
}

.mini-ws-input--schedule .mini-upload-btn :deep(.el-button) {
  background: transparent;
  border-color: transparent;
  color: var(--text-secondary);
}

.mini-ws-input--schedule .mini-upload-btn :deep(.el-button):hover {
  background: rgba(var(--color-primary-rgb), 0.08);
  color: var(--color-primary);
  border-color: transparent;
  box-shadow: none;
}

.mini-composer-left-actions {
  flex-shrink: 0;
  min-width: 142px;
  display: flex;
  flex-direction: row;
  align-items: center;
  align-self: start;
  padding-top: 1px;
  gap: 8px;
}

.mini-ws-input--schedule .mini-composer-left-actions {
  min-width: auto;
}

.mini-upload-btn {
  flex-shrink: 0;
}

.mini-expand-editor-btn.el-button {
  width: 40px;
  height: 40px;
  border: 1px solid transparent;
  border-radius: 8px;
  color: var(--color-primary);
  background: var(--el-fill-color-light);
  box-shadow: none;
  transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease, box-shadow 0.2s ease;
}

.mini-expand-editor-btn.el-button:hover {
  border-color: var(--border-light);
  color: var(--color-primary-light-1);
  background: var(--el-fill-color);
}

.mini-upload-btn :deep(.el-button) {
  width: 40px;
  height: 40px;
  border: 1px solid transparent;
  border-radius: 8px;
  color: var(--color-primary);
  background: var(--el-fill-color-light);
  box-shadow: none;
  transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease, box-shadow 0.2s ease;
}

.mini-upload-btn :deep(.el-button:hover) {
  border-color: var(--border-light);
  color: var(--color-primary-light-1);
  background: var(--el-fill-color);
}

.mini-input-wrap {
  position: relative;
  min-width: 0;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  justify-items: stretch;
  align-items: start;
  gap: 10px;
  box-sizing: border-box;
  min-height: 36px;
  padding: 0 10px;
  border: 1px solid transparent;
  border-radius: 10px;
  background: var(--el-fill-color-light);
  text-align: left;
  cursor: text;
  transition: background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}

.mini-input-wrap:hover {
  border-color: var(--border-light);
  background: var(--el-fill-color);
}

.mini-input-wrap:focus-within {
  border-color: transparent;
  background: var(--el-fill-color-light);
  box-shadow: none;
}

.mini-input-wrap.is-blocked {
  cursor: default;
}

.mini-ws-input--schedule .mini-input-wrap {
  min-height: 160px;
  grid-template-columns: minmax(0, 1fr);
  align-items: stretch;
  background: var(--bg-primary);
}

.mini-path-pill {
  max-width: 148px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  padding: 0 10px;
  overflow: hidden;
  border: 1px solid rgba(var(--color-primary-rgb), 0.18);
  border-radius: 8px;
  background: rgba(var(--color-primary-rgb), 0.08);
  color: var(--color-primary);
  font-size: 12px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: default;
  margin-top: 4px;
}

.mini-blocked-pill {
  position: absolute;
  right: 10px;
  top: 5px;
  z-index: 2;
  max-width: min(180px, calc(100% - 116px));
  height: 24px;
  display: inline-flex;
  align-items: center;
  padding: 0 8px;
  overflow: hidden;
  border: 1px solid var(--border-light);
  border-radius: 8px;
  background: var(--bg-tertiary);
  color: #f6c76b;
  font-size: 12px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: default;
}

.mini-structured-input {
  min-width: 0;
  width: 100%;
  justify-self: stretch;
  align-self: start;
  border: 0;
  border-radius: 0;
  background: transparent;
  text-align: left;
  box-shadow: none;
}

.mini-structured-input.is-focused {
  border-color: transparent;
  box-shadow: none;
}

.mini-structured-input :deep(.spc-editor),
.mini-structured-input :deep(.spc-preview) {
  width: 100%;
  color: var(--text-primary);
  text-align: left;
}

.mini-input-wrap.is-blocked .mini-structured-input :deep(.spc-editor) {
  padding-right: min(190px, 42%);
  cursor: not-allowed;
  color: var(--text-placeholder);
  -webkit-text-fill-color: var(--text-placeholder);
}

.mini-action-stack {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 1px;
}

.mini-ws-model-select {
  width: 112px;
  min-width: 0;
}

.mini-ws-model-select :deep(.el-select__wrapper) {
  min-height: 40px;
  border: 1px solid transparent;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  box-shadow: none;
  transition: background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}

.mini-ws-model-select :deep(.el-select__wrapper:hover) {
  border-color: var(--border-light);
  background: var(--el-fill-color);
}

.mini-ws-model-select :deep(.el-select__wrapper.is-focused) {
  border-color: var(--color-primary);
  background: var(--el-bg-color);
  box-shadow: none;
}

.mini-ws-model-select :deep(.el-select__placeholder),
.mini-ws-model-select :deep(.el-select__selected-item) {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-primary);
}

.mini-action-row {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.mini-action-row :deep(.el-button + .el-button) {
  margin-left: 0;
}

.mini-send-btn,
.mini-stop-btn {
  flex-shrink: 0;
  min-width: 108px;
  min-height: 40px;
  border-radius: 8px;
  font-weight: 700;
  letter-spacing: 0;
  box-shadow: none;
}

.mini-send-btn.el-button--primary {
  border: 0;
  background: var(--color-primary);
  color: #ffffff;
}

.mini-hide-btn {
  width: 40px;
  height: 40px;
  min-width: 40px;
  display: grid;
  place-items: center;
  border: 1px solid transparent;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  color: var(--color-primary);
  transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease, box-shadow 0.2s ease;
}

.mini-hide-btn:hover {
  border-color: var(--border-light);
  background: var(--el-fill-color);
  color: var(--color-primary-light-1);
}

.mini-ws-input--compact-window {
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
  min-height: 48px;
  padding: 5px 8px;
}

.mini-ws-input--compact-window .mini-composer-left-actions,
.mini-ws-input--compact-window .mini-ws-model-select,
.mini-ws-input--compact-window .mini-hide-btn {
  display: none;
}

.mini-ws-input--compact-window .mini-input-wrap {
  grid-template-columns: auto minmax(0, 1fr);
  min-height: 36px;
  padding: 0 8px;
}

.mini-ws-input--compact-window .mini-path-pill {
  max-width: 132px;
}

.mini-ws-input--compact-window .mini-action-stack {
  display: flex;
  min-width: 0;
  padding: 0;
}

.mini-ws-input--compact-window .mini-send-btn,
.mini-ws-input--compact-window .mini-stop-btn {
  min-width: 72px;
  min-height: 36px;
}

:deep(.mini-ws--maximized) .mini-ws-input {
  padding: 4px 10px;
}

:deep(.mini-ws--maximized) .mini-ws-files {
  padding: 6px 24px;
}

@media (max-width: 720px) {
  .mini-ws-input {
    grid-template-columns: minmax(0, 1fr);
    align-items: stretch;
  }

  .mini-composer-left-actions,
  .mini-action-stack {
    min-width: 0;
  }

  .mini-action-stack {
    justify-content: space-between;
  }

}
</style>

<style lang="scss">
.mini-ws-model-select-popper.el-select__popper {
  max-width: min(360px, calc(100vw - 24px));
  overflow: hidden;
  border: 1px solid var(--border-light);
  background: var(--bg-secondary);
  box-shadow: 0 18px 46px rgba(0, 0, 0, 0.1);
}

.mini-ws-model-select-popper.el-select__popper:not([data-popper-placement]) {
  visibility: hidden;
}

.mini-ws-model-select-popper.el-select__popper .el-select-dropdown {
  width: 100% !important;
  min-width: 0 !important;
  max-width: 100%;
  background: transparent;
}

.mini-ws-model-select-popper.el-select__popper .el-scrollbar,
.mini-ws-model-select-popper.el-select__popper .el-select-dropdown__wrap,
.mini-ws-model-select-popper.el-select__popper .el-select-dropdown__list {
  max-width: 100%;
}

.mini-ws-model-select-popper.el-select__popper .el-select-dropdown__item {
  max-width: 100%;
  padding-right: 28px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-secondary);
}

.mini-ws-model-select-popper.el-select__popper .el-select-dropdown__item span {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mini-ws-model-select-popper.el-select__popper .el-select-dropdown__item.is-hovering,
.mini-ws-model-select-popper.el-select__popper .el-select-dropdown__item:hover {
  background: rgba(var(--color-primary-rgb), 0.08);
  color: var(--text-primary);
}

.mini-ws-model-select-popper.el-select__popper .el-select-dropdown__item.is-selected {
  color: var(--color-primary);
  background: rgba(var(--color-primary-rgb), 0.12);
}

.mini-ws-model-select-popper.el-select__popper .el-popper__arrow::before {
  background: var(--bg-tertiary);
}
</style>
