import { computed, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import {
  normalizeFullCodePath,
  useMiniWorkstationSessionView,
  type SessionFilterValue
} from './useMiniWorkstationSessionView'
import type { WorkspaceSessionItem } from '@/architecture/presentation/context/api/workspace'

function createSession(overrides: Partial<WorkspaceSessionItem>): WorkspaceSessionItem {
  return {
    session_id: 's1',
    title: '默认会话',
    status: 'active',
    full_code_path: '/luobei/app/demo',
    created_at: '2026-05-01T00:00:00Z',
    updated_at: '2026-05-01T00:00:00Z',
    ...overrides
  }
}

function createHarness(options: {
  mini?: WorkspaceSessionItem[]
  global?: WorkspaceSessionItem[]
  sessionId?: string
  fullCodePath?: string
  dirName?: string
  pathNameMap?: Record<string, string>
  hasCurrentGeneratedArtifacts?: boolean
  keyword?: string
  filter?: SessionFilterValue
} = {}) {
  const miniSessionList = ref(options.mini || [])
  const globalSessionList = ref(options.global || [])
  const sessionId = ref<string | undefined>(options.sessionId)
  const sending = ref(false)
  const fullCodePath = ref(options.fullCodePath || '/luobei/app/demo')
  const pathNameMap = ref(options.pathNameMap)
  const firstUserMessagePreview = ref('创建应用')
  const hasCurrentGeneratedArtifacts = ref(!!options.hasCurrentGeneratedArtifacts)
  const sessionSearchKeyword = ref(options.keyword || '')
  const sessionFilter = ref<SessionFilterValue>(options.filter || 'all')

  const api = useMiniWorkstationSessionView({
    miniSessionList: computed(() => miniSessionList.value),
    globalSessionList: computed(() => globalSessionList.value),
    sessionId,
    sending,
    fullCodePath,
    dirName: () => options.dirName,
    pathNameMap: () => pathNameMap.value,
    firstUserMessagePreview,
    hasCurrentGeneratedArtifacts,
    sessionSearchKeyword,
    sessionFilter
  })

  return {
    api,
    miniSessionList,
    globalSessionList,
    sessionId,
    hasCurrentGeneratedArtifacts,
    sessionSearchKeyword,
    sessionFilter
  }
}

describe('useMiniWorkstationSessionView', () => {
  it('normalizes workspace paths', () => {
    expect(normalizeFullCodePath('/a/b///')).toBe('/a/b')
    expect(normalizeFullCodePath('  /a/b  ')).toBe('/a/b')
  })

  it('uses mapped path names before falling back to path segments', () => {
    const { api } = createHarness({
      fullCodePath: '/luobei/app/support/tickets',
      pathNameMap: {
        '/luobei/app/support/tickets': '工单中心'
      }
    })

    expect(api.displayPath.value).toBe('工单中心')
    expect(api.getSessionDirectoryPath(createSession({
      full_code_path: '/luobei/app/sales/customers'
    }))).toBe('sales / customers')
  })

  it('classifies session status for running, waiting, output and done buckets', () => {
    const { api, hasCurrentGeneratedArtifacts } = createHarness({ sessionId: 's-current' })

    expect(api.getSessionStatusKind(createSession({ status: 'generating' }))).toBe('running')
    expect(api.getSessionStatusKind(createSession({ status: 'pending_confirmation' }))).toBe('output')
    expect(api.getSessionStatusKind(createSession({ status: 'pending_test' }))).toBe('output')
    expect(api.getSessionStatusLabel(createSession({ status: 'pending_test' }))).toBe('待自动测试')
    expect(api.getSessionStatusKind(createSession({ status: 'pending_build_repair' }))).toBe('output')
    expect(api.getSessionStatusLabel(createSession({ status: 'pending_build_repair' }))).toBe('新文件')
    expect(api.getSessionStatusKind(createSession({ status: 'finished' }))).toBe('done')

    hasCurrentGeneratedArtifacts.value = true
    expect(api.getSessionStatusKind(createSession({
      session_id: 's-current',
      status: 'active'
    }))).toBe('output')
  })

  it('keeps current active session in the summary list', () => {
    const { api } = createHarness({
      sessionId: 'active',
      mini: [
        createSession({ session_id: 'a', updated_at: '2026-05-02T00:00:00Z' }),
        createSession({ session_id: 'b', updated_at: '2026-05-03T00:00:00Z' }),
        createSession({ session_id: 'c', updated_at: '2026-05-04T00:00:00Z' }),
        createSession({ session_id: 'd', updated_at: '2026-05-05T00:00:00Z' })
      ]
    })

    expect(api.summarySessions.value.map(item => item.session_id)).toContain('active')
  })

  it('filters session center lists by keyword and status', () => {
    const { api, sessionSearchKeyword, sessionFilter } = createHarness({
      keyword: '客户',
      filter: 'running',
      mini: [
        createSession({ session_id: 'running-customer', title: '客户跟进', status: 'running' }),
        createSession({ session_id: 'done-customer', title: '客户归档', status: 'done' }),
        createSession({ session_id: 'running-ticket', title: '工单管理', status: 'running' })
      ]
    })

    expect(api.currentDirectorySessionList.value.map(item => item.session_id)).toEqual(['running-customer'])

    sessionFilter.value = 'done'
    expect(api.currentDirectorySessionList.value.map(item => item.session_id)).toEqual(['done-customer'])

    sessionSearchKeyword.value = '工单'
    sessionFilter.value = 'running'
    expect(api.currentDirectorySessionList.value.map(item => item.session_id)).toEqual(['running-ticket'])
  })

  it('keeps list order stable when the active session changes and treats failed sessions as ended', () => {
    const { api, sessionId } = createHarness({
      sessionId: 'active',
      filter: 'all',
      mini: [
        createSession({ session_id: 'newer', updated_at: '2026-05-03T00:00:00Z' }),
        createSession({ session_id: 'active', updated_at: '2026-05-01T00:00:00Z' }),
      ]
    })

    expect(api.currentDirectorySessionList.value.map(item => item.session_id)).toEqual(['newer', 'active'])
    sessionId.value = 'newer'
    expect(api.currentDirectorySessionList.value.map(item => item.session_id)).toEqual(['newer', 'active'])
    sessionId.value = 'active'
    expect(api.currentDirectorySessionList.value.map(item => item.session_id)).toEqual(['newer', 'active'])
    expect(api.matchesSessionFilter(createSession({ status: 'failed' }), 'done')).toBe(true)
    expect(api.matchesSessionFilter(createSession({ status: 'cancelled' }), 'done')).toBe(true)
  })
})
