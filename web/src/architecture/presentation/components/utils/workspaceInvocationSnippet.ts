import { escapeHtml } from '@/architecture/shared/sanitizeHtml'
import { resolveWorkspaceUrl } from '@/architecture/shared/routing/route'

export type WorkspaceInvocationTool =
  | 'run_form_submit'
  | 'run_table_create'
  | 'run_table_update'
  | 'run_table_delete'
  | string

export interface WorkspaceInvocationParam {
  key: string
  value: unknown
  fixed?: boolean
}

export interface WorkspaceInvocationSnippetInput {
  tool: WorkspaceInvocationTool
  resourcePath: string
  params?: Record<string, unknown> | WorkspaceInvocationParam[]
  note?: string
}

export interface WorkspacePromptSegment {
  type: 'text' | 'resource'
  text: string
  path?: string
  start: number
  end: number
}

export interface WorkspaceInvocationBlock {
  tool: string
  resourcePath: string
  params: WorkspaceInvocationParam[]
  startLine: number
  endLine: number
}

const TOOL_RESOURCE_PREFIX = 'tool:'
const RESOURCE_TOKEN_PATTERN = /<(?:\/|\.\/|\.\.\/|tool:)[^>\s]+>/g
const RESOURCE_TOKEN_BODY_PATTERN = /^(?:\/|\.\/|\.\.\/|tool:)[^>\s]+$/
const DEFAULT_INVOCATION_NOTE = '复制后粘贴到工作台，AI 会按下面信息识别并调用。'

interface TextRange {
  start: number
  end: number
}

export function wrapWorkspaceResourcePath(path: string): string {
  const normalized = normalizeWorkspaceResourcePath(path)
  return normalized ? `<${normalized}>` : ''
}

export function normalizeWorkspaceResourcePath(path: string): string {
  const body = stripWorkspaceResourceWrapper(path)
  if (!body) return ''
  if (isWorkspaceToolResourcePath(body)) {
    return body
  }
  if (isRelativeWorkspaceResourcePath(body)) {
    return body
  }
  return body.startsWith('/') ? body : `/${body}`
}

export function resolveWorkspaceResourcePath(path: string, basePath = ''): string {
  const normalized = normalizeWorkspaceResourcePath(path)
  if (!normalized || !isRelativeWorkspaceResourcePath(normalized)) {
    return normalized
  }

  return joinWorkspacePath(basePath, normalized) || normalized
}

export interface WorkspaceResourceInsertion {
  value: string
  insertedText: string
  cursor: number
}

export function insertWorkspaceResourceTokensAtOffset(
  text: string,
  paths: string[],
  offset: number,
  basePath = ''
): WorkspaceResourceInsertion {
  const source = String(text || '')
  const cursor = Math.max(0, Math.min(offset, source.length))
  const existingPaths = new Set(
    parseWorkspacePromptSegments(source, basePath)
      .filter(segment => segment.type === 'resource')
      .map(segment => normalizeWorkspaceResourcePath(segment.path || ''))
      .filter(Boolean)
  )
  const tokens: string[] = []

  paths.forEach((path) => {
    const normalized = normalizeWorkspaceResourcePath(path)
    const resolved = normalizeWorkspaceResourcePath(resolveWorkspaceResourcePath(normalized, basePath))
    if (!normalized || !resolved || existingPaths.has(resolved)) return
    existingPaths.add(resolved)
    tokens.push(wrapWorkspaceResourcePath(normalized))
  })

  if (tokens.length === 0) {
    return { value: source, insertedText: '', cursor }
  }

  const prefix = cursor > 0 && !/\s/.test(source[cursor - 1] || '') ? ' ' : ''
  const suffix = cursor < source.length && !/\s/.test(source[cursor] || '') ? ' ' : ''
  const insertedText = `${prefix}${tokens.join(' ')}${suffix}`
  return {
    value: `${source.slice(0, cursor)}${insertedText}${source.slice(cursor)}`,
    insertedText,
    cursor: cursor + insertedText.length,
  }
}

