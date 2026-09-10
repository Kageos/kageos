<template>
  <template v-if="displayMessages.length > 0">
    <div
      v-for="(msg, i) in displayMessages"
      :key="msg.display_key"
      :class="['mini-msg', msg.role, { 'mini-msg--maximized': maximized, 'mini-msg--activity-group': msg.display_kind === 'activity_group' }]"
    >
      <div v-if="msg.display_kind === 'activity_group'" class="mini-activity-group">
        <details class="mini-activity-details">
          <summary class="mini-activity-summary">
            <span class="mini-activity-main">
              <el-icon v-if="isActivityGroupRunning(msg)" class="is-loading" :size="12">
                <Loading />
              </el-icon>
              <el-icon v-else-if="getActivityGroupErrorCount(msg) > 0" :size="12" color="#f56c6c">
                <CircleClose />
              </el-icon>
              <el-icon v-else :size="12" color="#67c23a">
                <CircleCheck />
              </el-icon>
              <span>执行过程 {{ getActivityGroupProcessLabel(msg) }}</span>
            </span>
            <span class="mini-activity-tools">{{ getActivityGroupToolSummary(msg) }}</span>
            <span v-if="getActivityGroupThinkingPreview(msg)" class="mini-activity-thinking-preview">
              思考：{{ getActivityGroupThinkingPreview(msg) }}
            </span>
            <span :class="['mini-activity-status', getActivityGroupStatusClass(msg)]">
              {{ getActivityGroupStatusLabel(msg) }}
            </span>
          </summary>
          <div class="mini-activity-detail">
            <details
              v-for="entry in msg.activity_entries"
              :key="`activity-step-${entry.index}`"
              class="mini-activity-step"
            >
              <summary class="mini-activity-step-head">
                <span>{{ getActivityEntryRoundLabel(entry) }}</span>
                <span v-if="entry.message.created_at">{{ formatMessageTime(entry.message.created_at) }}</span>
                <span>{{ getActivityEntryToolSummary(entry.message) }}</span>
                <span :class="['mini-activity-step-status', getActivityMessageStatusClass(entry.message)]">
                  {{ getActivityMessageStatusLabel(entry.message) }}
                </span>
              </summary>
              <div class="mini-activity-step-body">
                <ModelContextPlanCard
                  v-if="entry.message.model_context_plan"
                  :plan="entry.message.model_context_plan"
                  :plans="entry.message.model_context_plans"
                  class="mini-msg-model-context mini-activity-model-context"
                />
                <details
                  v-for="(thinking, thinkingIndex) in getThinkingBlocksFromMessage(entry.message)"
                  :key="`activity-thinking-${entry.index}-${thinkingIndex}`"
                  class="mini-thinking-block"
                >
                  <summary class="mini-thinking-head">
                    <span>{{ t('miniWorkstation.thinkingTrace') }}</span>
                  </summary>
                  <div class="mini-thinking-body">{{ thinking.text }}</div>
                </details>
                <MessageToolCalls
                  v-for="(calls, callsIndex) in getToolCallBlocksFromMessage(entry.message)"
                  :key="`activity-tools-${entry.index}-${callsIndex}`"
                  :tool-calls="calls"
                  :file-groups="getFileGroupsFromCalls(calls)"
                  :confirm-disabled="sending"
                  @confirm-prd="emit('confirm-prd', $event)"
                />
              </div>
            </details>
          </div>
        </details>
      </div>
      <div v-else-if="msg.role === 'user'" class="mini-msg-user">
        <div class="mini-msg-user-header">
          <UserDisplay
            :username="msg.user || currentUsername || null"
            mode="simple"
            size="small"
            class="mini-msg-user-display"
          />
          <span class="mini-msg-time">{{ msg.created_at ? formatMessageTime(msg.created_at) : '—' }}</span>
        </div>
        <div
          class="mini-msg-user-body"
          :class="{ 'mini-msg-user-body--with-files': Boolean(msg.files?.length) }"
        >
          <OutputFilesDisplay
            v-if="msg.files?.length"
            :file-groups="[{ label: '', files: msg.files }]"
            :section-title="t('miniWorkstation.uploadedFiles')"
            :archive-download="false"
            deletable
            class="mini-msg-files"
          />
          <div
            v-if="msg.content"
            class="mini-content-block mini-md-content"
            v-html="renderWorkbenchMarkdown(msg.content)"
            @mouseover="showResourcePreviewFromEvent"
            @focusin="showResourcePreviewFromEvent"
            @focusout="scheduleCloseResourcePreview"
            @mouseleave="scheduleCloseResourcePreview"
            @copy="onWorkspaceMarkdownCopy"
          ></div>
        </div>
      </div>
      <template v-else>
        <div class="mini-msg-assistant-header">
          <MiniWorkstationResourceIdentity
            variant="message"
            :name="counterpartName"
            :full-code-path="fullCodePath"
            :resource-type="resourceType"
            :resource-template-type="resourceTemplateType"
          />
          <span v-if="getAssistantModelLabel(msg)" class="mini-msg-model">{{ getAssistantModelLabel(msg) }}</span>
          <span
            v-if="getAssistantCacheLabel(msg)"
            class="mini-msg-cache"
            :title="getAssistantCacheTitle(msg)"
          >
            {{ getAssistantCacheLabel(msg) }}
          </span>
          <span class="mini-msg-time">{{ msg.created_at ? formatMessageTime(msg.created_at) : '—' }}</span>
          <span
            v-if="getAssistantDurationLabel(msg, getDisplayMessageIndex(msg, i))"
            :class="['mini-msg-output-duration', { 'mini-msg-output-duration--running': isAssistantTimerRunning(msg, getDisplayMessageIndex(msg, i)) }]"
          >
            <span v-if="isAssistantTimerRunning(msg, getDisplayMessageIndex(msg, i))" class="mini-msg-output-duration-dot"></span>
            {{ t('miniWorkstation.outputDuration', { duration: getAssistantDurationLabel(msg, getDisplayMessageIndex(msg, i)) }) }}
          </span>
        </div>
        <div
          v-if="getActivityEntriesForMessage(msg).length"
          class="mini-activity-group mini-activity-group--attached"
        >
          <details class="mini-activity-details">
            <summary class="mini-activity-summary">
              <span class="mini-activity-main">
                <el-icon v-if="isActivityGroupRunning(msg)" class="is-loading" :size="12">
                  <Loading />
                </el-icon>
                <el-icon v-else-if="getActivityGroupErrorCount(msg) > 0" :size="12" color="#f56c6c">
                  <CircleClose />
                </el-icon>
                <el-icon v-else :size="12" color="#67c23a">
                  <CircleCheck />
                </el-icon>
                <span>执行过程 {{ getActivityGroupProcessLabel(msg) }}</span>
              </span>
              <span class="mini-activity-tools">{{ getActivityGroupToolSummary(msg) }}</span>
              <span v-if="getActivityGroupThinkingPreview(msg)" class="mini-activity-thinking-preview">
                思考：{{ getActivityGroupThinkingPreview(msg) }}
              </span>
              <span :class="['mini-activity-status', getActivityGroupStatusClass(msg)]">
                {{ getActivityGroupStatusLabel(msg) }}
              </span>
            </summary>
            <div class="mini-activity-detail">
              <details
                v-for="entry in getActivityEntriesForMessage(msg)"
                :key="`attached-activity-step-${entry.index}`"
                class="mini-activity-step"
              >
                <summary class="mini-activity-step-head">
                  <span>{{ getActivityEntryRoundLabel(entry) }}</span>
                  <span v-if="entry.message.created_at">{{ formatMessageTime(entry.message.created_at) }}</span>
                  <span>{{ getActivityEntryToolSummary(entry.message) }}</span>
                  <span :class="['mini-activity-step-status', getActivityMessageStatusClass(entry.message)]">
                    {{ getActivityMessageStatusLabel(entry.message) }}
                  </span>
                </summary>
                <div class="mini-activity-step-body">
                  <ModelContextPlanCard
                    v-if="entry.message.model_context_plan"
                    :plan="entry.message.model_context_plan"
                    :plans="entry.message.model_context_plans"
                    class="mini-msg-model-context mini-activity-model-context"
                  />
                  <details
                    v-for="(thinking, thinkingIndex) in getThinkingBlocksFromMessage(entry.message)"
                    :key="`attached-activity-thinking-${entry.index}-${thinkingIndex}`"
                    class="mini-thinking-block"
                  >
                    <summary class="mini-thinking-head">
                      <span>{{ t('miniWorkstation.thinkingTrace') }}</span>
                    </summary>
                    <div class="mini-thinking-body">{{ thinking.text }}</div>
                  </details>
                  <MessageToolCalls
                    v-for="(calls, callsIndex) in getToolCallBlocksFromMessage(entry.message)"
                    :key="`attached-activity-tools-${entry.index}-${callsIndex}`"
                    :tool-calls="calls"
                    :file-groups="getFileGroupsFromCalls(calls)"
                    :confirm-disabled="sending"
                    @confirm-prd="emit('confirm-prd', $event)"
                  />
                </div>
              </details>
            </div>
          </details>
        </div>
        <ModelContextPlanCard
          v-if="shouldRenderAssistantTechnicalInline(msg) && msg.model_context_plan"
          :plan="msg.model_context_plan"
          :plans="msg.model_context_plans"
          class="mini-msg-model-context"
        />
        <div v-if="msg.blocks?.length" class="mini-msg-assistant">
          <template v-for="(block, bi) in getRenderBlocksForMessage(msg)" :key="bi">
            <div
              v-if="block.type === 'content'"
              class="mini-content-block mini-md-content"
              v-html="renderContentBlock(block.text, getDisplayMessageIndex(msg, i), bi, getRenderBlocksForMessage(msg).length)"
              @mouseover="showResourcePreviewFromEvent"
              @focusin="showResourcePreviewFromEvent"
              @focusout="scheduleCloseResourcePreview"
              @mouseleave="scheduleCloseResourcePreview"
              @copy="onWorkspaceMarkdownCopy"
            ></div>
            <details
              v-else-if="block.type === 'thinking'"
              class="mini-thinking-block"
              :open="isStreamingThinkingBlock(getDisplayMessageIndex(msg, i), bi, getRenderBlocksForMessage(msg).length)"
            >
              <summary class="mini-thinking-head">
                <el-icon v-if="isStreamingThinkingBlock(getDisplayMessageIndex(msg, i), bi, getRenderBlocksForMessage(msg).length)" class="is-loading" :size="12">
                  <Loading />
                </el-icon>
                <span>{{ t('miniWorkstation.thinkingTrace') }}</span>
              </summary>
              <div class="mini-thinking-body">{{ block.text }}</div>
            </details>
            <template v-else-if="block.type === 'tool_calls'">
              <template v-if="maximized">
                <MessageToolCalls
                  :tool-calls="block.calls"
                  :file-groups="getFileGroupsFromCalls(block.calls)"
                  :confirm-disabled="sending"
                  @confirm-prd="emit('confirm-prd', $event)"
                />
              </template>
              <template v-else>
                <div v-if="getVisibleToolCallsFromCalls(block.calls).length" class="mini-tools-block">
                  <div
                    v-for="(tc, ti) in getVisibleToolCallsFromCalls(block.calls)"
                    :key="`${tc.name}-${ti}`"
                    class="mini-tool-tag"
                  >
                    <el-icon v-if="tc.status === 'streaming' || tc.status === 'running'" class="is-loading" :size="12">
                      <Loading />
                    </el-icon>
                    <el-icon v-else-if="tc.status === 'ok'" :size="12" color="#67c23a">
                      <CircleCheck />
                    </el-icon>
                    <el-icon v-else-if="tc.status === 'error'" :size="12" color="#f56c6c">
                      <CircleClose />
                    </el-icon>
                    <span>{{ getToolDisplayName(tc) }}</span>
                  </div>
                </div>
                <PrdPreview
                  v-for="(tc, pi) in getPrdCallsFromCalls(block.calls)"
                  :key="`prd-${tc.name}-${pi}`"
                  :data="tc.result_data"
                  :confirm-disabled="sending"
                  show-confirmation
                  @confirm="emit('confirm-prd', $event)"
                  class="mini-msg-prd-preview"
                />
                <OutputFilesDisplay
                  v-if="getFileGroupsFromCalls(block.calls).length"
                  :file-groups="getFileGroupsFromCalls(block.calls)"
                  deletable
                  class="mini-msg-files"
                />
                <OutputDisplayFields
                  v-if="getDisplayFieldsFromCalls(block.calls).length"
                  :fields="getDisplayFieldsFromCalls(block.calls)"
                  class="mini-msg-display-fields"
                />
              </template>
            </template>
          </template>
        </div>
        <template v-else>
          <div
            v-if="msg.content"
            class="mini-msg-assistant mini-content-block mini-md-content"
            v-html="renderWorkbenchMarkdown(msg.content)"
            @mouseover="showResourcePreviewFromEvent"
            @focusin="showResourcePreviewFromEvent"
            @focusout="scheduleCloseResourcePreview"
            @mouseleave="scheduleCloseResourcePreview"
            @copy="onWorkspaceMarkdownCopy"
          ></div>
          <template v-if="maximized && msg.tool_calls?.length">
            <MessageToolCalls
              :tool-calls="msg.tool_calls"
              :file-groups="getFileGroupsFromCalls(msg.tool_calls)"
              :confirm-disabled="sending"
              @confirm-prd="emit('confirm-prd', $event)"
            />
          </template>
          <template v-else-if="msg.tool_calls?.length">
            <div v-if="getVisibleToolCallsFromCalls(msg.tool_calls).length" class="mini-tools-block">
              <div
                v-for="(tc, ti) in getVisibleToolCallsFromCalls(msg.tool_calls)"
                :key="`msg-tool-${tc.name}-${ti}`"
                class="mini-tool-tag"
              >
                <el-icon v-if="tc.status === 'streaming' || tc.status === 'running'" class="is-loading" :size="12">
                  <Loading />
                </el-icon>
                <el-icon v-else-if="tc.status === 'ok'" :size="12" color="#67c23a">
                  <CircleCheck />
                </el-icon>
                <el-icon v-else-if="tc.status === 'error'" :size="12" color="#f56c6c">
                  <CircleClose />
                </el-icon>
                <span>{{ getToolDisplayName(tc) }}</span>
              </div>
            </div>
            <PrdPreview
              v-for="(tc, pi) in getPrdCallsFromCalls(msg.tool_calls)"
              :key="`msg-prd-${tc.name}-${pi}`"
              :data="tc.result_data"
              :confirm-disabled="sending"
              show-confirmation
              @confirm="emit('confirm-prd', $event)"
              class="mini-msg-prd-preview"
            />
            <OutputFilesDisplay
              v-if="getFileGroupsFromCalls(msg.tool_calls).length"
              :file-groups="getFileGroupsFromCalls(msg.tool_calls)"
              deletable
              class="mini-msg-files"
            />
            <OutputDisplayFields
              v-if="getDisplayFieldsFromCalls(msg.tool_calls).length"
              :fields="getDisplayFieldsFromCalls(msg.tool_calls)"
              class="mini-msg-display-fields"
            />
          </template>
        </template>
      </template>
    </div>
  </template>
  <div v-else class="mini-ws-empty">
    <span>{{ t('miniWorkstation.startByCommand') }}</span>
  </div>
  <WorkspaceResourceHoverCard
    :preview="resourcePreview"
    @mouseenter="cancelCloseResourcePreview"
    @mouseleave="scheduleCloseResourcePreview"
    @focusin="cancelCloseResourcePreview"
    @focusout="scheduleCloseResourcePreview"
    @open="openResourcePreviewTarget"
    @close="closeResourcePreview"
  />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { CircleCheck, CircleClose, Loading } from '@element-plus/icons-vue'
