import { computed, nextTick, ref, type Ref } from 'vue'
import { ElMessage } from 'element-plus'
import type { ChatMessage } from '@/architecture/presentation/composables/useWorkspaceChatStream'
import {
  createWorkspaceHandoff,
  recordWorkspaceInteractionEvent,
  resolveWorkspaceSessionInteraction,
  type WorkspaceInteraction
} from '@/architecture/presentation/context/api/workspace'
import { translate } from '@/architecture/shared/i18n'

export interface UseMiniWorkstationPendingInteractionOptions {
  messages: Ref<ChatMessage[]>
  sessionId: Ref<string | undefined>
  fullCodePath: Ref<string>
  sending: Ref<boolean>
  currentSessionDisablesPendingInteraction: Ref<boolean>
  outputRef: Ref<HTMLElement | undefined>
  loadMiniSessions: () => void | Promise<void>
  loadGlobalSessions: () => void | Promise<void>
  loadMiniSessionMessages: (sessionId: string) => Promise<void>
  sendTextToSession: (
    sessionId: string,
    content: string,
    displayText?: string,
    meta?: { contextUsage?: string; artifactKind?: string; interactionAction?: string; resume?: boolean }
  ) => Promise<boolean>
}

type StageInteractionArtifact = Record<string, unknown> & {
  kind?: string
  interaction?: Partial<WorkspaceInteraction>
}