export function unwrapWorkspaceResourceToken(token: string, basePath = ''): string {
  const trimmed = String(token || '').trim()
  if (!trimmed.startsWith('<') || !trimmed.endsWith('>')) return ''
  const body = trimmed.slice(1, -1).trim()
  if (!RESOURCE_TOKEN_BODY_PATTERN.test(body)) return ''
  return resolveWorkspaceResourcePath(body, basePath)
}

export function parseWorkspacePromptSegments(text: string, basePath = ''): WorkspacePromptSegment[] {
  const source = String(text || '')
  const segments: WorkspacePromptSegment[] = []
  const protectedRanges = getMarkdownCodeRanges(source)
  let cursor = 0
  let match: RegExpExecArray | null

  RESOURCE_TOKEN_PATTERN.lastIndex = 0
  while ((match = RESOURCE_TOKEN_PATTERN.exec(source)) !== null) {
    const raw = match[0]
    const start = match.index
    const end = start + raw.length
    if (isEscapedResourceTokenStart(source, start) || isIndexInRanges(start, protectedRanges)) {
      continue
    }
    if (start > cursor) {
      segments.push({
        type: 'text',
        text: source.slice(cursor, start),
        start: cursor,
        end: start,
      })
    }
    segments.push({
      type: 'resource',
      text: raw,
      path: unwrapWorkspaceResourceToken(raw, basePath),
      start,
      end,
    })
    cursor = end
  }

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

export function renderWorkspaceResourceTokensAsHtml(
  text: string,
  basePath = '',
  resourceLabels: Record<string, string> = {}
): string {
  const source = String(text || '')
  const segments = parseWorkspacePromptSegments(source, basePath)
  if (!segments.some((segment) => segment.type === 'resource')) {
    return source
  }

  return segments.map((segment) => {
    if (segment.type !== 'resource') {
      return segment.text
    }
    return renderWorkspaceResourceToken(segment, resourceLabels)
  }).join('')
}

export function workspaceResourceKind(pathOrToken: string): string {
  const path = normalizeWorkspaceResourcePath(pathOrToken)
  if (isWorkspaceToolResourcePath(path)) return 'tool'
  const { pathPart } = splitWorkspacePathQuery(path)
  if (pathPart.endsWith('.table')) return 'table'
  if (pathPart.endsWith('.form')) return 'form'
  if (pathPart.endsWith('.chart')) return 'chart'
  if (pathPart.endsWith('.docs')) return 'docs'
  return 'directory'
}

function renderWorkspaceResourceToken(segment: WorkspacePromptSegment, resourceLabels: Record<string, string>): string {
  const path = normalizeWorkspaceResourcePath(segment.path || segment.text)
  if (!path) {
    return escapeHtml(segment.text)
  }
  const kind = workspaceResourceKind(path)
  const href = kind === 'tool' ? `#${path}` : resolveWorkspaceUrl(path)
  const label = getMappedWorkspaceResourceLabel(path, resourceLabels) || workspaceResourceLabel(path)
  const typeLabel = workspaceResourceTypeLabel(kind)
  const iconSrc = workspaceResourceIconSrc(kind)
  const iconHtml = workspaceResourceIconHtml(kind, typeLabel)
  return [
    `<a class="workspace-resource-token is-${escapeHtml(kind)}"`,
    ` href="${escapeHtml(href)}"`,
    ` data-full-code-path="${escapeHtml(path)}"`,
    ` data-resource-kind="${escapeHtml(kind)}"`,
    ` data-resource-label="${escapeHtml(label)}"`,
    ` data-resource-type-label="${escapeHtml(typeLabel)}"`,
    ` data-resource-icon-src="${escapeHtml(iconSrc)}"`,
    ` title="${escapeHtml(path)}">`,
    `<span class="workspace-resource-token__icon">`,
    iconHtml,
    `</span>`,
    `<span class="workspace-resource-token__label">${escapeHtml(label)}</span>`,
    `</a>`,
  ].join('')
}

function getMappedWorkspaceResourceLabel(path: string, resourceLabels: Record<string, string>): string {
  const normalized = normalizeWorkspaceResourcePath(path)
  return String(
    resourceLabels[normalized]
      || resourceLabels[normalized.replace(/^\/+/, '')]
      || ''
  ).trim()
}

function workspaceResourceLabel(pathOrToken: string): string {
  const path = normalizeWorkspaceResourcePath(pathOrToken)
  if (isWorkspaceToolResourcePath(path)) {
    return workspaceToolName(path) || path
  }
  const { pathPart } = splitWorkspacePathQuery(path)
  const tail = pathPart.split('/').filter(Boolean).pop()
  return tail || path || pathOrToken
}

function workspaceResourceTypeLabel(kind: string): string {
  switch (kind) {
    case 'table':
      return '表格'
    case 'form':
      return '表单'
    case 'chart':
      return '图表'
    case 'docs':
      return '文档'
    case 'tool':
      return '内置工具'
    default:
      return '服务目录'
  }
}

export function workspaceResourceIconSrc(kind: string): string {
  switch (kind) {
    case 'directory':
      return '/service-tree/custom-folder.svg'
    case 'form':
      return '/service-tree/编辑.svg'
    case 'docs':
      return '/文档.svg'
    default:
      return ''
  }
}

export function workspaceResourceIconHtml(kind: string, typeLabel = ''): string {
  const src = workspaceResourceIconSrc(kind)
  if (src) {
    return `<img class="workspace-resource-token__img" src="${escapeHtml(src)}" alt="${escapeHtml(typeLabel)}" />`
  }
  if (kind === 'table') {
    return [
      `<svg class="workspace-resource-token__svg table-icon" viewBox="0 0 1024 1024" aria-hidden="true">`,
      `<path d="M0 0m36.608 0l950.784 0q36.608 0 36.608 36.608l0 219.392q0 36.608-36.608 36.608l-950.784 0q-36.608 0-36.608-36.608l0-219.392q0-36.608 36.608-36.608Z" fill="#553CCE"></path>`,
      `<path d="M0 365.738667m36.608 0l219.392 0q36.608 0 36.608 36.608l0 219.392q0 36.608-36.608 36.608l-219.392 0q-36.608 0-36.608-36.608l0-219.392q0-36.608 36.608-36.608Z" fill="#553CCE"></path>`,
      `<path d="M365.738667 365.738667m36.608 0l219.392 0q36.608 0 36.608 36.608l0 219.392q0 36.608-36.608 36.608l-219.392 0q-36.608 0-36.608-36.608l0-219.392q0-36.608 36.608-36.608Z" fill="#553CCE"></path>`,
      `<path d="M731.392 365.738667m36.608 0l219.392 0q36.608 0 36.608 36.608l0 219.392q0 36.608-36.608 36.608l-219.392 0q-36.608 0-36.608-36.608l0-219.392q0-36.608 36.608-36.608Z" fill="#553CCE"></path>`,
      `<path d="M0 731.392m36.608 0l219.392 0q36.608 0 36.608 36.608l0 219.392q0 36.608-36.608 36.608l-219.392 0q-36.608 0-36.608-36.608l0-219.392q0-36.608 36.608-36.608Z" fill="#553CCE"></path>`,
      `<path d="M365.738667 731.392m36.608 0l219.392 0q36.608 0 36.608 36.608l0 219.392q0 36.608-36.608 36.608l-219.392 0q-36.608 0-36.608-36.608l0-219.392q0-36.608 36.608-36.608Z" fill="#553CCE"></path>`,
      `<path d="M731.392 731.392m36.608 0l219.392 0q36.608 0 36.608 36.608l0 219.392q0 36.608-36.608 36.608l-219.392 0q-36.608 0-36.608-36.608l0-219.392q0-36.608 36.608-36.608Z" fill="#553CCE"></path>`,
      `</svg>`,
    ].join('')
  }
  if (kind === 'chart') {
    return [
      `<svg class="workspace-resource-token__svg chart-icon" viewBox="0 0 1024 1024" aria-hidden="true">`,
      `<path d="M976 944H48c-26.496 0-48-21.44-48-48V128c0-26.496 21.504-48 48-48s48 21.504 48 48v720h880c26.496 0 48 21.504 48 48 0 26.56-21.504 48-48 48zM864 800h-96c-26.496 0-48-21.504-48-48V416c0-26.496 21.504-48 48-48h96c26.496 0 48 21.504 48 48v336c0 26.56-21.504 48-48 48z m-272 0h-96c-26.496 0-48-21.44-48-48V224c0-26.496 21.504-48 48-48h96c26.496 0 48 21.504 48 48v528c0 26.56-21.504 48-48 48z m-272 0h-96c-26.496 0-48-21.504-48-48v-96c0-26.496 21.504-48 48-48h96c26.496 0 48 21.504 48 48v96c0 26.56-21.504 48-48 48z" fill="#9377E0"></path>`,
      `</svg>`,
    ].join('')
  }
  if (kind === 'tool') {
    return `<span class="workspace-resource-token__glyph workspace-resource-token__glyph--tool" aria-hidden="true"></span>`
  }
  return [
    `<svg class="workspace-resource-token__svg function-icon" viewBox="0 0 1024 1024" aria-hidden="true">`,
    `<path d="M832 128H192c-35.35 0-64 28.65-64 64v640c0 35.35 28.65 64 64 64h640c35.35 0 64-28.65 64-64V192c0-35.35-28.65-64-64-64zM288 320h448v64H288v-64zm0 160h448v64H288v-64zm0 160h320v64H288v-64z" fill="#6366f1"></path>`,
    `</svg>`,
  ].join('')
}

export function buildWorkspaceInvocationSnippet(input: WorkspaceInvocationSnippetInput): string {
  const resourceToken = wrapWorkspaceResourcePath(input.resourcePath)
  const lines = [
    '函数调用：',
    `用途：${input.note || DEFAULT_INVOCATION_NOTE}`,
    `工具：${input.tool}`,
    `函数：${resourceToken}`,
  ]
  const params = normalizeInvocationParams(input.params)

  if (params.length > 0) {
    lines.push('', '参数：')
    params.forEach((param) => {
      const fixedSuffix = param.fixed ? '（固定）' : ''
      lines.push(`${param.key} = ${formatInvocationValue(param.value)}${fixedSuffix}`)
    })
  }

  return lines.join('\n')
}

export function normalizeInvocationParams(
  params?: Record<string, unknown> | WorkspaceInvocationParam[]
): WorkspaceInvocationParam[] {
  if (!params) return []
  if (Array.isArray(params)) {
    return params
      .map((param) => ({
        key: String(param.key || '').trim(),
        value: param.value,
        fixed: !!param.fixed,
      }))
      .filter((param) => param.key)
  }

  return Object.entries(params)
    .filter(([key]) => key.trim())
    .map(([key, value]) => ({ key, value }))
}

export function filterEmptyInvocationParams(params: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(params || {}).filter(([, value]) => !isEmptyInvocationValue(value))
  )
}