import type { OutputDisplayField } from '@/architecture/presentation/composables/useOutputDisplayFields'
import type { OutputFileGroup } from '@/architecture/presentation/composables/useOutputFileGroups'
import type { AssistantBlock, ChatMessage, ChatMessageToolCall } from '@/architecture/presentation/composables/useWorkspaceChatStream'
import MessageToolCalls from './MessageToolCalls.vue'
import ModelContextPlanCard from './ModelContextPlanCard.vue'
import OutputDisplayFields from './OutputDisplayFields.vue'
import OutputFilesDisplay from './OutputFilesDisplay.vue'
import PrdPreview from './PrdPreview.vue'
import MiniWorkstationResourceIdentity from './MiniWorkstationResourceIdentity.vue'
import WorkspaceResourceHoverCard from './WorkspaceResourceHoverCard.vue'
import UserDisplay from '@/architecture/presentation/shared/components/UserDisplay.vue'
import { useAuthStore } from '@/architecture/presentation/context/appStoresContext'
import { useWorkspaceResourceHoverPreview } from '@/architecture/presentation/composables/useWorkspaceResourceHoverPreview'
import type { WorkspaceInteraction } from '@/architecture/presentation/context/api/workspace'
import {
  getVisibleWorkspaceToolCalls,
  getWorkspaceToolCallDisplayName
} from '@/architecture/presentation/utils/workspaceRoleDisplay'
import { getWorkspaceResourceSelectionText, renderWorkspaceResourceTokensAsHtml } from './utils/workspaceInvocationSnippet'

