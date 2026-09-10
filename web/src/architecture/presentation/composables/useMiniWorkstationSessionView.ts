import { computed, type Ref } from 'vue'
import type { WorkspaceSessionItem } from '@/architecture/presentation/context/api/workspace'
import { translate } from '@/architecture/shared/i18n'
import { formatWorkspaceRoleName } from '@/architecture/presentation/utils/workspaceRoleDisplay'

export type SessionFilterValue = 'all' | 'running' | 'waiting' | 'output' | 'done'
export type SessionStatusKind = 'running' | 'waiting' | 'done' | 'cancelled' | 'failed' | 'active' | 'output'

export const miniWorkstationSessionFilters: Array<{ label: string; value: SessionFilterValue }> = [
  { label: translate('miniWorkstation.filterAll'), value: 'all' },
  { label: translate('miniWorkstation.filterRunning'), value: 'running' },
  { label: translate('miniWorkstation.filterWaiting'), value: 'waiting' },
  { label: translate('miniWorkstation.filterOutput'), value: 'output' },
  { label: translate('miniWorkstation.filterDone'), value: 'done' }
]

export function getMiniWorkstationSessionFilters(): Array<{ label: string; value: SessionFilterValue }> {
  return [
    { label: translate('miniWorkstation.filterAll'), value: 'all' },
    { label: translate('miniWorkstation.filterRunning'), value: 'running' },
    { label: translate('miniWorkstation.filterWaiting'), value: 'waiting' },
    { label: translate('miniWorkstation.filterOutput'), value: 'output' },
    { label: translate('miniWorkstation.filterDone'), value: 'done' }
  ]
}

interface UseMiniWorkstationSessionViewOptions {
  miniSessionList: Ref<WorkspaceSessionItem[]>
  globalSessionList: Ref<WorkspaceSessionItem[]>
  sessionId: Ref<string | undefined>
  sending: Ref<boolean>
  fullCodePath: Ref<string>
  dirName: () => string | undefined
  pathNameMap: () => Record<string, string> | undefined
  firstUserMessagePreview: Ref<string>
  hasCurrentGeneratedArtifacts: Ref<boolean>
  sessionSearchKeyword: Ref<string>
  sessionFilter: Ref<SessionFilterValue>
}

