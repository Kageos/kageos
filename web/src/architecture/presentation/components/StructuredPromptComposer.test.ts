import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { getUserInfo } from '@/architecture/presentation/context/api/auth'
import { searchUsersFuzzy } from '@/architecture/presentation/context/api/user'
import { searchResources, searchFunctions, getServiceTreeDetail } from '@/architecture/presentation/context/api/service-tree'
import StructuredPromptComposer from './StructuredPromptComposer.vue'

vi.mock('@/architecture/presentation/context/api/auth', () => ({ getUserInfo: vi.fn(async () => ({ username: 'beiluo', nickname: '北落', avatar: '', email: '', signature: '' })) }))

vi.mock('@/architecture/presentation/context/api/user', () => ({
  getUsersByUsernames: vi.fn(async () => ({ users: [] })),
  searchUsersFuzzy: vi.fn(async () => ({
    users: [{
      username: 'system',
      nickname: 'system(系统)',
      avatar: '',
      email: '',
      signature: '',
    }],
  })),
}))

vi.mock('@/architecture/presentation/context/api/service-tree', () => ({
  getServiceTreeDetail: vi.fn(async () => {
    throw new Error('not found')
  }),
  searchFunctions: vi.fn(async () => ({ functions: [] })),
  searchResources: vi.fn(async () => ({
    items: [{
      id: 1,
      name: '订单表',
      code: 'orders',
      type: 'function',
      full_code_path: '/system/app/orders.table',
      description: '订单数据表',
      template_type: 'table',
      run_count: 0,
    }],
  })),
}))

const IconStub = {
  template: '<span><slot /></span>',
}

function mountComposer(modelValue: string) {
  return mount(StructuredPromptComposer, {
    props: {
      modelValue,
      placeholder: '输入任务',
    },
    global: {
      stubs: {
        ElIcon: IconStub,
        EditPen: IconStub,
        View: IconStub,
      },
    },
  })
}

