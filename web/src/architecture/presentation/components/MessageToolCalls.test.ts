import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import MessageToolCalls from './MessageToolCalls.vue'

describe('MessageToolCalls', () => {
  it('shows explicit tool count and groups duplicate tool names', () => {
    const wrapper = mount(MessageToolCalls, {
      props: {
        toolCalls: [
          { name: 'change_role', status: 'ok' },
          { name: 'change_role', status: 'ok' },
          { name: 'read_dir', status: 'ok' },
        ],
        fileGroups: [],
      },
      global: {
        stubs: {
          OutputFilesDisplay: true,
          OutputDisplayFields: true,
          PrdPreview: true,
        },
      },
    })

    expect(wrapper.get('.summary-title').text()).toBe('工具调用 3 个')
    expect(wrapper.text()).toContain('change_role x2')
    expect(wrapper.text()).toContain('read_dir')
  })

  it('expands tool details by default when a PRD preview is available', () => {
    const wrapper = mount(MessageToolCalls, {
      props: {
        toolCalls: [
          {
            name: 'write_prd',
            status: 'ok',
            result_data: {
              kind: 'agent_app_prd',
              title: '确认 PRD',
            },
          },
        ],
        fileGroups: [],
      },
      global: {
        stubs: {
          OutputFilesDisplay: true,
          OutputDisplayFields: true,
          PrdPreview: true,
        },
      },
    })

    expect((wrapper.get('details').element as HTMLDetailsElement).open).toBe(true)
    expect(wrapper.find('prd-preview-stub').exists()).toBe(true)
  })

  it('does not expand a dedicated card for build failures', () => {
    const wrapper = mount(MessageToolCalls, {
      props: {
        toolCalls: [{
          name: 'build_workspace',
          status: 'error',
          error: 'compile failed',
          result_data: { kind: 'agent_app_build_failure' },
        }],
        fileGroups: [],
      },
      global: {
        stubs: {
          OutputFilesDisplay: true,
          OutputDisplayFields: true,
          PrdPreview: true,
        },
      },
    })

    expect((wrapper.get('details').element as HTMLDetailsElement).open).toBe(false)
    expect(wrapper.find('.mini-msg-build-diagnostics').exists()).toBe(false)
  })
})