const { t } = useI18n()

const authStore = useAuthStore()
const currentUsername = authStore.user?.username || authStore.userName || ''
const {
  resourcePreview,
  showResourcePreviewFromEvent,
  scheduleCloseResourcePreview,
  cancelCloseResourcePreview,
  closeResourcePreview,
  openResourcePreviewTarget,
} = useWorkspaceResourceHoverPreview()

const props = withDefaults(defineProps<{
  messages: ChatMessage[]
  maximized: boolean
  sending: boolean
  counterpartName?: string
  fullCodePath?: string
  resourceType?: string
  resourceTemplateType?: string
  resourceLabels?: Record<string, string>
  streamingDisplayLength: number
  renderMarkdown: (text: string) => string
  formatMessageTime: (value: string) => string
  getFileGroupsFromCalls: (calls: ChatMessageToolCall[]) => OutputFileGroup[]
  getDisplayFieldsFromCalls: (calls: ChatMessageToolCall[]) => OutputDisplayField[]
  pendingInteraction?: WorkspaceInteraction | null
}>(), {
  counterpartName: '',
  fullCodePath: '',
  resourceType: '',
  resourceTemplateType: '',
  resourceLabels: () => ({}),
  pendingInteraction: null,
})

const emit = defineEmits<{
  (e: 'confirm-prd', payload: { remark: string; prd: unknown }): void
  (e: 'view', interaction: WorkspaceInteraction): void
  (e: 'revise', payload: { interaction: WorkspaceInteraction; text: string }): void
  (e: 'cancel', interaction: WorkspaceInteraction): void
  (e: 'confirm', interaction: WorkspaceInteraction): void
}>()

interface RuntimeTimer {
  messageIndex: number
  createdAt: string
  startedAt: number
  completedAt?: number
}

interface ActivityEntry {
  message: ChatMessage
  index: number
}

type MiniDisplayMessage = ChatMessage & {
  display_kind: 'message' | 'activity_group'
  display_key: string
  original_index: number
  activity_entries: ActivityEntry[]
}

const assistantTimer = ref<RuntimeTimer | null>(null)
const assistantTimerNow = ref(Date.now())
let assistantTimerInterval: ReturnType<typeof setInterval> | null = null

const displayMessages = computed<MiniDisplayMessage[]>(() => {
  const out: MiniDisplayMessage[] = []
  let pendingActivity: ActivityEntry[] = []

  const flushActivity = () => {
    if (!pendingActivity.length) return
    const first = pendingActivity[0]
    const last = pendingActivity[pendingActivity.length - 1]
    if (!first || !last) return
    out.push({
      role: 'assistant',
      content: '',
      user: last.message.user,
      created_at: last.message.created_at,
      display_kind: 'activity_group',
      display_key: `activity-${first.index}`,
      original_index: first.index,
      activity_entries: pendingActivity
    })
    pendingActivity = []
  }

  const pushMessage = (message: ChatMessage, index: number, activityEntries: ActivityEntry[] = []) => {
    out.push({
      ...message,
      display_kind: 'message',
      display_key: `message-${index}-${message.role}-${message.created_at || ''}-${activityEntries[0]?.index ?? 'plain'}`,
      original_index: index,
      activity_entries: activityEntries
    })
  }

  props.messages.forEach((message, index) => {
    if (isCollapsibleActivityMessage(message)) {
      pendingActivity.push({ message, index })
      return
    }
    if (message.role === 'assistant' && pendingActivity.length) {
      const activityEntries = pendingActivity
      pendingActivity = []
      pushMessage(message, index, activityEntries)
      return
    }
    flushActivity()
    pushMessage(message, index)
  })

  flushActivity()
  return out
})

