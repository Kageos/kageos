import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SystemUsagePanel from './SystemUsagePanel.vue'

const usageApi = vi.hoisted(() => ({ getSystemResourceUsage: vi.fn() }))

vi.mock('@/architecture/presentation/context/api/system-settings', () => usageApi)
vi.mock('vue-echarts', () => ({ default: { name: 'VChart', props: ['option'], template: '<div class="usage-echart-stub" />' } }))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => `${key}${params ? ` ${Object.values(params).join(' ')}` : ''}`,
  }),
}))

describe('SystemUsagePanel', () => {
  beforeEach(() => {
    usageApi.getSystemResourceUsage.mockReset()
    usageApi.getSystemResourceUsage.mockResolvedValue({
      available: true,
      collected_at: '2026-09-02T02:00:00Z',
      period_days: 7,
      ranking_basis: 'period',
      operations_today: 9,
      operations_period: 42,
      failed_operations: 2,
      successful_calls: 61,
      snapshot_schedule_local: '02:00',
      daily_history: [
        { date: '2026-09-01', operations: 33, failed: 1 },
        { date: '2026-09-02', operations: 9, failed: 2 },
      ],
      top_directories: [{ path: '/system/demos/sales', name: '销售', function_count: 3, total_calls: 120, period_calls: 40 }],
      top_functions: [{ path: '/system/demos/sales/orders.table', name: '订单', directory_path: '/system/demos/sales', directory_name: '销售', template_type: 'table', total_calls: 80, period_calls: 31 }],
      directory_total: 18,
      function_total: 31,
      ranking_page: 1,
      ranking_page_size: 10,
    })
  })

  it('renders period rankings and configures axes with exact hover values', async () => {
    const wrapper = mount(SystemUsagePanel)
    await flushPromises()

    expect(usageApi.getSystemResourceUsage).toHaveBeenCalledWith(7, 1, 10)
    expect(wrapper.text()).toContain('销售')
    expect(wrapper.find('.rank-resource-icon img').exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'ElPagination' }).exists()).toBe(true)

    const option = wrapper.findComponent({ name: 'VChart' }).props('option') as any
    expect(option.xAxis.type).toBe('category')
    expect(option.yAxis.type).toBe('value')
    expect(option.series.map((item: any) => item.type)).toEqual(['line', 'bar'])
    const tooltip = option.tooltip.formatter([{ dataIndex: 0 }])
    expect(tooltip).toContain('33')
    expect(tooltip).toContain('1')
  })
  it('keeps the newest period when earlier requests finish later', async () => {
    const wrapper = mount(SystemUsagePanel)
    await flushPromises()
    const base = await usageApi.getSystemResourceUsage.mock.results[0]!.value
    let finishOld!: (value: unknown) => void
    let finishNew!: (value: unknown) => void
    usageApi.getSystemResourceUsage.mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve }))
    usageApi.getSystemResourceUsage.mockImplementationOnce(() => new Promise(resolve => { finishNew = resolve }))
    const period = wrapper.findAllComponents({ name: 'ElRadioGroup' })[0]!
    period.vm.$emit('update:modelValue', 30)
    period.vm.$emit('change', 30)
    await wrapper.vm.$nextTick()
    period.vm.$emit('update:modelValue', 7)
    period.vm.$emit('change', 7)
    await wrapper.vm.$nextTick()
    finishNew({ ...base, operations_period: 9876 })
    await flushPromises()
    finishOld({ ...base, operations_period: 1234 })
    await flushPromises()
    expect(wrapper.find('.usage-summary-grid').text()).toContain('9,876')
    expect(wrapper.find('.usage-summary-grid').text()).not.toContain('1,234')
    expect(usageApi.getSystemResourceUsage).toHaveBeenLastCalledWith(7, 1, 10)
  })

})
