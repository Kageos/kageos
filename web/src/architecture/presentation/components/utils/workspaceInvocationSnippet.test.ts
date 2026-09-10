import { describe, expect, it } from 'vitest'
import {
  buildWorkspaceInvocationSnippet,
  filterEmptyInvocationParams,
  insertWorkspaceResourceTokensAtOffset,
  parseWorkspaceInvocationBlocks,
  parseWorkspacePromptSegments,
  renderWorkspaceResourceTokensAsHtml,
  resolveWorkspaceResourcePath,
  unwrapWorkspaceResourceToken,
  wrapWorkspaceResourcePath,
  workspaceResourceKind,
} from './workspaceInvocationSnippet'

describe('workspaceInvocationSnippet', () => {
  it('wraps and unwraps resource paths with the lightweight token syntax', () => {
    const token = wrapWorkspaceResourcePath('/system/demos/weixin/wechat_articles/search_articles.form')

    expect(token).toBe('</system/demos/weixin/wechat_articles/search_articles.form>')
    expect(unwrapWorkspaceResourceToken(token)).toBe('/system/demos/weixin/wechat_articles/search_articles.form')
  })

  it('preserves relative resource tokens and resolves them against a workspace path', () => {
    const token = wrapWorkspaceResourcePath('./record_screening.form')
    const siblingToken = wrapWorkspaceResourcePath('../shared/jobs.table')

    expect(token).toBe('<./record_screening.form>')
    expect(siblingToken).toBe('<../shared/jobs.table>')
    expect(unwrapWorkspaceResourceToken(token)).toBe('./record_screening.form')
    expect(unwrapWorkspaceResourceToken(token, '/system/democase/recruit_interview')).toBe('/system/democase/recruit_interview/record_screening.form')
    expect(unwrapWorkspaceResourceToken(siblingToken, '/system/democase/recruit_interview')).toBe('/system/democase/shared/jobs.table')
    expect(resolveWorkspaceResourcePath('../shared/jobs.table', '/system/democase/recruit_interview')).toBe('/system/democase/shared/jobs.table')
  })

  it('splits prompt text into text and resource segments', () => {
    const segments = parseWorkspacePromptSegments('调用 </system/app/search.form> 后总结')

    expect(segments.map((segment) => segment.type)).toEqual(['text', 'resource', 'text'])
    expect(segments[1]).toMatchObject({
      path: '/system/app/search.form',
      text: '</system/app/search.form>',
    })
  })

  it('inserts dragged resources at the caret with readable spacing', () => {
    const result = insertWorkspaceResourceTokensAtOffset(
      '请检查然后总结',
      ['/system/sales/customers.table'],
      3
    )

    expect(result.value).toBe('请检查 </system/sales/customers.table> 然后总结')
    expect(result.cursor).toBe('请检查 </system/sales/customers.table> '.length)
  })

  it('splits relative resource tokens with a resolved segment path', () => {
    const segments = parseWorkspacePromptSegments('调用 <./record_screening.form> 后总结', '/system/democase/recruit_interview')

    expect(segments.map((segment) => segment.type)).toEqual(['text', 'resource', 'text'])
    expect(segments[1]).toMatchObject({
      path: '/system/democase/recruit_interview/record_screening.form',
      text: '<./record_screening.form>',
    })
  })

  it('renders resource tokens as safe workspace links', () => {
    const html = renderWorkspaceResourceTokensAsHtml(
      '先查 <./orders.table>，再提交 </system/app/notify.form>',
      '/system/app/runbook.docs'
    )

    expect(html).toContain('class="workspace-resource-token is-table"')
    expect(html).toContain('href="/workspace/system/app/orders.table"')
    expect(html).toContain('data-full-code-path="/system/app/orders.table"')
    expect(html).toContain('orders.table')
    expect(html).toContain('class="workspace-resource-token is-form"')
    expect(html).toContain('href="/workspace/system/app/notify.form"')
  })

  it('uses workspace display names and keeps sent tokens consistent with the composer', () => {
    const html = renderWorkspaceResourceTokensAsHtml(
      '分析 </system/demo/customer_follow> 和 </system/demo/meeting>',
      '/system/demo',
      {
        '/system/demo/customer_follow': '客户跟进',
        '/system/demo/meeting': '会议管理',
      }
    )

    expect(html).toContain('>客户跟进</span>')
    expect(html).toContain('>会议管理</span>')
    expect(html).not.toContain('workspace-resource-token__type')
    expect(html).not.toContain('>服务目录</span>')
  })

  it('renders docs resource tokens with Chinese labels and preview metadata', () => {
    const html = renderWorkspaceResourceTokensAsHtml('阅读 <./runbook.docs>', '/system/app/index.docs')

    expect(html).toContain('class="workspace-resource-token is-docs"')
    expect(html).toContain('href="/workspace/system/app/runbook.docs"')
    expect(html).toContain('data-resource-kind="docs"')
    expect(html).toContain('data-resource-type-label="文档"')
    expect(html).toContain('data-resource-icon-src="/文档.svg"')
    expect(html).toContain('workspace-resource-token__img')
    expect(html).toContain('runbook.docs')
    expect(html).not.toContain('Docs')
  })

  it('renders built-in tool tokens without converting them to workspace paths', () => {
    const segments = parseWorkspacePromptSegments('异常时调用 <tool:send_notification> 通知')

    expect(segments.map((segment) => segment.type)).toEqual(['text', 'resource', 'text'])
    expect(segments[1]).toMatchObject({
      path: 'tool:send_notification',
      text: '<tool:send_notification>',
    })

    const html = renderWorkspaceResourceTokensAsHtml('异常时调用 <tool:send_notification> 通知')

    expect(unwrapWorkspaceResourceToken('<tool:send_notification>')).toBe('tool:send_notification')
    expect(workspaceResourceKind('<tool:send_notification>')).toBe('tool')
    expect(html).toContain('class="workspace-resource-token is-tool"')
    expect(html).toContain('href="#tool:send_notification"')
    expect(html).toContain('data-full-code-path="tool:send_notification"')
    expect(html).toContain('send_notification')
    expect(html).toContain('内置工具')
    expect(html).toContain('workspace-resource-token__glyph--tool')
    expect(html).not.toContain('tool-icon')
  })

  it('keeps resource token syntax literal inside markdown code', () => {
    const inlineHtml = renderWorkspaceResourceTokensAsHtml('写作 `<tool:send_notification>` 来引用工具')
    const fencedHtml = renderWorkspaceResourceTokensAsHtml([
      '```md',
      '<tool:send_notification>',
      '</system/app/orders.table>',
      '```',
    ].join('\n'))

    expect(inlineHtml).toBe('写作 `<tool:send_notification>` 来引用工具')
    expect(fencedHtml).toContain('<tool:send_notification>')
    expect(fencedHtml).toContain('</system/app/orders.table>')
    expect(inlineHtml).not.toContain('workspace-resource-token')
    expect(fencedHtml).not.toContain('workspace-resource-token')
  })

  it('keeps escaped resource token syntax literal', () => {
    const html = renderWorkspaceResourceTokensAsHtml('展示 \\<tool:send_notification> 原始写法')

    expect(html).toBe('展示 \\<tool:send_notification> 原始写法')
    expect(html).not.toContain('workspace-resource-token')
  })

  it('detects resource kind from function suffixes', () => {
    expect(workspaceResourceKind('/system/app/list.table')).toBe('table')
    expect(workspaceResourceKind('/system/app/input.form')).toBe('form')
    expect(workspaceResourceKind('/system/app/summary.chart')).toBe('chart')
    expect(workspaceResourceKind('/system/app/runbook.docs')).toBe('docs')
    expect(workspaceResourceKind('tool:send_notification')).toBe('tool')
    expect(workspaceResourceKind('/system/app/orders')).toBe('directory')
  })

  it('builds a workspace-readable invocation block', () => {
    const snippet = buildWorkspaceInvocationSnippet({
      tool: 'run_form_submit',
      resourcePath: '/system/app/search.form',
      params: {
        keyword: 'AI 最新热点',
        page_size: 5,
      },
    })

    expect(snippet).toContain('函数调用：')
    expect(snippet).toContain('用途：复制后粘贴到工作台')
    expect(snippet).toContain('工具：run_form_submit')
    expect(snippet).toContain('函数：</system/app/search.form>')
    expect(snippet).toContain('keyword = AI 最新热点')
    expect(snippet).toContain('page_size = 5')
  })

  it('parses invocation blocks back into a light structure for preview', () => {
    const snippet = buildWorkspaceInvocationSnippet({
      tool: 'run_table_update',
      resourcePath: '/system/app/orders.table',
      params: [
        { key: 'body', value: [{ id: 1, updates: { status: '已处理' } }] },
        { key: 'groupid', value: 1, fixed: true },
      ],
    })

    const blocks = parseWorkspaceInvocationBlocks(snippet)

    expect(blocks).toHaveLength(1)
    expect(blocks[0]).toMatchObject({
      tool: 'run_table_update',
      resourcePath: '/system/app/orders.table',
    })
    expect(blocks[0]?.params.map((param) => ({ key: param.key, fixed: param.fixed }))).toEqual([
      { key: 'body', fixed: false },
      { key: 'groupid', fixed: true },
    ])
  })

  it('parses relative invocation block resources against a workspace path', () => {
    const blocks = parseWorkspaceInvocationBlocks([
      '函数调用：',
      '工具：run_form_submit',
      '函数：<./record_screening.form>',
    ].join('\n'), '/system/democase/recruit_interview')

    expect(blocks).toHaveLength(1)
    expect(blocks[0]?.resourcePath).toBe('/system/democase/recruit_interview/record_screening.form')
  })

  it('filters empty params before copying partially filled forms', () => {
    expect(filterEmptyInvocationParams({
      keyword: 'AI',
      empty: '',
      none: null,
      list: [],
      config_id: 0,
    })).toEqual({
      keyword: 'AI',
      config_id: 0,
    })
  })
})