export function parseWorkspaceInvocationBlocks(text: string, basePath = ''): WorkspaceInvocationBlock[] {
  const lines = String(text || '').split(/\r?\n/)
  const blocks: WorkspaceInvocationBlock[] = []

  for (let index = 0; index < lines.length; index += 1) {
    if (lines[index]?.trim() !== '函数调用：') {
      continue
    }

    let endLine = lines.length - 1
    for (let next = index + 1; next < lines.length; next += 1) {
      if (lines[next]?.trim() === '函数调用：') {
        endLine = next - 1
        break
      }
    }

    const blockLines = lines.slice(index, endLine + 1)
    const tool = readBlockValue(blockLines, '工具')
    const functionLine = readBlockValue(blockLines, '函数')
    const resourcePath = unwrapWorkspaceResourceToken(functionLine, basePath)
    const params = readBlockParams(blockLines)

    if (tool || resourcePath || params.length > 0) {
      blocks.push({
        tool,
        resourcePath,
        params,
        startLine: index,
        endLine,
      })
    }
  }

  return blocks
}

export async function copyTextToClipboard(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text)
    return
  }

  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', 'true')
  textarea.style.position = 'fixed'
  textarea.style.left = '-9999px'
  document.body.appendChild(textarea)
  textarea.select()
  const ok = document.execCommand('copy')
  document.body.removeChild(textarea)
  if (!ok) {
    throw new Error('copy failed')
  }
}