function getDisplayMessageIndex(message: MiniDisplayMessage, fallback: number): number {
  return typeof message.original_index === 'number' ? message.original_index : fallback
}

function isCollapsibleActivityMessage(message: ChatMessage): boolean {
  if (message.role !== 'assistant') return false
  if (hasAssistantPrimaryContent(message)) return false
  const calls = getAllToolCallsFromMessage(message)
  if (hasImportantToolSurface(calls)) return false
  return hasTechnicalActivity(message, calls)
}

function hasAssistantPrimaryContent(message: ChatMessage): boolean {
  if ((message.content || '').trim()) return true
  if (!message.blocks?.length) return false
  return message.blocks.some((block) => block.type === 'content' && block.text.trim())
}

function hasTechnicalActivity(message: ChatMessage, calls = getAllToolCallsFromMessage(message)): boolean {
  return calls.length > 0 ||
    !!message.model_context_plan ||
    !!message.model_context_plans?.length ||
    getThinkingBlocksFromMessage(message).length > 0
}

function hasImportantToolSurface(calls: ChatMessageToolCall[]): boolean {
  if (!calls.length) return false
  return getPrdCallsFromCalls(calls).length > 0 ||
    props.getFileGroupsFromCalls(calls).length > 0 ||
    props.getDisplayFieldsFromCalls(calls).length > 0
}

function getAllToolCallsFromMessage(message: ChatMessage): ChatMessageToolCall[] {
  if (message.blocks?.length) {
    return message.blocks.flatMap((block) => block.type === 'tool_calls' ? block.calls : [])
  }
  return message.tool_calls || []
}

function getToolCallBlocksFromMessage(message: ChatMessage): ChatMessageToolCall[][] {
  if (message.blocks?.length) {
    return message.blocks
      .filter((block): block is { type: 'tool_calls'; calls: ChatMessageToolCall[] } => block.type === 'tool_calls')
      .map((block) => block.calls)
      .filter((calls) => getVisibleToolCallsFromCalls(calls).length > 0)
  }
  return message.tool_calls?.length ? [message.tool_calls] : []
}

function getThinkingBlocksFromMessage(message: ChatMessage): { text: string }[] {
  if (!message.blocks?.length) return []
  return message.blocks
    .filter((block): block is { type: 'thinking'; text: string } => block.type === 'thinking')
    .filter((block) => block.text.trim())
}

function shouldRenderAssistantTechnicalInline(message: ChatMessage): boolean {
  if (!hasAssistantPrimaryContent(message)) return true
  return hasImportantToolSurface(getAllToolCallsFromMessage(message))
}

function getRenderBlocksForMessage(message: ChatMessage): AssistantBlock[] {
  if (!message.blocks?.length) return []
  if (shouldRenderAssistantTechnicalInline(message)) return message.blocks
  return message.blocks.filter((block) => block.type === 'content')
}

function getActivityEntriesForMessage(message: MiniDisplayMessage): ActivityEntry[] {
  const entries = [...(message.activity_entries || [])]
  if (
    message.display_kind === 'message' &&
    hasAssistantPrimaryContent(message) &&
    !shouldRenderAssistantTechnicalInline(message) &&
    hasTechnicalActivity(message)
  ) {
    entries.push({ message, index: message.original_index })
  }
  return entries
}

function getAssistantOutputSize(message: ChatMessage): number {
  let size = message.content?.length ?? 0
  if (message.blocks?.length) {
    for (const block of message.blocks) {
      if (block.type === 'content') {
        size += block.text.length
      } else if (block.type === 'thinking') {
        size += block.text.length
      } else {
        const visibleCalls = getVisibleToolCallsFromCalls(block.calls)
        size += visibleCalls.length
        for (const call of visibleCalls) {
          size += (call.arguments?.length ?? 0) + (call.result?.length ?? 0) + (call.error?.length ?? 0)
        }
      }
    }
  }
  if (message.tool_calls?.length) {
    const visibleCalls = getVisibleToolCallsFromCalls(message.tool_calls)
    size += visibleCalls.length
    for (const call of visibleCalls) {
      size += (call.arguments?.length ?? 0) + (call.result?.length ?? 0) + (call.error?.length ?? 0)
    }
  }
  if (message.model_context_plans?.length || message.model_context_plan) {
    size += message.model_context_plans?.length || 1
  }
  return size
}

function hasAssistantVisibleOutput(message: ChatMessage): boolean {
  return message.role === 'assistant' && getAssistantOutputSize(message) > 0
}

function isAssistantTimerTarget(message: ChatMessage, msgIndex: number): boolean {
  const timer = assistantTimer.value
  return !!timer && message.role === 'assistant' && timer.messageIndex === msgIndex && timer.createdAt === (message.created_at || '')
}

function syncAssistantTimerInterval() {
  const timer = assistantTimer.value
  if (timer && timer.completedAt == null) {
    if (assistantTimerInterval == null) {
      assistantTimerInterval = setInterval(() => {
        assistantTimerNow.value = Date.now()
      }, 250)
    }
    return
  }
  if (assistantTimerInterval != null) {
    clearInterval(assistantTimerInterval)
    assistantTimerInterval = null
  }
}

function syncAssistantTimer() {
  const now = Date.now()
  const lastIndex = props.messages.length - 1
  const lastMessage = props.messages[lastIndex]

  if (props.sending && lastMessage?.role === 'assistant' && hasAssistantVisibleOutput(lastMessage)) {
    const createdAt = lastMessage.created_at || ''
    const timer = assistantTimer.value
    if (!timer || timer.messageIndex !== lastIndex || timer.createdAt !== createdAt || timer.completedAt != null) {
      assistantTimer.value = { messageIndex: lastIndex, createdAt, startedAt: now }
      assistantTimerNow.value = now
    }
  } else if (!props.sending && assistantTimer.value && assistantTimer.value.completedAt == null) {
    assistantTimer.value = { ...assistantTimer.value, completedAt: now }
    assistantTimerNow.value = now
  }

  const timer = assistantTimer.value
  if (timer) {
    const targetMessage = props.messages[timer.messageIndex]
    if (!targetMessage || targetMessage.role !== 'assistant' || (targetMessage.created_at || '') !== timer.createdAt) {
      assistantTimer.value = null
    }
  }

  syncAssistantTimerInterval()
}

