import { computed, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatMessage } from './useWorkspaceChatStream'
import {
  buildWorkspaceInteractionFromArtifact,
  useMiniWorkstationPendingInteraction
} from './useMiniWorkstationPendingInteraction'

const recordWorkspaceInteractionEventMock = vi.hoisted(() => vi.fn(async () => {}))
const resolveWorkspaceSessionInteractionMock = vi.hoisted(() => vi.fn(async () => {}))
const createWorkspaceHandoffMock = vi.hoisted(() => vi.fn(async () => ({
  session_id: 'handoff-session',
  content: 'handoff content',
  display_content: 'handoff display',
  artifact_kind: 'agent_app_prd'
})))

vi.mock('@/architecture/presentation/context/api/workspace', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/architecture/presentation/context/api/workspace')>()
  return {
    ...actual,
    recordWorkspaceInteractionEvent: recordWorkspaceInteractionEventMock,
    resolveWorkspaceSessionInteraction: resolveWorkspaceSessionInteractionMock,
    createWorkspaceHandoff: createWorkspaceHandoffMock
  }
})

function assistantWithArtifact(resultData: unknown): ChatMessage {
  return {
    role: 'assistant',
    content: '',
    tool_calls: [{
      name: 'write_prd',
      status: 'ok',
      result_data: resultData
    }]
  }
}

function createHarness(messagesValue: ChatMessage[]) {
  const messages = ref<ChatMessage[]>(messagesValue)
  const sessionId = ref<string | undefined>('session-1')
  const fullCodePath = ref('/alice/demo/ticket')
  const sending = ref(false)
  const currentSessionDisablesPendingInteraction = ref(false)
  const outputRef = ref<HTMLElement>()
  const loadMiniSessions = vi.fn()
  const loadGlobalSessions = vi.fn()
  const loadMiniSessionMessages = vi.fn(async () => {})
  const sendTextToSession = vi.fn(async () => true)

  const api = useMiniWorkstationPendingInteraction({
    messages: computed(() => messages.value),
    sessionId,
    fullCodePath,
    sending,
    currentSessionDisablesPendingInteraction,
    outputRef,
    loadMiniSessions,
    loadGlobalSessions,
    loadMiniSessionMessages,
    sendTextToSession
  })

  return {
    api,
    messages,
    sessionId,
    fullCodePath,
    sending,
    currentSessionDisablesPendingInteraction,
    loadMiniSessions,
    loadGlobalSessions,
    loadMiniSessionMessages,
    sendTextToSession
  }
}

describe('useMiniWorkstationPendingInteraction', () => {
  beforeEach(() => {
    recordWorkspaceInteractionEventMock.mockClear()
    resolveWorkspaceSessionInteractionMock.mockClear()
    createWorkspaceHandoffMock.mockClear()
  })

  it('builds pending interactions from tool artifacts', () => {
    const interaction = buildWorkspaceInteractionFromArtifact({
      kind: 'agent_app_prd',
      interaction: {
        id: 'prd-1',
        status: 'pending_confirmation',
        blocking: true,
        title: '确认 PRD'
      }
    })

    expect(interaction).toMatchObject({
      id: 'prd-1',
      card_type: 'prd_confirmation',
      artifact_kind: 'agent_app_prd',
      status: 'pending_confirmation',
      blocking: true,
      title: '确认 PRD'
    })
  })

  it('does not restore a composer gate from historical PRD artifacts', async () => {
    const { api } = createHarness([
      assistantWithArtifact({
        kind: 'agent_app_prd',
        interaction: {
          id: 'prd-1',
          status: 'pending_confirmation',
          card_type: 'prd_confirmation',
          blocking: true
        }
      })
    ])

    expect(api.pendingInteraction.value).toBeNull()
    expect(api.composerBlocked.value).toBe(false)

    await expect(api.handleBeforeSend({ text: '继续实现', files: null })).resolves.toBe(false)
    expect(recordWorkspaceInteractionEventMock).not.toHaveBeenCalled()
  })

  it('ignores retired build repair interactions from historical artifacts', async () => {
    const { api } = createHarness([
      assistantWithArtifact({
        kind: 'agent_app_build_failure',
        interaction: {
          id: 'repair-1',
          status: 'pending_build_repair',
          card_type: 'build_repair',
          blocking: true,
          artifact_kind: 'agent_app_build_failure'
        }
      })
    ])

    expect(api.pendingInteraction.value).toBeNull()
    expect(api.composerBlocked.value).toBe(false)
    await expect(api.handleBeforeSend({ text: '我来继续改', files: null })).resolves.toBe(false)
    expect(recordWorkspaceInteractionEventMock).not.toHaveBeenCalled()
  })

  it('unblocks the composer immediately after confirming a PRD', async () => {
    const prdArtifact = {
      kind: 'agent_app_prd',
      project: { name: '线索跟进提醒' },
      interaction: {
        id: 'prd-1',
        status: 'pending_confirmation',
        card_type: 'prd_confirmation',
        blocking: true,
        artifact_kind: 'agent_app_prd'
      }
    }
    const { api, sessionId, sendTextToSession } = createHarness([
      assistantWithArtifact(prdArtifact)
    ])

    expect(api.pendingInteraction.value).toBeNull()
    expect(api.composerBlocked.value).toBe(false)

    await api.handleConfirmPrd({ remark: '', prd: prdArtifact })

    expect(recordWorkspaceInteractionEventMock).toHaveBeenCalledWith(expect.objectContaining({
      session_id: 'session-1',
      action: 'confirm_prd',
      interaction_id: 'prd-1',
      card_type: 'prd_confirmation',
      artifact_kind: 'agent_app_prd'
    }))
    expect(resolveWorkspaceSessionInteractionMock).toHaveBeenCalledWith('session-1')
    expect(sessionId.value).toBe('handoff-session')
    expect(sendTextToSession).toHaveBeenCalledWith(
      'handoff-session',
      'handoff content',
      'handoff display',
      { contextUsage: 'artifact', artifactKind: 'agent_app_prd', resume: true }
    )
    expect(api.pendingInteraction.value).toBeNull()
    expect(api.composerBlocked.value).toBe(false)
  })

  it('keeps the composer unblocked after reopening a confirmed PRD conversation', () => {
    const prdArtifact = {
      kind: 'agent_app_prd',
      project: { name: '线索跟进提醒' },
      interaction: {
        status: 'pending_confirmation',
        card_type: 'prd_confirmation',
        blocking: true,
        artifact_kind: 'agent_app_prd'
      }
    }
    const { api } = createHarness([
      assistantWithArtifact(prdArtifact),
      {
        role: 'user',
        content: '已确认 PRD，进入开发阶段',
        raw_content: '阶段交接上下文',
        context_usage: 'artifact',
        artifact_kind: 'agent_app_prd'
      }
    ])

    expect(api.pendingInteraction.value).toBeNull()
    expect(api.composerBlocked.value).toBe(false)
  })
})