export function getWorkspaceResourceSelectionText(root: HTMLElement | null): string {
  if (!root || typeof window === 'undefined' || typeof document === 'undefined') {
    return ''
  }
  const selection = window.getSelection?.()
  if (!selection || selection.rangeCount === 0 || selection.isCollapsed) {
    return ''
  }

  const fragments: DocumentFragment[] = []
  let hasResourceToken = false
  for (let index = 0; index < selection.rangeCount; index += 1) {
    const range = selection.getRangeAt(index)
    if (!rangeIntersectsNode(range, root)) continue
    const fragment = range.cloneContents()
    const links = Array.from(fragment.querySelectorAll<HTMLAnchorElement>('a.workspace-resource-token'))
    if (links.length === 0) {
      fragments.push(fragment)
      continue
    }
    hasResourceToken = true
    links.forEach((link) => {
      link.replaceWith(document.createTextNode(getWorkspaceResourceTokenCopyText(link)))
    })
    fragments.push(fragment)
  }

  if (!hasResourceToken) return ''
  return fragments.map(fragment => fragment.textContent || '').join('').trim()
}

function readBlockValue(lines: string[], key: string): string {
  const prefix = `${key}：`
  const line = lines.find((item) => item.trim().startsWith(prefix))
  return line ? line.trim().slice(prefix.length).trim() : ''
}