export function useMiniWorkstationSessionView(options: UseMiniWorkstationSessionViewOptions) {
  const displayPath = computed(() => {
    if (!options.fullCodePath.value) return translate('miniWorkstation.noDirectorySelected')
    return resolvePathDisplayName(options.fullCodePath.value)
  })

  const currentSessionItem = computed(() => {
    if (!options.sessionId.value) return null
    return options.miniSessionList.value.find(item => item.session_id === options.sessionId.value)
      || options.globalSessionList.value.find(item => item.session_id === options.sessionId.value)
      || null
  })

  const currentFallbackSession = computed<WorkspaceSessionItem | null>(() => {
    if (!options.sessionId.value || currentSessionItem.value) return null
    const now = new Date().toISOString()
    return {
      session_id: options.sessionId.value,
      title: options.firstUserMessagePreview.value || options.dirName() || displayPath.value || translate('miniWorkstation.newSession'),
      status: options.sending.value ? 'generating' : 'active',
      full_code_path: options.fullCodePath.value,
      directory_name: options.dirName() || displayPath.value,
      role_display_name: options.dirName() || displayPath.value,
      created_at: now,
      updated_at: now
    }
  })

  const activeSummarySession = computed(() => currentSessionItem.value || currentFallbackSession.value)

  const currentOutputSessionList = computed(() => {
    const currentPath = normalizeFullCodePath(options.fullCodePath.value)
    const active = activeSummarySession.value
    const seenIds = new Set<string>()
    const list: WorkspaceSessionItem[] = []

    const addIfCurrentPath = (session: WorkspaceSessionItem | null | undefined) => {
      if (!session?.session_id || seenIds.has(session.session_id)) return
      const sessionPath = normalizeFullCodePath(session.full_code_path || options.fullCodePath.value || '')
      if (currentPath && sessionPath && sessionPath !== currentPath) return
      seenIds.add(session.session_id)
      list.push(session)
    }

    for (const session of options.miniSessionList.value) {
      addIfCurrentPath(session)
    }

    if (active?.session_id && !seenIds.has(active.session_id)) {
      addIfCurrentPath(active)
    }

    return list
  })

  const recentSessionSourceList = computed(() => {
    const byId = new Map<string, WorkspaceSessionItem>()
    for (const session of [...options.globalSessionList.value, ...options.miniSessionList.value]) {
      if (!session.session_id) continue
      byId.set(session.session_id, session)
    }
    if (currentFallbackSession.value) {
      byId.set(currentFallbackSession.value.session_id, currentFallbackSession.value)
    }
    return Array.from(byId.values())
      .sort((left, right) => getSessionTimestamp(right) - getSessionTimestamp(left))
  })

  const summarySessions = computed(() => {
    const active = activeSummarySession.value
    const list = [...recentSessionSourceList.value]
    if (active && !list.some(item => item.session_id === active.session_id)) {
      list.unshift(active)
    }
    const visible = list.slice(0, 4)
    if (!active || visible.some(item => item.session_id === active.session_id)) {
      return visible
    }
    return [...visible.slice(0, 3), active]
  })

  const recentSessionCenterSourceList = computed(() => {
    return options.globalSessionList.value.length > 0 ? options.globalSessionList.value : recentSessionSourceList.value
  })

  const currentDirectorySessionList = computed(() => filterSessionCenterList(options.miniSessionList.value))
  const recentSessionCenterList = computed(() => filterSessionCenterList(recentSessionCenterSourceList.value))

  function filterSessionCenterList(list: WorkspaceSessionItem[]) {
    const keyword = options.sessionSearchKeyword.value.trim().toLowerCase()
    return list
      .filter((session) => {
        if (!matchesSessionFilter(session, options.sessionFilter.value)) return false
        return matchesSessionKeyword(session, keyword)
      })
      .sort((left, right) => getSessionTimestamp(right) - getSessionTimestamp(left))
  }

  function matchesSessionKeyword(session: WorkspaceSessionItem, keyword: string) {
    if (!keyword) return true
    return [
      session.title,
      session.user,
      getSessionRoleDisplayName(session),
      session.role_display_name,
      session.role_id,
      session.directory_name,
      session.full_code_path,
      getSessionDirectoryPath(session)
    ].some(value => (value || '').toLowerCase().includes(keyword))
  }

  function getMappedPathName(fullCodePath: string) {
    const normalizedPath = normalizeFullCodePath(fullCodePath)
    if (!normalizedPath) return ''
    const pathNameMap = options.pathNameMap()
    return pathNameMap?.[normalizedPath]
      || pathNameMap?.[normalizedPath.replace(/^\/+/, '')]
      || ''
  }

  function resolvePathDisplayName(fullCodePath: string) {
    const normalizedPath = normalizeFullCodePath(fullCodePath)
    if (!normalizedPath) return ''
    const mappedName = getMappedPathName(normalizedPath)
    if (mappedName) return mappedName
    if (normalizedPath === normalizeFullCodePath(options.fullCodePath.value) && options.dirName()) {
      return options.dirName() || ''
    }
    const parts = normalizedPath.split('/').filter(Boolean).map(decodePathSegment)
    if (parts.length >= 2) {
      return parts.slice(-2).join(' / ')
    }
    return parts[0] || normalizedPath
  }

  function getSessionDirectoryPath(session: WorkspaceSessionItem) {
    const explicitDirectoryName = (session.directory_name || '').trim()
    if (explicitDirectoryName) {
      return explicitDirectoryName
    }

    const path = normalizeFullCodePath(session.full_code_path || options.fullCodePath.value || '')
    if (!path) {
      return options.dirName() || displayPath.value || translate('miniWorkstation.currentDirectory')
    }

    const mappedName = getMappedPathName(path)
    if (mappedName) {
      return mappedName
    }

    if (path === normalizeFullCodePath(options.fullCodePath.value) && options.dirName()) {
      return options.dirName() || ''
    }

    return resolvePathDisplayName(path) || options.dirName() || displayPath.value || translate('miniWorkstation.currentDirectory')
  }

  function getSessionSubtitle(session: WorkspaceSessionItem) {
    return [getSessionDirectoryPath(session), getSessionStatusLabel(session)].filter(Boolean).join(' · ')
  }

  function getSessionCenterSubtitle(session: WorkspaceSessionItem) {
    return [getSessionDirectoryPath(session), getSessionRoleDisplayName(session) || session.user || getSessionStatusLabel(session)]
      .filter(Boolean)
      .join(' · ') || translate('miniWorkstation.currentDirectory')
  }

  function getSessionRoleDisplayName(session: WorkspaceSessionItem) {
    return formatWorkspaceRoleName(session.role_id, session.role_display_name)
  }

  function getSessionStatusKind(session: WorkspaceSessionItem): SessionStatusKind {
    const status = getSessionRawStatus(session)
    if ([
      'generating',
      'running',
      'tool_running',
      'thinking',
      'streaming',
      'processing',
      'executing'
    ].includes(status)) return 'running'

    if ([
      'waiting',
      'pending',
      'waiting_approval',
      'paused',
      'queued'
    ].includes(status)) return 'waiting'

    if (['cancelled', 'canceled', 'abort', 'aborted'].includes(status)) return 'cancelled'
    if (['failed', 'failure', 'error', 'timeout'].includes(status)) return 'failed'
    if (['output', 'new_file', 'new_output', 'has_output', 'artifact', 'artifact_ready', 'pending_confirmation', 'pending_test', 'pending_build_repair'].includes(status)) return 'output'
    if (sessionHasGeneratedArtifacts(session)) return 'output'
    if (session.handoff_kind) return 'output'
    if (['done', 'completed', 'complete', 'success', 'succeeded', 'finished'].includes(status)) return 'done'
    if (status === 'active' || !status) return 'active'
    return 'active'
  }

  function getSessionStatusLabel(session: WorkspaceSessionItem) {
    const status = getSessionRawStatus(session)
    if (status === 'pending_test') return translate('miniWorkstation.statusAutoTestPending')
    const labels: Record<SessionStatusKind, string> = {
      running: translate('miniWorkstation.statusRunning'),
      waiting: translate('miniWorkstation.statusWaiting'),
      done: translate('miniWorkstation.statusDone'),
      cancelled: translate('miniWorkstation.statusCancelled'),
      failed: translate('miniWorkstation.statusFailed'),
      active: translate('miniWorkstation.statusSession'),
      output: translate('miniWorkstation.statusNewFile')
    }
    return labels[getSessionStatusKind(session)]
  }

  function getSessionStatusClass(session: WorkspaceSessionItem) {
    return `is-${getSessionStatusKind(session)}`
  }

  function matchesSessionFilter(session: WorkspaceSessionItem, filter: SessionFilterValue) {
    const kind = getSessionStatusKind(session)
    if (filter === 'all') return true
    if (filter === 'running') return kind === 'running'
    if (filter === 'waiting') return kind === 'waiting'
    if (filter === 'output') return kind === 'output'
    if (filter === 'done') return kind === 'done' || kind === 'cancelled' || kind === 'failed'
    return true
  }

  function sessionHasGeneratedArtifacts(session: WorkspaceSessionItem) {
    return !!session.session_id
      && session.session_id === options.sessionId.value
      && options.hasCurrentGeneratedArtifacts.value
  }

  return {
    displayPath,
    currentSessionItem,
    currentFallbackSession,
    activeSummarySession,
    currentOutputSessionList,
    recentSessionSourceList,
    summarySessions,
    recentSessionCenterSourceList,
    currentDirectorySessionList,
    recentSessionCenterList,
    getSessionTitle,
    getSessionDirectoryPath,
    getSessionSubtitle,
    getSessionCenterSubtitle,
    getSessionTimestamp,
    getSessionStatusLabel,
    getSessionStatusKind,
    getSessionStatusClass,
    matchesSessionFilter,
    normalizeFullCodePath,
    resolvePathDisplayName
  }
}

export function getSessionTitle(session: WorkspaceSessionItem) {
  return session.title || formatWorkspaceRoleName(session.role_id, session.role_display_name) || translate('miniWorkstation.unnamedSession')
}

export function getSessionTimestamp(session: WorkspaceSessionItem) {
  const time = new Date(session.updated_at || session.created_at).getTime()
  return Number.isFinite(time) ? time : 0
}

export function normalizeFullCodePath(fullCodePath: string) {
  return (fullCodePath || '').trim().replace(/\/+$/g, '')
}

function getSessionRawStatus(session: WorkspaceSessionItem) {
  return String(session.status || '').trim().toLowerCase()
}

function decodePathSegment(segment: string) {
  try {
    return decodeURIComponent(segment)
  } catch {
    return segment
  }
}
