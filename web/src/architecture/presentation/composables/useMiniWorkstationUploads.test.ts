import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { appendWorkspaceResourceTokens, extractClipboardFiles, useMiniWorkstationUploads } from './useMiniWorkstationUploads'

vi.mock('@/architecture/presentation/context/appStoresContext', () => ({
  useAuthStore: () => ({ userName: 'demo' }),
}))

function file(name: string, options: { type?: string; content?: string } = {}) {
  return new File([options.content || 'x'], name, {
    type: options.type || 'text/plain',
    lastModified: 123
  })
}

describe('extractClipboardFiles', () => {
  it('extracts files from clipboard file lists', () => {
    const image = file('clip.png', { type: 'image/png' })
    const doc = file('notes.txt')

    const files = extractClipboardFiles({
      files: [image, doc],
      items: []
    } as unknown as DataTransfer)

    expect(files).toEqual([image, doc])
  })

  it('extracts file items and keeps text-only clipboard data alone', () => {
    const image = file('screenshot.png', { type: 'image/png' })

    const files = extractClipboardFiles({
      files: [],
      items: [
        { kind: 'string', getAsFile: () => null },
        { kind: 'file', getAsFile: () => image }
      ]
    } as unknown as DataTransfer)

    expect(files).toEqual([image])
    expect(extractClipboardFiles({
      files: [],
      items: [{ kind: 'string', getAsFile: () => null }]
    } as unknown as DataTransfer)).toEqual([])
  })

  it('prefers file lists when both clipboard surfaces expose files', () => {
    const image = file('clip.png', { type: 'image/png' })

    const files = extractClipboardFiles({
      files: [image],
      items: [{ kind: 'file', getAsFile: () => image }]
    } as unknown as DataTransfer)

    expect(files).toEqual([image])
  })
})

describe('appendWorkspaceResourceTokens', () => {
  it('adds a dragged resource without generated instruction copy', () => {
    const result = appendWorkspaceResourceTokens('', ['/system/sales/customers.table'])

    expect(result).toBe('</system/sales/customers.table>')
    expect(result).not.toContain('请处理以下')
  })

  it('keeps previously dragged resources when another one is added', () => {
    const first = appendWorkspaceResourceTokens('', ['/system/sales/customers.table'])
    const second = appendWorkspaceResourceTokens(first, ['/system/sales/create_customer.form'])

    expect(second).toBe('</system/sales/customers.table> </system/sales/create_customer.form>')
  })

  it('appends multiple resources in one payload and ignores duplicates', () => {
    const result = appendWorkspaceResourceTokens(
      '核对 ',
      [
        '/system/sales/customers.table',
        '/system/sales/create_customer.form',
        '/system/sales/customers.table',
      ]
    )

    expect(result).toBe('核对 </system/sales/customers.table> </system/sales/create_customer.form>')
  })

  it('recognizes an existing relative token against the current service directory', () => {
    const result = appendWorkspaceResourceTokens(
      '<./customers.table>',
      ['/system/sales/customers.table'],
      '/system/sales'
    )

    expect(result).toBe('<./customers.table>')
  })
})

describe('workspace node drop', () => {
  it('lets the composer insert dragged resources at its remembered caret', async () => {
    const inputText = ref('分析 ')
    const insertWorkspaceResources = vi.fn()
    const uploads = useMiniWorkstationUploads({
      fullCodePath: ref('/system/sales'),
      inputText,
      inputRef: ref({ focus: vi.fn(), insertWorkspaceResources }),
    })

    await uploads.onDrop({
      dataTransfer: {
        types: ['application/x-workspace-node'],
        getData: () => JSON.stringify({ full_code_path: '/system/sales/customers.table' }),
        files: [],
      },
    } as unknown as DragEvent)

    expect(inputText.value).toBe('分析 ')
    expect(insertWorkspaceResources).toHaveBeenCalledWith(['/system/sales/customers.table'])
  })
})
