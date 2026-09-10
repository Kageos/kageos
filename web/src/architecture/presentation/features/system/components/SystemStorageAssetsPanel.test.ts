import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SystemStorageAssetsPanel from './SystemStorageAssetsPanel.vue'

const storageApi = vi.hoisted(() => ({
  listSystemStorageAssets: vi.fn(),
  getSystemStorageAssetAccessURL: vi.fn(),
  listSystemStorageAssetAudits: vi.fn(),
  deleteFileRefs: vi.fn(),
}))

vi.mock('@/architecture/presentation/context/api/storage', () => storageApi)
vi.mock('@/architecture/presentation/context/api/app', () => ({
  getAppList: vi.fn().mockResolvedValue([{ user: 'alice', code: 'app', name: '订单工作空间' }]),
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => `${key}${params ? ` ${Object.values(params).join(' ')}` : ''}` }) }))

describe('SystemStorageAssetsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    storageApi.listSystemStorageAssets.mockReset()
    storageApi.getSystemStorageAssetAccessURL.mockResolvedValue({ url: '/files/a.png' })
    storageApi.listSystemStorageAssetAudits.mockResolvedValue({
      list: [{ id: 9, action: 'preview', username: 'alice', ip_address: '127.0.0.1', accessed_at: '2026-09-01T02:00:00Z' }],
    })
    storageApi.listSystemStorageAssets.mockResolvedValue({
      list: [{
        id: 1, bucket: 'kageos', ref: 'kageos/alice/app/orders/a.png', file_key: 'alice/app/orders/a.png',
        router: 'alice/app/orders/export.table', file_name: 'a.png', file_size: 2048, content_type: 'image/png',
        username: 'alice', tenant: 'alice', status: 'completed', uploaded_at: '2026-09-01T01:00:00Z',
        previewable: true, preview_kind: 'image', thumbnail_url: '/files/a-thumb.png', preview_count: 2, download_count: 3,
      }],
      total: 1, page: 1, page_size: 20,
      summary: { active_files: 1, active_bytes: 2048, deleted_files: 2, failed_files: 0 },
      directories: [{ router: 'alice/app/orders/export.table', file_count: 1, size_bytes: 2048 }],
      workspaces: [{ path: 'alice/app', file_count: 1, size_bytes: 2048 }],
      metadata_available: true, console_url: 'http://127.0.0.1:9001', coverage: 'tracked_uploads',
    })
  })

  it('shows auditable files grouped by service path', async () => {
    const wrapper = mount(SystemStorageAssetsPanel)
    await flushPromises()

    expect(storageApi.listSystemStorageAssets).toHaveBeenCalledWith(expect.objectContaining({ status: 'completed', page_size: 20 }))
    expect(wrapper.text()).not.toContain('systemSettings.resources.assets.openConsole')
    expect(wrapper.text()).toContain('a.png')
    expect(wrapper.text()).toContain('订单工作空间')
    expect(wrapper.text()).toContain('/orders')
    expect(wrapper.text()).toContain('export.table')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('3')
    expect(wrapper.text()).toContain('2.0 KB')
    expect(wrapper.find('.workspace-usage-card').exists()).toBe(true)
  })

  it('previews a supported file and records preview access separately', async () => {
    const wrapper = mount(SystemStorageAssetsPanel)
    await flushPromises()

    const previewButton = wrapper.findAll('button').find(button => button.text().includes('systemSettings.resources.assets.preview'))
    expect(previewButton).toBeTruthy()
    await previewButton!.trigger('click')
    await flushPromises()

    expect(storageApi.getSystemStorageAssetAccessURL).toHaveBeenCalledWith('kageos/alice/app/orders/a.png', 'preview')
    expect(document.body.querySelector<HTMLImageElement>('.asset-preview-stage img')?.getAttribute('src')).toBe('/files/a.png')
    wrapper.unmount()
  })

  it('loads per-file access history from the detail drawer', async () => {
    const wrapper = mount(SystemStorageAssetsPanel)
    await flushPromises()

    await wrapper.find('.file-name-button').trigger('click')
    await flushPromises()

    expect(storageApi.listSystemStorageAssetAudits).toHaveBeenCalledWith('kageos/alice/app/orders/a.png', 30)
    expect(document.body.textContent).toContain('127.0.0.1')
    wrapper.unmount()
  })
  it('switches to gallery and keeps file details accessible', async () => {
    const wrapper = mount(SystemStorageAssetsPanel)
    await flushPromises()
    wrapper.findComponent({ name: 'ElRadioGroup' }).vm.$emit('update:modelValue', 'grid')
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.asset-gallery').exists()).toBe(true)
    expect(wrapper.find('.asset-table').exists()).toBe(false)
    await wrapper.find('.gallery-copy .file-name-button').trigger('click')
    await flushPromises()
    expect(storageApi.listSystemStorageAssetAudits).toHaveBeenCalled()
    wrapper.unmount()
  })

})