export function useMiniWorkstationPendingInteraction(options: UseMiniWorkstationPendingInteractionOptions) {
  const handledInteractionKeys = ref<Set<string>>(new Set())

  // 阶段确认不再是会话级状态。历史消息即使带有 pending_confirmation，
  // 也只在 PRD 预览中提供快捷确认，不恢复输入框卡点。
  const pendingInteraction = computed<WorkspaceInteraction | null>(() => null)

  const composerBlocked = computed(() => {
    const interaction = pendingInteraction.value
    return !!interaction && isComposerBlockingInteraction(interaction)
  })

  const composerBlockedLabel = computed(() => {
    const interaction = pendingInteraction.value
    if (!interaction) return ''
    if (interaction.card_type === 'prd_confirmation') return translate('miniWorkstation.blockingPrd')
    if (interaction.card_type === 'build_repair') return translate('miniWorkstation.blockingRepair')
    return translate('miniWorkstation.blockingGeneric')
  })

  const composerBlockedPlaceholder = computed(() => {
    const interaction = pendingInteraction.value
    if (!interaction) return translate('miniWorkstation.composerDefaultPlaceholder')
    return interaction.help_text || interaction.description || translate('miniWorkstation.interactionNeedAction')
  })

  async function handleBeforeSend(_payload: { text: string; files: unknown[] | null }) {
    const interaction = pendingInteraction.value
    if (!interaction) {
      return false
    }
    if (isComposerBlockingInteraction(interaction)) {
      ElMessage.warning(translate('miniWorkstation.interactionHandleFirst'))
      return { cancel: true, preserveDraft: true }
    }
    if (interaction.card_type === 'build_repair') {
      markInteractionHandled(interaction)
      return { interactionAction: 'continue_development' }
    }
    return false
  }

  async function viewPendingInteraction(target?: WorkspaceInteraction) {
    const interaction = target || pendingInteraction.value
    if (!interaction) return
    await recordPendingInteractionAction(interaction, viewInteractionAction(interaction))
    await nextTick()
    focusInteractionArtifact(interaction)
  }

  function confirmPendingInteraction(target?: WorkspaceInteraction) {
    const interaction = target || pendingInteraction.value
    if (!interaction) return
    if (interaction.card_type === 'build_repair') {
      void (async () => {
        await recordPendingInteractionAction(interaction, 'start_build_repair')
        await handleConfirmBuildHandoff({ artifact: interaction.artifact })
      })()
      return
    }
    if (interaction.card_type === 'prd_confirmation') {
      void (async () => {
        await recordPendingInteractionAction(interaction, 'confirm_prd')
        await handleConfirmPrd({ remark: '', prd: interaction.artifact }, { auditRecorded: true })
      })()
      return
    }
    ElMessage.warning(translate('miniWorkstation.confirmActionMissing'))
  }

  async function revisePendingInteraction(payload: { text: string; interaction?: WorkspaceInteraction }) {
    const interaction = payload.interaction || pendingInteraction.value
    const text = payload.text.trim()
    if (!interaction || !options.sessionId.value || !text || options.sending.value) return
    const isBuildRepair = interaction.card_type === 'build_repair'
    if (!isBuildRepair && interaction.card_type !== 'prd_confirmation') {
      ElMessage.warning(translate('miniWorkstation.reviseActionMissing'))
      return
    }
    const prefix = isBuildRepair ? translate('miniWorkstation.continueDevelopment') : translate('miniWorkstation.revisePrd')
    const action = isBuildRepair ? 'continue_development' : 'revise_prd'
    await recordPendingInteractionAction(interaction, action, `${prefix}：${text}`)
    markInteractionHandled(interaction)
    await options.sendTextToSession(
      options.sessionId.value,
      `${prefix}：${text}`,
      `${prefix}：${text}`,
      { interactionAction: action }
    )
  }

  async function cancelPendingInteraction(target?: WorkspaceInteraction) {
    const interaction = target || pendingInteraction.value
    if (!interaction) return
    await recordPendingInteractionAction(interaction, cancelInteractionAction(interaction))
    markInteractionHandled(interaction)
    await clearCurrentPendingInteractionStatus()
    ElMessage.info(interaction.card_type === 'build_repair' ? translate('miniWorkstation.cancelBuildRepairInfo') : translate('miniWorkstation.cancelConfirmationInfo'))
  }

  async function handleConfirmPrd(payload: { remark: string; prd: unknown }, confirmOptions: { auditRecorded?: boolean } = {}) {
    const remark = payload.remark.trim()
    if (!options.sessionId.value || !options.fullCodePath.value || options.sending.value) {
      ElMessage.warning(translate('miniWorkstation.confirmPrdNotReady'))
      return
    }
    const sourceSessionId = options.sessionId.value
    const interaction = buildWorkspaceInteractionFromArtifact(payload.prd) || currentPrdPendingInteraction()
    let handoff
    try {
      handoff = await createWorkspaceHandoff({
        source_session_id: sourceSessionId,
        full_code_path: options.fullCodePath.value,
        target_role: getPrdTargetRole(payload.prd),
        artifact_kind: 'agent_app_prd',
        artifact: payload.prd,
        remark,
        context_policy: 'full'
      })
    } catch (error: any) {
      ElMessage.error(error?.message || translate('miniWorkstation.confirmPrdFailed'))
      return
    }
    if (interaction && !confirmOptions.auditRecorded) {
      await recordPendingInteractionAction(interaction, 'confirm_prd')
    }
    if (interaction) markInteractionHandled(interaction)
    await clearPendingInteractionStatusForSession(sourceSessionId)
    options.sessionId.value = handoff.session_id
    void options.sendTextToSession(
      handoff.session_id,
      handoff.content,
      handoff.display_content,
      { contextUsage: 'artifact', artifactKind: handoff.artifact_kind, resume: true }
    )
  }

  function markInteractionHandled(interaction: WorkspaceInteraction) {
    const next = new Set(handledInteractionKeys.value)
    next.add(getInteractionKey(interaction))
    handledInteractionKeys.value = next
  }

  function currentPrdPendingInteraction(): WorkspaceInteraction | null {
    const interaction = pendingInteraction.value
    return interaction?.card_type === 'prd_confirmation' ? interaction : null
  }

  async function clearCurrentPendingInteractionStatus() {
    await clearPendingInteractionStatusForSession(options.sessionId.value)
  }

  async function clearPendingInteractionStatusForSession(targetSessionId: string | undefined) {
    if (!targetSessionId) return
    try {
      await resolveWorkspaceSessionInteraction(targetSessionId)
      void options.loadMiniSessions()
      void options.loadGlobalSessions()
    } catch (error: any) {
      ElMessage.warning(error?.message || translate('miniWorkstation.pendingSyncFailed'))
    }
  }

  async function recordPendingInteractionAction(interaction: WorkspaceInteraction, action: string, displayContent?: string) {
    if (!options.sessionId.value) return
    const interactionKey = getInteractionKey(interaction)
    try {
      await recordWorkspaceInteractionEvent({
        session_id: options.sessionId.value,
        action,
        interaction_id: interactionKey,
        card_type: interaction.card_type,
        status: interaction.status,
        artifact_kind: interaction.artifact_kind,
        content: JSON.stringify({
          kind: 'workspace_interaction_event',
          interaction_id: interactionKey,
          action,
          card_type: interaction.card_type,
          status: interaction.status,
          artifact_kind: interaction.artifact_kind,
        }),
        display_content: displayContent || interactionAuditText(interaction, action),
      })
      await options.loadMiniSessionMessages(options.sessionId.value)
    } catch (error: any) {
      ElMessage.warning(error?.message || translate('miniWorkstation.interactionRecordFailed'))
    }
  }

  function focusInteractionArtifact(interaction: WorkspaceInteraction) {
    const root = options.outputRef.value
    if (!root) return
    const selector = interaction.card_type === 'build_repair'
      ? '.mini-msg-build-diagnostics'
      : '.mini-msg-prd-preview'
    const targets = root.querySelectorAll(selector)
    const target = targets[targets.length - 1]
    if (target instanceof HTMLElement) {
      target.scrollIntoView({ behavior: 'smooth', block: 'center' })
      return
    }
    root.scrollTo({ top: root.scrollHeight, behavior: 'smooth' })
  }

  async function handleConfirmBuildHandoff(payload: { artifact: unknown }) {
    if (!options.sessionId.value || !options.fullCodePath.value || options.sending.value) {
      ElMessage.warning(translate('miniWorkstation.repairNotReady'))
      return
    }
    let handoff
    try {
      handoff = await createWorkspaceHandoff({
        source_session_id: options.sessionId.value,
        full_code_path: options.fullCodePath.value,
        target_role: getBuildHandoffTargetRole(payload.artifact),
        artifact_kind: getStageArtifactKind(payload.artifact, 'agent_app_build_failure'),
        artifact: payload.artifact,
        remark: '',
        context_policy: 'full',
        display_content: translate('miniWorkstation.buildRepairDisplayContent')
      })
    } catch (error: any) {
      ElMessage.error(error?.message || translate('miniWorkstation.buildRepairCreateFailed'))
      return
    }
    const interaction = buildWorkspaceInteractionFromArtifact(payload.artifact)
    if (interaction) markInteractionHandled(interaction)
    options.sessionId.value = handoff.session_id
    void options.sendTextToSession(
      handoff.session_id,
      handoff.content,
      handoff.display_content,
      { contextUsage: 'artifact', artifactKind: handoff.artifact_kind, resume: true }
    )
  }

  return {
    pendingInteraction,
    composerBlocked,
    composerBlockedLabel,
    composerBlockedPlaceholder,
    handleBeforeSend,
    handleConfirmPrd,
    viewPendingInteraction,
    revisePendingInteraction,
    cancelPendingInteraction,
    confirmPendingInteraction
  }
}