function readBlockParams(lines: string[]): WorkspaceInvocationParam[] {
  const paramTitleIndex = lines.findIndex((line) => line.trim() === '参数：')
  if (paramTitleIndex < 0) return []

  return lines
    .slice(paramTitleIndex + 1)
    .map((line) => parseParamLine(line))
    .filter((param): param is WorkspaceInvocationParam => !!param)
}

function parseParamLine(line: string): WorkspaceInvocationParam | null {
  const trimmed = line.trim()
  if (!trimmed || !trimmed.includes('=')) return null
  const [rawKey, ...rawValueParts] = trimmed.split('=')
  const key = String(rawKey || '').trim()
  const rawValue = rawValueParts.join('=').trim()
  if (!key) return null

  return {
    key,
    value: rawValue.replace(/（固定）$/, '').trim(),
    fixed: rawValue.endsWith('（固定）'),
  }
}

function formatInvocationValue(value: unknown): string {
  if (value === undefined || value === null || value === '') return '（留空）'
  if (typeof value === 'string') return value
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

function isEmptyInvocationValue(value: unknown): boolean {
  if (value === undefined || value === null || value === '') return true
  if (Array.isArray(value)) return value.length === 0
  if (typeof value === 'object') return Object.keys(value as Record<string, unknown>).length === 0
  return false
}

function stripWorkspaceResourceWrapper(value: string): string {
  const trimmed = String(value || '').trim()
  if (!trimmed) return ''
  if (trimmed.startsWith('<') && trimmed.endsWith('>')) {
    return trimmed.slice(1, -1).trim()
  }
  return trimmed
}

function getWorkspaceResourceTokenCopyText(link: HTMLAnchorElement): string {
  const path = link.dataset.fullCodePath || link.getAttribute('href')?.replace(/^#/, '') || ''
  const token = wrapWorkspaceResourcePath(path)
  return token || (link.textContent || '').trim()
}

function rangeIntersectsNode(range: Range, node: Node): boolean {
  if (typeof range.intersectsNode === 'function') {
    return range.intersectsNode(node)
  }
  return node.contains(range.commonAncestorContainer) || range.commonAncestorContainer.contains(node)
}

function getMarkdownCodeRanges(source: string): TextRange[] {
  const ranges = getMarkdownFenceRanges(source)
  ranges.push(...getMarkdownInlineCodeRanges(source, ranges))
  ranges.sort((a, b) => a.start - b.start || a.end - b.end)
  return ranges
}

function getMarkdownFenceRanges(source: string): TextRange[] {
  const ranges: TextRange[] = []
  let position = 0
  let fenceStart = -1
  let fenceMarker = ''
  let fenceSize = 0

  while (position < source.length) {
    const lineStart = position
    const newlineIndex = source.indexOf('\n', position)
    const lineEnd = newlineIndex === -1 ? source.length : newlineIndex + 1
    const lineText = source.slice(lineStart, newlineIndex === -1 ? lineEnd : newlineIndex)
    const fenceMatch = lineText.match(/^[ \t]{0,3}(`{3,}|~{3,})/)
    if (fenceMatch) {
      const marker = fenceMatch[1] || ''
      if (!marker) {
        if (newlineIndex === -1) break
        position = lineEnd
        continue
      }
      const markerChar = marker.charAt(0)
      const markerSize = marker.length
      if (fenceStart < 0) {
        fenceStart = lineStart
        fenceMarker = markerChar
        fenceSize = markerSize
      } else if (markerChar === fenceMarker && markerSize >= fenceSize) {
        ranges.push({ start: fenceStart, end: lineEnd })
        fenceStart = -1
        fenceMarker = ''
        fenceSize = 0
      }
    }

    if (newlineIndex === -1) break
    position = lineEnd
  }

  if (fenceStart >= 0) {
    ranges.push({ start: fenceStart, end: source.length })
  }
  return ranges
}

function getMarkdownInlineCodeRanges(source: string, fencedRanges: TextRange[]): TextRange[] {
  const ranges: TextRange[] = []
  let index = 0
  while (index < source.length) {
    if (isIndexInRanges(index, fencedRanges)) {
      index += 1
      continue
    }
    if (source[index] !== '`') {
      index += 1
      continue
    }

    const runLength = countBacktickRun(source, index)
    const marker = '`'.repeat(runLength)
    const closeIndex = source.indexOf(marker, index + runLength)
    if (closeIndex < 0) {
      index += runLength
      continue
    }
    ranges.push({ start: index, end: closeIndex + runLength })
    index = closeIndex + runLength
  }
  return ranges
}

function countBacktickRun(source: string, start: number): number {
  let end = start
  while (end < source.length && source[end] === '`') {
    end += 1
  }
  return end - start
}

function isEscapedResourceTokenStart(source: string, start: number): boolean {
  let slashCount = 0
  for (let index = start - 1; index >= 0 && source[index] === '\\'; index -= 1) {
    slashCount += 1
  }
  return slashCount % 2 === 1
}

function isIndexInRanges(index: number, ranges: TextRange[]): boolean {
  return ranges.some(range => index >= range.start && index < range.end)
}

export function isWorkspaceToolResourcePath(path: string): boolean {
  return String(path || '').trim().startsWith(TOOL_RESOURCE_PREFIX)
}

export function workspaceToolName(pathOrToken: string): string {
  const body = stripWorkspaceResourceWrapper(pathOrToken)
  if (!isWorkspaceToolResourcePath(body)) return ''
  return body.slice(TOOL_RESOURCE_PREFIX.length).trim()
}

export function wrapWorkspaceToolName(name: string): string {
  const normalized = String(name || '').trim()
  return normalized ? `<${TOOL_RESOURCE_PREFIX}${normalized}>` : ''
}

function isRelativeWorkspaceResourcePath(path: string): boolean {
  return path.startsWith('./') || path.startsWith('../')
}

function joinWorkspacePath(basePath: string, relativePath: string): string {
  const baseDir = workspacePathDirectory(basePath)
  if (!baseDir) return ''

  const { pathPart, queryPart } = splitWorkspacePathQuery(relativePath)
  const stack = baseDir.split('/').filter(Boolean)
  pathPart.split('/').forEach((part) => {
    if (!part || part === '.') return
    if (part === '..') {
      stack.pop()
      return
    }
    stack.push(part)
  })
  const resolved = `/${stack.join('/')}`
  return queryPart ? `${resolved}?${queryPart}` : resolved
}

function workspacePathDirectory(basePath: string): string {
  const normalized = normalizeWorkspaceResourcePath(basePath)
  if (!normalized.startsWith('/')) return ''

  const { pathPart } = splitWorkspacePathQuery(normalized)
  const parts = pathPart.split('/').filter(Boolean)
  if (parts.length === 0) return ''

  const tail = parts[parts.length - 1] || ''
  if (tail.includes('.')) {
    parts.pop()
  }
  return `/${parts.join('/')}`
}

function splitWorkspacePathQuery(value: string): { pathPart: string; queryPart: string } {
  const index = value.indexOf('?')
  if (index < 0) {
    return { pathPart: value, queryPart: '' }
  }
  return {
    pathPart: value.slice(0, index),
    queryPart: value.slice(index + 1),
  }
}