function formatDuration(ms: number): string {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (hours > 0) return t('miniWorkstation.durationHours', { hours, minutes, seconds })
  if (minutes > 0) return t('miniWorkstation.durationMinutes', { minutes, seconds })
  return t('miniWorkstation.durationSeconds', { seconds })
}

function getAssistantDurationLabel(message: ChatMessage, msgIndex: number): string {
  if (!isAssistantTimerTarget(message, msgIndex)) return ''
  const timer = assistantTimer.value
  if (!timer) return ''
  const end = timer.completedAt ?? assistantTimerNow.value
  return formatDuration(end - timer.startedAt)
}

function isAssistantTimerRunning(message: ChatMessage, msgIndex: number): boolean {
  const timer = assistantTimer.value
  return isAssistantTimerTarget(message, msgIndex) && !!timer && timer.completedAt == null
}

function isStreamingThinkingBlock(msgIndex: number, blockIndex: number, blockCount: number): boolean {
  return props.sending && msgIndex === props.messages.length - 1 && blockIndex === blockCount - 1
}

function renderContentBlock(text: string, msgIndex: number, blockIndex: number, blockCount: number): string {
  const isStreamingTail =
    props.sending &&
    msgIndex === props.messages.length - 1 &&
    blockIndex === blockCount - 1

  return renderWorkbenchMarkdown(isStreamingTail ? text.slice(0, props.streamingDisplayLength) : text)
}

function renderWorkbenchMarkdown(text: string): string {
  return props.renderMarkdown(renderWorkspaceResourceTokensAsHtml(text, props.fullCodePath, props.resourceLabels))
}

function onWorkspaceMarkdownCopy(event: ClipboardEvent) {
  const root = event.currentTarget instanceof HTMLElement ? event.currentTarget : null
  const text = getWorkspaceResourceSelectionText(root)
  if (!text) return
  event.preventDefault()
  event.clipboardData?.setData('text/plain', text)
}

function getPrdCallsFromCalls(calls: ChatMessageToolCall[]): ChatMessageToolCall[] {
  return calls.filter((call) => call.name === 'write_prd' && call.status === 'ok' && call.result_data != null)
}

function getVisibleToolCallsFromCalls(calls: ChatMessageToolCall[]): ChatMessageToolCall[] {
  return getVisibleWorkspaceToolCalls(calls)
}

function getToolDisplayName(call: ChatMessageToolCall): string {
  return getWorkspaceToolCallDisplayName(call)
}

function getActivityGroupToolCalls(message: MiniDisplayMessage): ChatMessageToolCall[] {
  return getActivityEntriesForMessage(message).flatMap((entry) => getAllToolCallsFromMessage(entry.message))
}

function getActivityGroupVisibleToolCount(message: MiniDisplayMessage): number {
  return getVisibleToolCallsFromCalls(getActivityGroupToolCalls(message)).length
}

function getActivityGroupProcessLabel(message: MiniDisplayMessage): string {
  const toolCount = getActivityGroupVisibleToolCount(message)
  if (toolCount > 0) return `${toolCount} 个工具`
  const stepCount = Math.max(1, getActivityEntriesForMessage(message).length)
  return `${stepCount} 段`
}

function getActivityGroupErrorCount(message: MiniDisplayMessage): number {
  return getActivityGroupToolCalls(message).filter((call) => call.status === 'error').length
}

function isToolCallRunning(call: ChatMessageToolCall): boolean {
  return call.status === 'streaming' || call.status === 'running'
}

function isActivityGroupRunning(message: MiniDisplayMessage): boolean {
  return getActivityGroupToolCalls(message).some(isToolCallRunning)
}

function getActivityGroupStatusLabel(message: MiniDisplayMessage): string {
  if (isActivityGroupRunning(message)) return '执行中'
  const errors = getActivityGroupErrorCount(message)
  if (errors > 0) return `${errors} 失败`
  return '完成'
}

function getActivityGroupStatusClass(message: MiniDisplayMessage): string {
  if (isActivityGroupRunning(message)) return 'is-running'
  if (getActivityGroupErrorCount(message) > 0) return 'is-error'
  return 'is-ok'
}

function getActivityMessageStatusLabel(message: ChatMessage): string {
  const calls = getAllToolCallsFromMessage(message)
  if (calls.some(isToolCallRunning)) return '执行中'
  const errors = calls.filter((call) => call.status === 'error').length
  if (errors > 0) return `${errors} 失败`
  return '完成'
}

function getActivityMessageStatusClass(message: ChatMessage): string {
  const calls = getAllToolCallsFromMessage(message)
  if (calls.some(isToolCallRunning)) return 'is-running'
  if (calls.some((call) => call.status === 'error')) return 'is-error'
  return 'is-ok'
}

function getActivityGroupToolSummary(message: MiniDisplayMessage): string {
  const calls = getActivityGroupToolCalls(message)
  if (calls.length) return summarizeToolCalls(calls, 4)
  if (getActivityEntriesForMessage(message).some((entry) => getThinkingBlocksFromMessage(entry.message).length > 0)) return '思考'
  return '模型上下文'
}

function getActivityGroupThinkingPreview(message: MiniDisplayMessage): string {
  const entries = getActivityEntriesForMessage(message)
  for (let i = entries.length - 1; i >= 0; i--) {
    const entry = entries[i]
    if (!entry) continue
    const blocks = getThinkingBlocksFromMessage(entry.message)
    for (let j = blocks.length - 1; j >= 0; j--) {
      const block = blocks[j]
      if (!block) continue
      const preview = compactThinkingPreview(block.text)
      if (preview) return preview
    }
  }
  return ''
}

function compactThinkingPreview(text: string): string {
  const compact = String(text || '').replace(/\s+/g, ' ').trim()
  if (!compact) return ''
  const maxLength = 86
  return compact.length > maxLength ? `...${compact.slice(compact.length - maxLength)}` : compact
}

function getActivityEntryToolSummary(message: ChatMessage): string {
  const calls = getAllToolCallsFromMessage(message)
  const visible = getVisibleToolCallsFromCalls(calls)
  if (visible.length) return `工具 ${visible.length} 个 · ${summarizeToolCalls(calls, 3)}`
  if (getThinkingBlocksFromMessage(message).length > 0) return '思考'
  return '模型上下文'
}

function summarizeToolCalls(calls: ChatMessageToolCall[], maxItems: number): string {
  const visible = getVisibleToolCallsFromCalls(calls)
  if (!visible.length) return '工具调用'
  const counts = new Map<string, number>()
  for (const call of visible) {
    const name = getToolDisplayName(call)
    counts.set(name, (counts.get(name) || 0) + 1)
  }
  const entries = Array.from(counts.entries())
  const shown = entries.slice(0, maxItems).map(([name, count]) => count > 1 ? `${name} x${count}` : name)
  const hidden = entries.length - shown.length
  return hidden > 0 ? `${shown.join(' · ')} · +${hidden}` : shown.join(' · ')
}