describe('StructuredPromptComposer', () => {
  it('browses the current directory on bare slash and filters types and other directories', async () => {
    vi.useFakeTimers()
    const result = { items: [
      { id: 1, name: '订单表', code: 'orders', type: 'function' as const, template_type: 'table', full_code_path: '/system/app/orders.table' },
      { id: 2, name: '新增订单', code: 'create', type: 'function' as const, template_type: 'form', full_code_path: '/system/app/create.form' },
      { id: 3, name: '外部订单', code: 'orders', type: 'function' as const, template_type: 'table', full_code_path: '/system/app2/orders.table' },
    ], total: 3, page: 1, page_size: 100 }
    vi.mocked(searchResources).mockResolvedValueOnce(result)
    vi.mocked(searchFunctions).mockResolvedValueOnce({ functions: result.items.map(item => ({ ...item, description: '', app_id: 1, app_user: 'system', app_code: 'app' })), total: 3, page: 1, page_size: 100 }).mockResolvedValueOnce({ functions: result.items.map(item => ({ ...item, description: '', app_id: 1, app_user: 'system', app_code: 'app' })), total: 3, page: 1, page_size: 100 })
    const wrapper = mountComposer('')
    try {
      await wrapper.setProps({ fullCodePath: '/system/app/current.docs' })
      const editor = wrapper.find('[data-testid="structured-prompt-editor"]')
      editor.element.textContent = '/'
      await editor.trigger('input')
      await vi.advanceTimersByTimeAsync(230)
      expect(searchResources).toHaveBeenLastCalledWith(expect.objectContaining({ keyword: '', full_code_path: '/system/app' }))
      const panel = () => document.querySelector('[data-testid="structured-prompt-mention-panel"]')!
      expect(panel().textContent).toContain('订单表')
      expect(Array.from(panel().querySelectorAll('[role="tab"]')).map(tab => tab.textContent)).toEqual(['全部', '文档', '目录', '数据表', '表单', '图表', '其他'])
      expect(Array.from(panel().querySelectorAll('.spc-mention-type')).map(tag => tag.textContent)).toEqual(['数据表', '表单'])
      expect(panel().textContent).not.toContain('外部订单')
      const formTab = Array.from(panel().querySelectorAll<HTMLButtonElement>('[role="tab"]')).find(button => button.textContent === '表单')!
      formTab.click()
      await vi.advanceTimersByTimeAsync(230)
      expect(panel().textContent).toContain('新增订单')
      expect(panel().textContent).not.toContain('订单表')
      const other = Array.from(panel().querySelectorAll<HTMLButtonElement>('.spc-resource-scopes button'))[1]!
      other.click()
      await vi.advanceTimersByTimeAsync(230)
      expect(searchFunctions).toHaveBeenLastCalledWith(expect.objectContaining({ full_code_path: '', template_type: 'form' }))
      expect(panel().textContent).not.toContain('新增订单')
      // A fresh slash session resets scope and type.
      await editor.trigger('keydown', { key: 'Escape' })
      editor.element.textContent = '/'
      await editor.trigger('input')
      await vi.advanceTimersByTimeAsync(230)
      expect(searchResources).toHaveBeenLastCalledWith(expect.objectContaining({ full_code_path: '/system/app', resource_type: 'all' }))
      await editor.trigger('keydown', { key: 'Enter' })
      expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('</system/app/orders.table> ')
    } finally {
      wrapper.unmount()
      vi.useRealTimers()
    }
  })

  it('keeps the trailing empty line and middle caret stable when typing newlines', async () => {
    const wrapper = mount(StructuredPromptComposer, { attachTo: document.body, props: { modelValue: '第一行' } })
    try {
      const editor = wrapper.find('[data-testid="structured-prompt-editor"]')
      const element = editor.element as HTMLElement
      element.focus()
      const range = document.createRange()
      range.selectNodeContents(element)
      range.collapse(false)
      window.getSelection()!.removeAllRanges()
      window.getSelection()!.addRange(range)
      await editor.trigger('keydown', { key: 'Enter', shiftKey: true })
      expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('第一行\n')
      expect(element.lastChild?.nodeName).toBe('BR')
      const selection = window.getSelection()!
      expect(selection.anchorNode?.textContent).toBe('\n')
      expect(selection.anchorOffset).toBe(1)
      range.setStart(element.firstChild!, 1)
      range.collapse(true)
      selection.removeAllRanges()
      selection.addRange(range)
      await editor.trigger('keydown', { key: 'Enter', shiftKey: true })
      expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('第\n一行\n')
      expect(selection.anchorNode?.textContent).toBe('\n')
      expect(selection.anchorOffset).toBe(1)
    } finally { wrapper.unmount() }
  })

  it('shows the signed-in user on bare @ and allows keyboard insertion', async () => {
    vi.useFakeTimers()
    const wrapper = mountComposer('')
    try {
      const editor = wrapper.find('[data-testid="structured-prompt-editor"]')
      editor.element.textContent = '@'
      await editor.trigger('input')
      await vi.advanceTimersByTimeAsync(230)
      expect(getUserInfo).toHaveBeenCalled()
      expect(document.querySelector('[data-testid="structured-prompt-mention-panel"]')?.textContent).toContain('我自己')
      await editor.trigger('keydown', { key: 'Enter' })
      expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('@beiluo ')
    } finally { wrapper.unmount(); vi.useRealTimers() }
  })

  it('opens resource details outside the editor and loads the directory purpose', async () => {
    const wrapper = mountComposer('</system/app>')
    try {
      vi.mocked(getServiceTreeDetail).mockResolvedValueOnce({
        id: 1, name: '客户管理', code: 'app', type: 'package', full_code_path: '/system/app',
        description: '维护客户资料并安排后续跟进。', tags: '', app_id: 1, ref_id: 0, created_at: '', updated_at: '',
      })
      await wrapper.find('.spc-editor-token.is-resource').trigger('click')
      await flushPromises()
      const card = document.querySelector<HTMLElement>('[data-testid="structured-prompt-info-card"]')!
      expect(wrapper.element.contains(card)).toBe(false)
      expect(card.textContent).toContain('维护客户资料并安排后续跟进。')
      expect(card.textContent).toContain('客户管理')
      card.querySelector<HTMLButtonElement>('[aria-label="关闭信息卡片"]')!.click()
      await nextTick()
      expect(document.querySelector('[data-testid="structured-prompt-info-card"]')).toBeNull()
    } finally { wrapper.unmount() }
  })

  it('keeps editing and preview content left-aligned', async () => {
    const wrapper = mountComposer('从左侧开始输入')
    const editor = wrapper.find('[data-testid="structured-prompt-editor"]')

    expect(editor.attributes('style')).toContain('text-align: left')
    await wrapper.findAll('.spc-mode-btn')[1]?.trigger('click')
    expect(wrapper.find('[data-testid="structured-prompt-preview"]').attributes('style')).toContain('text-align: left')
  })

  it('renders resource path tokens in edit mode', () => {
    const wrapper = mountComposer('调用 </system/demos/weixin/wechat_articles/search_articles.form>')

    const token = wrapper.find('.spc-editor-token')
    expect(token.exists()).toBe(true)
    expect(token.attributes('data-token-raw')).toBe('</system/demos/weixin/wechat_articles/search_articles.form>')
    expect(token.text()).toBe('search_articles.form')
  })

  it('renders multiple dragged resource paths as separate tokens', () => {
    const wrapper = mountComposer('</system/sales/customers.table> </system/sales/create_customer.form>')

    const tokens = wrapper.findAll('.spc-editor-token.is-resource')
    expect(tokens).toHaveLength(2)
    expect(tokens.map(token => token.text())).toEqual(['customers.table', 'create_customer.form'])
    expect(wrapper.text()).not.toContain('请处理以下')
  })

  it('renders relative resource tokens against the current workspace path', () => {
    const wrapper = mount(StructuredPromptComposer, {
      props: {
        modelValue: '调用 <./record_screening.form>',
        fullCodePath: '/system/democase/recruit_interview',
      },
      global: {
        stubs: {
          ElIcon: IconStub,
          EditPen: IconStub,
          View: IconStub,
        },
      },
    })

    const token = wrapper.find('.spc-editor-token')
    expect(token.exists()).toBe(true)
    expect(token.attributes('data-token-raw')).toBe('<./record_screening.form>')
    expect(token.attributes('data-path')).toBe('/system/democase/recruit_interview/record_screening.form')
    expect(token.text()).toBe('record_screening.form')
  })

  it('renders invocation cards in preview mode', async () => {
    const wrapper = mountComposer([
      '函数调用：',
      '用途：复制后粘贴到工作台，AI 会按下面信息识别并调用。',
      '工具：run_table_create',
      '函数：</system/app/orders.table>',
      '',
      '参数：',
      'body = [{"title":"测试"}]',
    ].join('\n'))

    await wrapper.findAll('.spc-mode-btn')[1]?.trigger('click')

    expect(wrapper.find('.spc-invocation-card').exists()).toBe(true)
    expect(wrapper.text()).toContain('run_table_create')
    expect(wrapper.text()).toContain('orders.table')
    expect(wrapper.text()).toContain('body')
  })

  it('renders relative invocation resources in preview mode', async () => {
    const wrapper = mount(StructuredPromptComposer, {
      props: {
        modelValue: [
          '函数调用：',
          '工具：run_form_submit',
          '函数：<./record_screening.form>',
        ].join('\n'),
        fullCodePath: '/system/democase/recruit_interview',
      },
      global: {
        stubs: {
          ElIcon: IconStub,
          EditPen: IconStub,
          View: IconStub,
        },
      },
    })

    await wrapper.findAll('.spc-mode-btn')[1]?.trigger('click')

    const resource = wrapper.find('.spc-invocation-resource')
    expect(resource.exists()).toBe(true)
    expect(resource.attributes('title')).toBe('/system/democase/recruit_interview/record_screening.form')
    expect(resource.text()).toContain('record_screening.form')
  })

  it('renders readonly preview without exposing edit mode', async () => {
    const wrapper = mount(StructuredPromptComposer, {
      props: {
        modelValue: '请 @system 检查 </system/app/orders.table>',
        readonlyPreview: true,
        showToolbar: false,
        disabled: true,
      },
      global: {
        stubs: {
          ElIcon: IconStub,
          EditPen: IconStub,
          View: IconStub,
        },
      },
    })

    expect(wrapper.find('[data-testid="structured-prompt-preview"]').isVisible()).toBe(true)
    expect(wrapper.find('[data-testid="structured-prompt-editor"]').isVisible()).toBe(false)
    expect(wrapper.find('.spc-user-chip').text()).toContain('@system')
    expect(wrapper.find('.spc-resource-chip').text()).toContain('orders.table')
  })

  it('emits serialized raw resource tokens when edited', async () => {
    const wrapper = mountComposer('调用 </system/app/search.form>')
    const editor = wrapper.find('[data-testid="structured-prompt-editor"]')

    editor.element.appendChild(document.createTextNode(' 完成后总结'))
    await editor.trigger('input')

    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toBe('调用 </system/app/search.form> 完成后总结')
  })

  it('does not rebuild the contenteditable DOM while typing plain text', async () => {
    vi.useFakeTimers()
    try {
      const wrapper = mountComposer('')
      const editor = wrapper.find('[data-testid="structured-prompt-editor"]')
      const replaceChildren = vi.spyOn(editor.element, 'replaceChildren')

      editor.element.textContent = '继续完善客户资料'
      await editor.trigger('focus')
      await editor.trigger('input')
      await vi.advanceTimersByTimeAsync(280)

      expect(replaceChildren).not.toHaveBeenCalled()
      expect(editor.element.textContent).toBe('继续完善客户资料')
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('renders user mentions as chips while keeping raw @username text', async () => {
    const wrapper = mountComposer('请 @beiluo 协助处理')
    const editor = wrapper.find('[data-testid="structured-prompt-editor"]')

    const token = wrapper.find('.spc-editor-token.is-user')
    expect(token.exists()).toBe(true)
    expect(token.attributes('data-token-raw')).toBe('@beiluo')

    editor.element.appendChild(document.createTextNode('，谢谢'))
    await editor.trigger('input')

    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toBe('请 @beiluo 协助处理，谢谢')
  })

  it('shows system mentions with readable labels and a click card', async () => {
    const wrapper = mountComposer('交给 @system 处理')

    const token = wrapper.find('.spc-editor-token.is-user')
    expect(token.text()).toBe('@system(系统)')
    expect(token.attributes('data-token-raw')).toBe('@system')

    await token.trigger('click')

    const card = document.querySelector('[data-testid="structured-prompt-info-card"]')
    expect(card).not.toBeNull()
    expect(card?.textContent).toContain('@system(系统)')
    expect(card?.textContent).toContain('@system')
    wrapper.unmount()
  })

  it('normalizes already decorated user mentions instead of nesting labels', async () => {
    const wrapper = mountComposer('交给 @system(system(系统)) 处理')
    const editor = wrapper.find('[data-testid="structured-prompt-editor"]')

    const token = wrapper.find('.spc-editor-token.is-user')
    expect(token.text()).toBe('@system(系统)')
    expect(token.attributes('data-token-raw')).toBe('@system')

    editor.element.appendChild(document.createTextNode('，谢谢'))
    await editor.trigger('input')

    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toBe('交给 @system 处理，谢谢')
  })

  it('does not submit while mention search is open and waiting for options', async () => {
    const wrapper = mount(StructuredPromptComposer, {
      props: {
        modelValue: '',
        submitOnEnter: true,
      },
      global: {
        stubs: {
          ElIcon: IconStub,
          EditPen: IconStub,
          View: IconStub,
        },
      },
    })
    const editor = wrapper.find('[data-testid="structured-prompt-editor"]')

    editor.element.textContent = '@sys'
    await editor.trigger('input')
    await editor.trigger('keydown', { key: 'Enter' })

    expect(wrapper.emitted('enter')).toBeUndefined()
  })

  it('selects the first mention option on enter by default', async () => {
    vi.useFakeTimers()
    try {
      const wrapper = mount(StructuredPromptComposer, {
        props: {
          modelValue: '',
          submitOnEnter: true,
        },
        global: {
          stubs: {
            ElIcon: IconStub,
            EditPen: IconStub,
            View: IconStub,
          },
        },
      })
      const editor = wrapper.find('[data-testid="structured-prompt-editor"]')

      editor.element.textContent = '@sys'
      await editor.trigger('input')
      await vi.advanceTimersByTimeAsync(230)
      await editor.trigger('keydown', { key: 'Enter' })

      expect(searchUsersFuzzy).toHaveBeenCalledWith('sys', 8)
      expect(wrapper.emitted('enter')).toBeUndefined()
      expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('@system ')
      expect(wrapper.find('.spc-editor-token.is-user').text()).toBe('@system(系统)')
    } finally {
      vi.useRealTimers()
    }
  })

  it('selects the first mention option when enter is pressed before search finishes', async () => {
    vi.useFakeTimers()
    try {
      const wrapper = mount(StructuredPromptComposer, {
        props: {
          modelValue: '',
          submitOnEnter: true,
        },
        global: {
          stubs: {
            ElIcon: IconStub,
            EditPen: IconStub,
            View: IconStub,
          },
        },
      })
      const editor = wrapper.find('[data-testid="structured-prompt-editor"]')

      editor.element.textContent = '@sys'
      await editor.trigger('input')
      await editor.trigger('keydown', { key: 'Enter' })
      await vi.advanceTimersByTimeAsync(230)

      expect(searchUsersFuzzy).toHaveBeenCalledWith('sys', 8)
      expect(wrapper.emitted('enter')).toBeUndefined()
      expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('@system ')
      expect(wrapper.find('.spc-editor-token.is-user').text()).toBe('@system(系统)')
    } finally {
      vi.useRealTimers()
    }
  })

  it('selects the first mention option when enter arrives before the mention panel opens', async () => {
    vi.useFakeTimers()
    try {
      const wrapper = mount(StructuredPromptComposer, {
        props: {
          modelValue: '',
          submitOnEnter: true,
        },
        global: {
          stubs: {
            ElIcon: IconStub,
            EditPen: IconStub,
            View: IconStub,
          },
        },
      })
      const editor = wrapper.find('[data-testid="structured-prompt-editor"]')

      editor.element.textContent = '@sys'
      await editor.trigger('keydown', { key: 'Enter' })
      await vi.advanceTimersByTimeAsync(230)

      expect(searchUsersFuzzy).toHaveBeenCalledWith('sys', 8)
      expect(wrapper.emitted('enter')).toBeUndefined()
      expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('@system ')
      expect(wrapper.find('.spc-editor-token.is-user').text()).toBe('@system(系统)')
    } finally {
      vi.useRealTimers()
    }
  })

  it('selects the loaded mention option on the first enter after composition ends', async () => {
    vi.useFakeTimers()
    try {
      const wrapper = mount(StructuredPromptComposer, {
        props: {
          modelValue: '',
          submitOnEnter: true,
        },
        global: {
          stubs: {
            ElIcon: IconStub,
            EditPen: IconStub,
            View: IconStub,
          },
        },
      })
      const editor = wrapper.find('[data-testid="structured-prompt-editor"]')

      await vi.advanceTimersByTimeAsync(10)
      await editor.trigger('compositionstart')
      editor.element.textContent = '@sys'
      await editor.trigger('input')
      await editor.trigger('compositionend')
      await nextTick()
      await vi.advanceTimersByTimeAsync(230)
      await editor.trigger('keydown', { key: 'Enter' })

      expect(searchUsersFuzzy).toHaveBeenCalledWith('sys', 8)
      expect(wrapper.emitted('enter')).toBeUndefined()
      expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('@system ')
      expect(wrapper.find('.spc-editor-token.is-user').text()).toBe('@system(系统)')
    } finally {
      vi.useRealTimers()
    }
  })

  it('keeps resource mention icon components raw when rendering options', async () => {
    vi.useFakeTimers()
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    try {
      const wrapper = mount(StructuredPromptComposer, {
        props: {
          modelValue: '',
        },
        global: {
          stubs: {
            ElIcon: IconStub,
            EditPen: IconStub,
            View: IconStub,
          },
        },
      })
      const editor = wrapper.find('[data-testid="structured-prompt-editor"]')

      editor.element.textContent = '/orders'
      await editor.trigger('input')
      await vi.advanceTimersByTimeAsync(230)
      await nextTick()

      expect(searchResources).toHaveBeenCalledWith(expect.objectContaining({ keyword: 'orders' }))
      expect(document.body.querySelector('.spc-mention-resource-component')).not.toBeNull()
      expect(warnSpy.mock.calls.some((args) => args.join(' ').includes('Component that was made reactive'))).toBe(false)
      wrapper.unmount()
    } finally {
      warnSpy.mockRestore()
      vi.useRealTimers()
    }
  })

  it('confirms the resource highlighted with arrow keys and displays its name', async () => {
    vi.useFakeTimers()
    vi.mocked(searchResources).mockResolvedValueOnce({
      items: [
        {
          id: 1,
          name: '订单表',
          code: 'orders',
          type: 'function',
          full_code_path: '/system/app/orders.table',
          description: '订单数据表',
          template_type: 'table',
        },
        {
          id: 2,
          name: '客户资料',
          code: 'customers',
          type: 'package',
          full_code_path: '/system/app/customers',
          description: '客户资料服务目录',
        },
      ],
      total: 2,
      page: 1,
      page_size: 8,
    })
    try {
      const wrapper = mount(StructuredPromptComposer, {
        props: {
          modelValue: '',
          submitOnEnter: true,
        },
        global: {
          stubs: {
            ElIcon: IconStub,
            EditPen: IconStub,
            View: IconStub,
          },
        },
      })
      const editor = wrapper.find('[data-testid="structured-prompt-editor"]')

      editor.element.textContent = '/客'
      await editor.trigger('input')
      await vi.advanceTimersByTimeAsync(230)
      await nextTick()

      await editor.trigger('keydown', { key: 'ArrowDown' })
      await nextTick()
      const activeOption = document.body.querySelector('.spc-mention-option.is-active')
      expect(activeOption?.getAttribute('data-testid')).toBe('structured-prompt-mention-option-1')

      await editor.trigger('keydown', { key: 'Enter' })

      expect(wrapper.emitted('enter')).toBeUndefined()
      expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('</system/app/customers> ')
      expect(wrapper.find('.spc-editor-token.is-resource').text()).toContain('客户资料')
      expect(wrapper.find('.spc-editor-token.is-resource').text()).not.toContain('customers')
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not submit or rerender while Chinese IME composition is committing', async () => {
    const wrapper = mount(StructuredPromptComposer, {
      props: {
        modelValue: '',
        submitOnEnter: true,
      },
      global: {
        stubs: {
          ElIcon: IconStub,
          EditPen: IconStub,
          View: IconStub,
        },
      },
    })
    const editor = wrapper.find('[data-testid="structured-prompt-editor"]')

    await editor.trigger('compositionstart')
    editor.element.textContent = 'ni'
    await editor.trigger('input')

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()

    editor.element.textContent = '你'
    await editor.trigger('compositionend')
    await nextTick()

    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toBe('你')

    await editor.trigger('keydown', { key: 'Enter' })

    expect(wrapper.emitted('enter')).toBeUndefined()
  })

  it('emits enter when submit-on-enter is enabled', async () => {
    const wrapper = mount(StructuredPromptComposer, {
      props: {
        modelValue: '执行任务',
        submitOnEnter: true,
      },
      global: {
        stubs: {
          ElIcon: IconStub,
          EditPen: IconStub,
          View: IconStub,
        },
      },
    })

    await wrapper.find('[data-testid="structured-prompt-editor"]').trigger('keydown', { key: 'Enter' })

    expect(wrapper.emitted('enter')).toHaveLength(1)
  })
})
