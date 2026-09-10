import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import MiniWorkstationComposer from './MiniWorkstationComposer.vue'

const ButtonStub = {
  props: ['disabled', 'loading'],
  emits: ['click'],
  template: '<button type="button" :disabled="disabled" @click="$emit(\'click\')"><slot /></button>',
}

const UploadStub = {
  props: ['disabled'],
  template: '<div data-testid="upload" :data-disabled="disabled ? \'true\' : \'false\'"><slot /></div>',
}

function mountComposer(props: Record<string, unknown> = {}) {
  return mount(MiniWorkstationComposer, {
    props: {
      fullCodePath: '/system/ticket_sys/v1',
      dirName: 'v1',
      attachedFiles: [],
      uploading: false,
      inputText: '',
      sending: false,
      stopping: false,
      selectedLLMConfigId: 0,
      llmList: [],
      llmLoading: false,
      queuedCount: 0,
      registerInputRef: vi.fn(),
      onLLMSelectVisibleChange: vi.fn(),
      onFileChange: vi.fn(),
      removeFile: vi.fn(),
      onInputEnter: vi.fn(),
      ...props,
    },
    global: {
      stubs: {
        ElButton: ButtonStub,
        ElUpload: UploadStub,
        ElSelect: { template: '<div><slot /></div>' },
        ElOption: { template: '<span />' },
        ElTooltip: { template: '<span><slot /></span>' },
        ElIcon: { template: '<span><slot /></span>' },
      },
    },
  })
}

describe('MiniWorkstationComposer', () => {
  it('keeps the input visible but blocked for an explicit generic interaction', async () => {
    const wrapper = mountComposer({
      blocked: true,
      blockedLabel: '等待处理',
      blockedPlaceholder: '请先处理当前操作',
    })

    const editor = wrapper.find('[data-testid="mini-workstation-input"]')
    expect(editor.exists()).toBe(true)
    expect(editor.attributes('contenteditable')).toBe('false')
    expect(editor.attributes('data-placeholder')).toBe('请先处理当前操作')
    expect(wrapper.text()).toContain('等待处理')

    editor.element.textContent = '继续生成'
    await editor.trigger('input')
    expect(wrapper.emitted('update:inputText')).toBeUndefined()
    expect(wrapper.find('[data-testid="upload"]').attributes('data-disabled')).toBe('true')
  })

  it('emits input updates when unblocked', async () => {
    const wrapper = mountComposer()
    const editor = wrapper.find('[data-testid="mini-workstation-input"]')
    editor.element.textContent = '继续生成'
    await editor.trigger('input')

    expect(wrapper.emitted('update:inputText')?.[0]?.[0]).toBe('继续生成')
  })

  it('keeps Enter as submit for chat input', async () => {
    const onInputEnter = vi.fn()
    const wrapper = mountComposer({
      inputText: '继续生成',
      onInputEnter,
    })
    const editor = wrapper.find('[data-testid="mini-workstation-input"]')

    await editor.trigger('keydown', { key: 'Enter' })

    expect(onInputEnter).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('update:inputText')).toBeUndefined()
  })

  it('uses Enter as a newline in schedule instructions', async () => {
    const onInputEnter = vi.fn()
    const wrapper = mountComposer({
      variant: 'schedule',
      inputText: '第一行',
      onInputEnter,
    })
    const editor = wrapper.find('[data-testid="mini-workstation-input"]')

    await editor.trigger('keydown', { key: 'Enter' })

    expect(onInputEnter).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:inputText')?.at(-1)?.[0]).toBe('第一行\n')
  })

  it('uses the input-only layout for a non-maximized window', () => {
    const wrapper = mountComposer({
      compactWindow: true,
      attachedFiles: [
        { name: 'draft.txt', source_name: 'draft.txt', ref: 'file://draft.txt' },
        { name: '客户名单.xlsx', source_name: '客户名单.xlsx', ref: 'file://customers.xlsx' },
      ],
    })

    expect(wrapper.find('[data-testid="mini-workstation-composer"]').classes()).toContain('mini-ws-input--compact-window')
    expect(wrapper.find('[data-testid="mini-workstation-attached-files"]').exists()).toBe(true)
    expect(wrapper.find('.mini-ws-files').classes()).toContain('mini-ws-files--compact')
    expect(wrapper.findAll('.mini-ws-file-chip')).toHaveLength(2)
    expect(wrapper.text()).toContain('draft.txt')
    expect(wrapper.text()).toContain('客户名单.xlsx')
    expect(wrapper.find('[data-testid="mini-workstation-input"]').exists()).toBe(true)
    expect(wrapper.find('.mini-path-pill').exists()).toBe(true)
    expect(wrapper.find('[data-testid="mini-workstation-send"]').exists()).toBe(true)
  })

  it('allows an attached file to be removed from the visible file strip', async () => {
    const removeFile = vi.fn()
    const wrapper = mountComposer({
      attachedFiles: [{ name: 'draft.txt', source_name: 'draft.txt', ref: 'file://draft.txt' }],
      removeFile,
    })

    await wrapper.find('.mini-ws-file-chip__remove').trigger('click')

    expect(removeFile).toHaveBeenCalledWith(0)
  })
})