export function buildWorkspaceInteractionFromArtifact(value: unknown): WorkspaceInteraction | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const artifact = value as StageInteractionArtifact
  const rawInteraction = artifact.interaction
  if (!rawInteraction || typeof rawInteraction !== 'object') return null
  const status = typeof rawInteraction.status === 'string' ? rawInteraction.status.trim() : ''
  if (!status.startsWith('pending_')) return null
  const cardType = typeof rawInteraction.card_type === 'string' ? rawInteraction.card_type : fallbackCardType(artifact.kind, status)
  return {
    id: typeof rawInteraction.id === 'string' ? rawInteraction.id : getStageArtifactKey(artifact),
    card_type: cardType,
    artifact_kind: typeof rawInteraction.artifact_kind === 'string' ? rawInteraction.artifact_kind : artifact.kind,
    status,
    blocking: typeof rawInteraction.blocking === 'boolean' ? rawInteraction.blocking : true,
    title: typeof rawInteraction.title === 'string' ? rawInteraction.title : fallbackInteractionTitle(cardType),
    description: typeof rawInteraction.description === 'string' ? rawInteraction.description : undefined,
    help_text: typeof rawInteraction.help_text === 'string' ? rawInteraction.help_text : undefined,
    view_text: typeof rawInteraction.view_text === 'string' ? rawInteraction.view_text : undefined,
    confirm_text: typeof rawInteraction.confirm_text === 'string' ? rawInteraction.confirm_text : undefined,
    revise_text: typeof rawInteraction.revise_text === 'string' ? rawInteraction.revise_text : undefined,
    cancel_text: typeof rawInteraction.cancel_text === 'string' ? rawInteraction.cancel_text : undefined,
    target_role_on_confirm: typeof rawInteraction.target_role_on_confirm === 'string' ? rawInteraction.target_role_on_confirm : undefined,
    allowed_actions: Array.isArray(rawInteraction.allowed_actions) ? rawInteraction.allowed_actions.map(String) : undefined,
    artifact
  }
}