function getActivityEntryRoundLabel(entry: ActivityEntry): string {
  const round = entry.message.model_context_plan?.round
  if (typeof round === 'number') return `模型第 ${round + 1} 轮`
  return `消息第 ${entry.index + 1} 条`
}

function getAssistantModelLabel(message: ChatMessage): string {
  if (message.llm_config_name) return message.llm_config_name
  const provider = (message.llm_provider || '').trim()
  const model = (message.llm_model || '').trim()
  if (provider && model) return `${provider}/${model}`
  return model || provider
}

function getAssistantCacheLabel(message: ChatMessage): string {
  const usage = message.llm_usage
  if (!usage || usage.total_tokens <= 0) return ''
  if (usage.cached_tokens_reported === false) return t('miniWorkstation.cacheNotReported')
  const cached = Math.max(0, usage.cached_tokens || 0)
  const prompt = Math.max(0, usage.prompt_tokens || 0)
  const rate = prompt > 0 ? Math.round((cached / prompt) * 100) : 0
  return t('miniWorkstation.cacheLabel', { cached: formatTokenCount(cached), rate })
}

function getAssistantCacheTitle(message: ChatMessage): string {
  const usage = message.llm_usage
  if (!usage) return ''
  if (usage.cached_tokens_reported === false) {
    return [
      t('miniWorkstation.cacheMissingTitle'),
      t('miniWorkstation.inputTokens', { tokens: formatTokenCount(usage.prompt_tokens) }),
      t('miniWorkstation.outputTokens', { tokens: formatTokenCount(usage.completion_tokens) }),
      t('miniWorkstation.totalTokens', { tokens: formatTokenCount(usage.total_tokens) })
    ].join(' · ')
  }
  return [
    t('miniWorkstation.inputTokens', { tokens: formatTokenCount(usage.prompt_tokens) }),
    t('miniWorkstation.cachedTokens', { tokens: formatTokenCount(usage.cached_tokens) }),
    t('miniWorkstation.outputTokens', { tokens: formatTokenCount(usage.completion_tokens) }),
    t('miniWorkstation.totalTokens', { tokens: formatTokenCount(usage.total_tokens) })
  ].join(' · ')
}

function formatTokenCount(value: number): string {
  const n = Math.max(0, Math.round(value || 0))
  if (n >= 1_000_000) return `${trimTrailingZeros(n / 1_000_000)}m`
  if (n >= 1000) return `${trimTrailingZeros(n / 1000)}k`
  return String(n)
}

function trimTrailingZeros(value: number): string {
  return value.toFixed(value >= 10 ? 0 : 1).replace(/\.0$/, '')
}

watch(
  () => [
    props.sending ? 'sending' : 'idle',
    props.messages.length,
    props.messages.map((message, index) => `${index}:${message.role}:${message.created_at || ''}:${getAssistantOutputSize(message)}`).join('|')
  ].join(':'),
  syncAssistantTimer,
  { immediate: true }
)

onBeforeUnmount(() => {
  closeResourcePreview()
  if (assistantTimerInterval != null) clearInterval(assistantTimerInterval)
})
</script>

<style scoped>
.mini-ws-empty {
  box-sizing: border-box;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  min-height: 80px;
  color: var(--text-placeholder);
  font-size: 13px;
  letter-spacing: 0;
  text-transform: none;
  text-align: center;
}

.mini-msg {
  margin-bottom: 12px;
  animation: miniMsgEnter 0.22s ease-out;
}
.mini-msg--activity-group {
  margin: 4px 0 8px;
}
.mini-msg-user {
  display: flex;
  flex-direction: column;
  gap: 7px;
  align-items: flex-end;
}
.mini-msg-user-header {
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  gap: 7px;
  max-width: 100%;
}
.mini-msg-user-display {
  flex-shrink: 1;
  min-width: 0;
}
.mini-msg-user-display :deep(.user-display-wrapper) {
  display: inline-flex;
  color: var(--text-primary);
}
.mini-msg-time {
  flex-shrink: 0;
  font-size: 11px;
  color: var(--text-placeholder);
  margin-top: 2px;
}
.mini-msg-assistant-header {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  margin-bottom: 4px;
}
.mini-msg-model {
  flex-shrink: 1;
  min-width: 0;
  max-width: 220px;
  overflow: hidden;
  padding: 2px 6px;
  border: 1px solid rgba(var(--color-primary-rgb), 0.18);
  border-radius: 999px;
  background: rgba(var(--color-primary-rgb), 0.08);
  color: var(--color-primary);
  font-size: 10px;
  font-weight: 700;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mini-msg-cache {
  flex-shrink: 0;
  padding: 2px 6px;
  border: 1px solid rgba(var(--color-success-rgb), 0.18);
  border-radius: 999px;
  background: rgba(var(--color-success-rgb), 0.08);
  color: var(--color-success);
  font-size: 10px;
  font-weight: 700;
  line-height: 1.2;
  white-space: nowrap;
}
.mini-msg-model-context {
  margin-bottom: 3px;
}
.mini-msg-output-duration {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  padding: 2px 6px;
  border: 1px solid var(--border-light);
  border-radius: 999px;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  font-size: 10px;
  font-weight: 700;
  line-height: 1.2;
}
.mini-msg-output-duration--running {
  border-color: rgba(var(--color-primary-rgb), 0.2);
  color: var(--color-primary);
}
.mini-msg-output-duration-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentColor;
  box-shadow: none;
}
.mini-msg-assistant {
  padding: 8px 10px;
  border: 1px solid var(--border-light);
  border-radius: 10px;
  background: var(--bg-primary);
  box-shadow: none;
}

