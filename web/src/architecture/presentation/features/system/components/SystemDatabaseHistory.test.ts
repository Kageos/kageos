import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import SystemDatabaseHistory from './SystemDatabaseHistory.vue'
import type { SystemCapacityDailyPoint } from '@/architecture/presentation/context/api/system-settings'
vi.mock('vue-echarts', () => ({ default: { name: 'VChart', props: ['option'], template: '<div />' } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const point = (day: number, available: boolean, value: number): SystemCapacityDailyPoint => ({ collected_at: `2026-09-0${day}T03:00:00Z`, database_logical_bytes: value, database_size_available: available, database_logical_delta: 10, database_logical_delta_available: false, database_count: 1, database_count_available: available, database_count_delta: 0, database_count_delta_available: false, platform_database_count: 1, workspace_database_count: 0 })
describe('SystemDatabaseHistory', () => {
 it('keeps unavailable samples as gaps and supports period, database and table controls', async () => {
  const wrapper = mount(SystemDatabaseHistory, { props: { history: [point(1,true,100), point(2,false,0), point(3,true,150)], days: 7, database: '', databases: [] } })
  const chart = wrapper.findComponent({name:'VChart'})
  expect(chart.props('option').series[0].data).toEqual([100,null,150])
  expect(chart.props('option').series[0].connectNulls).toBe(false)
  const groups = wrapper.findAllComponents({name:'ElRadioGroup'})
  groups[0]!.vm.$emit('change', 30)
  expect(wrapper.emitted('change')?.[0]).toEqual([30,''])
  wrapper.findComponent({name:'ElSelect'}).vm.$emit('change','hr-server')
  expect(wrapper.emitted('change')?.[1]).toEqual([7,'hr-server'])
  groups[1]!.vm.$emit('update:modelValue','bar')
  await wrapper.vm.$nextTick()
  expect(chart.props('option').series[0].type).toBe('bar')
  groups[1]!.vm.$emit('update:modelValue','table')
  await wrapper.vm.$nextTick()
  expect(wrapper.findComponent({name:'VChart'}).exists()).toBe(false)
  expect(wrapper.findComponent({name:'ElTable'}).props('data')[1].database_size_available).toBe(false)
 })
 it('distinguishes unchanged allocation from unavailable comparisons and shows sample times', () => {
  const current = { ...point(3,true,100), database_logical_delta: 0, database_logical_delta_available: true, previous_collected_at: '2026-09-02T03:00:00Z' }
  const wrapper = mount(SystemDatabaseHistory, { props: {history:[point(2,false,0),current],days:7,database:'',databases:[]} })
  const tooltip = wrapper.findComponent({name:'VChart'}).props('option').tooltip.formatter
  expect(tooltip([{dataIndex:1}])).toContain('systemSettings.resources.dashboard.unchanged')
  expect(tooltip([{dataIndex:1}])).toContain('systemSettings.resources.dashboard.comparedAt')
  expect(tooltip([{dataIndex:0}])).toContain('systemSettings.resources.dashboard.unavailableComparison')
  expect(tooltip([{dataIndex:0}])).not.toContain('systemSettings.resources.dashboard.unchanged')
 })

})
