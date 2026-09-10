import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ElSwitch, ElMessageBox } from 'element-plus'
import SystemBackupPanel from './SystemBackupPanel.vue'

const backupApi = vi.hoisted(() => ({
  getSystemBackupOverview: vi.fn(),
  runSystemBackupNow: vi.fn(),
  testSystemBackupS3: vi.fn(),
  updateSystemBackupConfig: vi.fn(),
}))

vi.mock('@/architecture/presentation/context/api/system-settings', () => backupApi)
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('SystemBackupPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    backupApi.getSystemBackupOverview.mockResolvedValue({
      config: {
        enabled: true, schedule_time: '03:30', endpoint: '', region: 'us-east-1', bucket: 'backups', prefix: 'kageos',
        access_key_id: 'key', secret_access_key_set: true, use_ssl: true, force_path_style: false, keep_local: 2, retention_days: 30,
      },
      agent_available: true,
      running: false,
      records: [],
    })
  })

  const button = (wrapper: ReturnType<typeof mount>, key: string) => wrapper.findAll('button').find(item => item.text() === key)!

  const createPanel = () => mount(SystemBackupPanel, { global: { stubs: {
    ElDialog: { props: ['modelValue', 'beforeClose'], template: '<div v-if="modelValue" class="dialog"><slot /><slot name="footer" /></div>' },
  } } })
  const openEditor = async (wrapper: ReturnType<typeof mount>) => {
    await flushPromises()
    await button(wrapper, 'systemSettings.dataBackup.editConfig').trigger('click')
    await flushPromises()
  }

  it('shows the saved summary and opens the form only in the editor', async () => {
    const wrapper = createPanel()
    await flushPromises()
    expect(wrapper.find('.backup-form').exists()).toBe(false)
    await openEditor(wrapper)
    expect(wrapper.findAll('.backup-form-section')).toHaveLength(2)
    await button(wrapper, 'common.cancel').trigger('click')
    await flushPromises()
    expect(wrapper.find('.backup-form').exists()).toBe(false)
    wrapper.unmount()
  })

  it('tests a draft without saving and closes only after a successful save', async () => {
    const wrapper = createPanel()
    await openEditor(wrapper)
    wrapper.findComponent(ElSwitch).vm.$emit('update:modelValue', false)
    await flushPromises()
    backupApi.testSystemBackupS3.mockResolvedValue(undefined)
    await button(wrapper, 'systemSettings.dataBackup.test').trigger('click')
    await flushPromises()
    expect(backupApi.testSystemBackupS3).toHaveBeenCalledWith(expect.objectContaining({ enabled: false }))
    expect(backupApi.updateSystemBackupConfig).not.toHaveBeenCalled()
    expect(wrapper.find('.backup-status-row').text()).toContain('systemSettings.on')
    expect(wrapper.text()).toContain('systemSettings.dataBackup.testPassedUnsaved')
    backupApi.updateSystemBackupConfig.mockRejectedValueOnce(new Error('save failed'))
    await button(wrapper, 'userSettings.saveConfig').trigger('click')
    await flushPromises()
    expect(wrapper.find('.backup-form').exists()).toBe(true)
    expect(wrapper.text()).toContain('save failed')
    expect(wrapper.findComponent(ElSwitch).props('modelValue')).toBe(false)
    const current = await backupApi.getSystemBackupOverview()
    backupApi.updateSystemBackupConfig.mockResolvedValue({ ...current, config: { ...current.config, enabled: false } })
    await button(wrapper, 'userSettings.saveConfig').trigger('click')
    await flushPromises()
    expect(wrapper.find('.backup-form').exists()).toBe(false)
    expect(wrapper.find('.backup-status-row').text()).toContain('systemSettings.off')
    wrapper.unmount()
  })

  it('keeps unsaved edits when close is cancelled and discards only after confirmation', async () => {
    const confirm = vi.spyOn(ElMessageBox, 'confirm').mockRejectedValueOnce('cancel')
    const wrapper = createPanel()
    await openEditor(wrapper)
    wrapper.findComponent(ElSwitch).vm.$emit('update:modelValue', false)
    await flushPromises()
    await button(wrapper, 'common.cancel').trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(ElSwitch).props('modelValue')).toBe(false)
    expect(backupApi.updateSystemBackupConfig).not.toHaveBeenCalled()
    confirm.mockResolvedValueOnce('confirm' as Awaited<ReturnType<typeof ElMessageBox.confirm>>)
    await button(wrapper, 'common.cancel').trigger('click')
    await flushPromises()
    expect(wrapper.find('.backup-form').exists()).toBe(false)
    await openEditor(wrapper)
    expect(wrapper.findComponent(ElSwitch).props('modelValue')).toBe(true)
    confirm.mockRestore()
    wrapper.unmount()
  })

  it('offers add configuration when no bucket is configured', async () => {
    const current = await backupApi.getSystemBackupOverview()
    backupApi.getSystemBackupOverview.mockResolvedValue({ ...current, config: { ...current.config, bucket: '', secret_access_key_set: false } })
    const wrapper = createPanel()
    await flushPromises()
    await button(wrapper, 'systemSettings.dataBackup.addConfig').trigger('click')
    await flushPromises()
    expect(wrapper.find('.backup-form').exists()).toBe(true)
    wrapper.unmount()
  })
})