.mini-activity-group {
  color: var(--text-secondary);
}
.mini-activity-group--attached {
  margin: -1px 0 5px;
}
.mini-activity-group--attached .mini-activity-summary {
  min-height: 22px;
  padding: 2px 6px;
  border-color: transparent;
  color: var(--text-placeholder);
  font-size: 10px;
}
.mini-activity-group--attached .mini-activity-main {
  color: var(--text-secondary);
}
.mini-activity-details {
  width: 100%;
}
.mini-activity-summary {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
  min-height: 26px;
  padding: 3px 8px;
  border: 1px solid var(--border-light);
  border-radius: 8px;
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  list-style: none;
  font-size: 11px;
}
.mini-activity-summary::-webkit-details-marker {
  display: none;
}
.mini-activity-summary::before {
  content: '›';
  flex-shrink: 0;
  color: var(--text-placeholder);
}
.mini-activity-details[open] .mini-activity-summary {
  border-radius: 8px 8px 0 0;
}
.mini-activity-details[open] .mini-activity-summary::before {
  transform: rotate(90deg);
}
.mini-activity-main,
.mini-activity-status,
.mini-activity-step-status {
  display: inline-flex;
  align-items: center;
  flex-shrink: 0;
  gap: 4px;
  white-space: nowrap;
}
.mini-activity-main {
  color: var(--text-primary);
  font-weight: 600;
}
.mini-activity-tools {
  min-width: 0;
  overflow: hidden;
  color: var(--text-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mini-activity-thinking-preview {
  flex: 1 1 auto;
  min-width: 80px;
  overflow: hidden;
  color: var(--text-placeholder);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mini-activity-status {
  margin-left: auto;
  font-weight: 600;
}
.mini-activity-status.is-running,
.mini-activity-step-status.is-running {
  color: var(--color-primary);
}
.mini-activity-status.is-ok,
.mini-activity-step-status.is-ok {
  color: var(--color-success);
}
.mini-activity-status.is-error,
.mini-activity-step-status.is-error {
  color: var(--color-danger);
}
.mini-activity-detail {
  max-height: 260px;
  overflow: auto;
  padding: 5px 6px 6px;
  border: 1px solid var(--border-light);
  border-top: none;
  border-radius: 0 0 8px 8px;
  background: transparent;
}
.mini-msg--maximized .mini-activity-detail {
  max-height: 360px;
}
.mini-activity-step {
  border-bottom: 1px solid var(--border-light);
}
.mini-activity-step:last-child {
  border-bottom: none;
}
.mini-activity-step-head {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
  padding: 5px 2px;
  cursor: pointer;
  list-style: none;
  color: var(--text-secondary);
  font-size: 10px;
}
.mini-activity-step-head::-webkit-details-marker {
  display: none;
}
.mini-activity-step-head::before {
  content: '›';
  flex-shrink: 0;
  color: var(--text-placeholder);
}
.mini-activity-step[open] .mini-activity-step-head::before {
  transform: rotate(90deg);
}
.mini-activity-step-head span:nth-child(3) {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mini-activity-step-status {
  margin-left: auto;
}
.mini-activity-step-body {
  padding: 0 2px 6px 14px;
}
.mini-activity-model-context {
  margin: 0 0 4px;
}

.mini-content-block {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  font-family: inherit;
  color: var(--text-primary);
  word-break: break-word;
}
.mini-thinking-block {
  margin: 0 0 5px;
  color: var(--text-placeholder);
}
.mini-thinking-head {
  display: flex;
  align-items: center;
  gap: 4px;
  width: fit-content;
  cursor: pointer;
  list-style: none;
  padding: 1px 0;
  font-size: 10px;
  font-weight: 600;
  line-height: 1.2;
  color: var(--text-placeholder);
}
.mini-thinking-head::-webkit-details-marker {
  display: none;
}
.mini-thinking-head::before {
  content: '›';
  display: inline-block;
  transform: rotate(0deg);
  color: var(--text-placeholder);
}
.mini-thinking-block[open] .mini-thinking-head::before {
  transform: rotate(90deg);
}
.mini-thinking-body {
  max-height: 72px;
  overflow: auto;
  margin-top: 3px;
  padding: 5px 7px;
  border-left: 2px solid var(--border-light);
  font-size: 10px;
  line-height: 1.45;
  background: transparent;
  color: var(--text-placeholder);
  white-space: pre-wrap;
  word-break: break-word;
}
.mini-msg--maximized .mini-thinking-body {
  max-height: 96px;
}
.mini-md-content :deep(p) {
  margin: 0 0 6px;
}
.mini-md-content :deep(p:last-child) {
  margin-bottom: 0;
}
.mini-md-content :deep(ul),
.mini-md-content :deep(ol) {
  margin: 4px 0;
  padding-left: 18px;
}
.mini-md-content :deep(li) {
  margin: 2px 0;
}
.mini-md-content :deep(code) {
  background: var(--bg-tertiary);
  border: 1px solid var(--border-light);
  padding: 1px 4px;
  border-radius: 3px;
  font-size: 11px;
  font-family: 'SF Mono', 'Fira Code', monospace;
  color: var(--text-primary);
}
.mini-md-content :deep(pre) {
  background: var(--bg-tertiary);
  color: var(--text-primary);
  border: 1px solid var(--border-light);
  padding: 8px 10px;
  border-radius: 10px;
  overflow-x: auto;
  margin: 6px 0;
  font-size: 11px;
  line-height: 1.5;
  box-shadow: none;
}
.mini-md-content :deep(pre code) {
  background: none;
  padding: 0;
  font-size: inherit;
  color: inherit;
}
.mini-md-content :deep(h1),
.mini-md-content :deep(h2),
.mini-md-content :deep(h3),
.mini-md-content :deep(h4) {
  margin: 8px 0 4px;
  font-size: 13px;
  font-weight: 600;
}
.mini-md-content :deep(h1) { font-size: 15px; }
.mini-md-content :deep(h2) { font-size: 14px; }
.mini-md-content :deep(blockquote) {
  margin: 4px 0;
  padding: 2px 8px;
  border-left: 3px solid rgba(var(--color-primary-rgb), 0.24);
  color: var(--text-secondary);
  background: var(--bg-tertiary);
}
.mini-md-content :deep(table) {
  border-collapse: collapse;
  margin: 6px 0;
  font-size: 11px;
  width: 100%;
}
.mini-md-content :deep(th),
.mini-md-content :deep(td) {
  border: 1px solid var(--border-light);
  padding: 3px 6px;
}
.mini-md-content :deep(th) {
  background: var(--bg-tertiary);
  font-weight: 600;
}
.mini-md-content :deep(a) {
  color: var(--color-primary);
  text-decoration: none;
}
.mini-md-content :deep(img) {
  display: block;
  width: min(100%, 680px);
  max-width: 100%;
  height: auto;
  max-height: min(54vh, 520px);
  margin: 10px 0;
  object-fit: contain;
  border: 1px solid var(--border-light);
  border-radius: 10px;
  background: #0b0f16;
  box-shadow: none;
}
.mini-msg--maximized .mini-md-content :deep(img) {
  width: min(100%, 1080px);
  max-height: min(72vh, 820px);
}
.mini-md-content :deep(.workspace-resource-token) {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  max-width: 100%;
  vertical-align: baseline;
  padding: 1px 7px 1px 5px;
  margin: 0 1px;
  border: 1px solid rgba(var(--color-primary-rgb), 0.22);
  border-radius: 7px;
  background: rgba(var(--color-primary-rgb), 0.08);
  color: var(--text-primary);
  font-size: 0.92em;
  font-weight: 600;
  line-height: 1.55;
  text-decoration: none;
  white-space: nowrap;
}
.mini-md-content :deep(.workspace-resource-token:hover),
.mini-md-content :deep(.workspace-resource-token:focus-visible) {
  border-color: rgba(var(--color-primary-rgb), 0.44);
  background: rgba(var(--color-primary-rgb), 0.13);
  color: var(--color-primary);
  outline: none;
  text-decoration: none;
}
.mini-md-content :deep(.workspace-resource-token__icon) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 16px;
  height: 16px;
  line-height: 1;
}
.mini-md-content :deep(.workspace-resource-token__img),
.mini-md-content :deep(.workspace-resource-token__svg),
.mini-md-content :deep(.workspace-resource-token__glyph) {
  display: block;
  width: 16px;
  height: 16px;
  max-width: 16px;
  max-height: 16px;
  margin: 0;
  border: none;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
  object-fit: contain;
}
.mini-md-content :deep(.workspace-resource-token__glyph) {
  border-radius: 5px;
  background: linear-gradient(135deg, rgba(var(--color-primary-rgb), 0.95), rgba(var(--color-primary-rgb), 0.58));
  position: relative;
}
.mini-md-content :deep(.workspace-resource-token__glyph::after) {
  content: '';
  position: absolute;
  inset: 4px;
  border: 2px solid rgba(255, 255, 255, 0.9);
  border-radius: 999px;
}
.mini-md-content :deep(.workspace-resource-token__label) {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.mini-md-content :deep(.workspace-resource-token__type) {
  color: var(--text-secondary);
  font-size: 0.82em;
  font-weight: 700;
}
.mini-md-content :deep(hr) {
  border: none;
  border-top: 1px solid var(--border-light);
  margin: 8px 0;
}
.mini-tools-block {
  display: flex;
  flex-wrap: nowrap;
  gap: 3px;
  max-width: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  margin: 2px 0 4px;
  padding-bottom: 1px;
  scrollbar-width: none;
}
.mini-tools-block::-webkit-scrollbar {
  display: none;
}
.mini-tool-tag {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex: 0 0 auto;
  max-width: 180px;
  font-size: 10px;
  color: var(--text-secondary);
  border: 1px solid var(--border-light);
  background: transparent;
  padding: 1px 5px;
  border-radius: 999px;
  line-height: 1.4;
}
.mini-tool-tag span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mini-msg-user-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 10px;
  border: 1px solid rgba(var(--color-primary-rgb), 0.18);
  border-radius: 10px;
  background: rgba(var(--color-primary-rgb), 0.08);
  color: var(--text-primary);
}
.mini-msg-user-body--with-files {
  align-self: stretch;
  width: 100%;
}
.mini-msg-files {
  margin: 4px 0;
}

.mini-msg-prd-preview {
  margin: 6px 0;
}
.mini-msg-prd-preview :deep(.prd-preview) {
  --prd-bg: var(--bg-primary);
  --prd-surface: var(--bg-secondary);
  --prd-surface-strong: var(--bg-secondary);
  --prd-control-bg: var(--bg-tertiary);
  --prd-muted-bg: var(--bg-tertiary);
  --prd-primary-bg: rgba(var(--color-primary-rgb), 0.08);
  --prd-danger-bg: rgba(248, 113, 113, 0.14);
  --prd-warning-bg: rgba(251, 191, 36, 0.13);
  --prd-border: var(--border-light);
  --prd-border-soft: var(--border-light);
  --prd-shadow: none;
  --el-text-color-primary: var(--text-primary);
  --el-text-color-regular: var(--text-primary);
  --el-text-color-secondary: var(--text-secondary);
  --el-text-color-placeholder: var(--text-placeholder);
  --el-border-color: var(--border-light);
  --el-border-color-light: var(--border-light);
  --el-border-color-lighter: var(--border-light);
  --el-border-color-extra-light: var(--border-light);
  --el-fill-color-blank: var(--bg-secondary);
  --el-fill-color-extra-light: var(--bg-primary);
  --el-fill-color-light: var(--bg-tertiary);

  margin-top: 0;
  border-color: var(--border-light);
  background: var(--bg-primary);
  color: var(--text-primary);
}
.mini-msg-prd-preview :deep(.prd-preview-head),
.mini-msg-prd-preview :deep(.prd-section) {
  padding: 9px 10px;
}
.mini-msg-prd-preview :deep(.prd-table) {
  font-size: 11px;
}

.mini-msg-files :deep(.output-files-head) {
  font-size: 11px;
  margin-bottom: 4px;
}
.mini-msg-files :deep(.output-files-wrap) {
  padding: 6px;
  border-color: var(--border-light);
  background: var(--bg-primary);
}
.mini-msg-files :deep(.output-files-item) {
  padding: 6px;
  min-width: 120px;
  min-height: 0;
  border-color: var(--border-light);
  background: var(--bg-secondary);
}
.mini-msg-files :deep(.output-files-main) {
  grid-template-columns: 40px minmax(0, 1fr);
  gap: 8px;
}
.mini-msg-files :deep(.output-files-preview) {
  width: 40px;
  height: 40px;
}
.mini-msg-files :deep(.output-files-icon) {
  width: 40px;
  height: 40px;
  font-size: 18px;
}
.mini-msg-files :deep(.output-files-name) {
  font-size: 11px;
}
.mini-msg-files :deep(.output-files-meta) {
  font-size: 10px;
}
.mini-msg-files :deep(.output-files-footer) {
  align-items: flex-start;
  flex-direction: column;
  gap: 4px;
  padding-top: 6px;
}
.mini-msg-files :deep(.output-files-actions) {
  font-size: 11px;
  gap: 8px;
}

/* 图片、视频是消息正文，不应沿用普通附件的 40px 图标布局。 */
.mini-msg-files :deep(.output-files-item--media) {
  grid-column: 1 / -1;
  width: 100%;
  padding: 8px;
}
.mini-msg-files :deep(.output-files-item--media .output-files-main) {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 8px;
}
.mini-msg-files :deep(.output-files-item--media .output-files-preview) {
  width: 100%;
  height: clamp(210px, 48vw, 460px);
  aspect-ratio: auto;
  border-radius: 10px;
  background: #0b0f16;
}
.mini-msg-files :deep(.output-files-item--media .output-files-info) {
  width: 100%;
  margin-top: 0;
}
.mini-msg-files :deep(.output-files-item--media .output-files-footer) {
  align-items: center;
  flex-direction: row;
}

/* 最大化时工具详情使用 MessageToolCalls，仍在消息节点下统一放大媒体预览。 */
.mini-msg--maximized :deep(.output-files-item--media .output-files-preview) {
  width: 100%;
  height: clamp(360px, 58vw, 760px);
  aspect-ratio: auto;
}

.mini-msg-display-fields {
  margin: 4px 0;
}
.mini-msg-display-fields :deep(.odf-head) {
  font-size: 11px;
  margin-bottom: 4px;
}
.mini-msg-display-fields :deep(.odf-card-header) {
  padding: 4px 8px;
}
.mini-msg-display-fields :deep(.odf-label) {
  font-size: 11px;
}
.mini-msg-display-fields :deep(.odf-value) {
  padding: 4px 8px;
}
.mini-msg-display-fields :deep(.odf-pre) {
  font-size: 11px;
}

@keyframes miniMsgEnter {
  from {
    opacity: 0;
    transform: translateY(6px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