function getStageArtifactKey(artifact: unknown) {
  try {
    return JSON.stringify(artifact)
  } catch {
    return String(artifact)
  }
}

function getInteractionKey(interaction: WorkspaceInteraction) {
  return interaction.id || getStageArtifactKey(interaction.artifact) || `${interaction.status}:${interaction.card_type}`
}

function isComposerBlockingInteraction(interaction: WorkspaceInteraction) {
  if (interaction.card_type === 'build_repair') return false
  return interaction.blocking
}

function getPrdTargetRole(prd: unknown) {
  const interaction = buildWorkspaceInteractionFromArtifact(prd)
  if (interaction?.target_role_on_confirm) {
    return interaction.target_role_on_confirm
  }
  return 'app_developer'
}

function getBuildHandoffTargetRole(artifact: unknown) {
  const interaction = buildWorkspaceInteractionFromArtifact(artifact)
  if (interaction?.target_role_on_confirm) {
    return interaction.target_role_on_confirm
  }
  return 'build_engineer'
}

function getStageArtifactKind(artifact: unknown, fallback: string) {
  if (artifact && typeof artifact === 'object') {
    const data = artifact as { interaction?: { artifact_kind?: string }, kind?: string }
    return data.interaction?.artifact_kind || data.kind || fallback
  }
  return fallback
}

function fallbackCardType(kind: unknown, status: string) {
  if (kind === 'agent_app_build_failure' || status === 'pending_build_repair') return 'build_repair'
  if (kind === 'agent_app_prd' || status === 'pending_confirmation') return 'prd_confirmation'
  return 'stage_confirmation'
}

function fallbackInteractionTitle(cardType: string) {
  if (cardType === 'build_repair') return translate('miniWorkstation.buildRepairTitle')
  if (cardType === 'prd_confirmation') return translate('miniWorkstation.interactionPrdTitle')
  return translate('miniWorkstation.interactionWaitingTitle')
}

function viewInteractionAction(interaction: WorkspaceInteraction) {
  return interaction.card_type === 'build_repair' ? 'view_build_diagnostics' : 'view_prd'
}

function cancelInteractionAction(interaction: WorkspaceInteraction) {
  return interaction.card_type === 'build_repair' ? 'skip_build_repair' : 'cancel_prd'
}

function interactionAuditText(interaction: WorkspaceInteraction, action: string) {
  const label = interactionActionLabel(action)
  const title = interaction.title || fallbackInteractionTitle(interaction.card_type || '')
  return translate('miniWorkstation.interactionAudit', { label, title })
}

function interactionActionLabel(action: string) {
  const labels: Record<string, string> = {
    view_prd: translate('miniWorkstation.viewPrd'),
    confirm_prd: translate('miniWorkstation.confirmPrd'),
    revise_prd: translate('miniWorkstation.revisePrd'),
    cancel_prd: translate('miniWorkstation.cancelPrd'),
    view_build_diagnostics: translate('miniWorkstation.actionViewBuildDiagnostics'),
    start_build_repair: translate('miniWorkstation.actionStartBuildRepair'),
    continue_development: translate('miniWorkstation.continueDevelopment'),
    skip_build_repair: translate('miniWorkstation.skipRepair'),
  }
  return labels[action] || action
}
