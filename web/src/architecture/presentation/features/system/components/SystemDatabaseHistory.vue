<template>
  <section class="database-history-card">
    <header class="history-header">
      <div><span class="eyebrow">{{ t('systemSettings.resources.databaseInventoryTitle') }}</span><h4>{{ label('historyTitle') }}</h4><p>{{ label('historyDescription') }}</p></div>
      <el-radio-group :model-value="days" size="small" @change="changeDays">
        <el-radio-button :value="7">{{ label('days7') }}</el-radio-button><el-radio-button :value="30">{{ label('days30') }}</el-radio-button>
      </el-radio-group>
    </header>
    <div class="history-controls">
      <el-select :model-value="database" :placeholder="label('allDatabases')" filterable :aria-label="label('inspectDatabase')" @change="changeDatabase">
        <el-option :label="label('allDatabases')" value="" /><el-option v-for="item in databases" :key="databaseKey(item)" :label="databaseLabel(item)" :value="databaseKey(item)" />
      </el-select>
      <el-radio-group v-model="view" size="small"><el-radio-button value="line">{{ label('line') }}</el-radio-button><el-radio-button value="bar">{{ label('bar') }}</el-radio-button><el-radio-button value="table">{{ label('table') }}</el-radio-button></el-radio-group>
    </div>
    <el-table v-if="view === 'table'" :data="[...history].reverse()" size="small" max-height="360">
      <el-table-column :label="label('date')" min-width="120"><template #default="{ row }">{{ pointDate(row) }}</template></el-table-column>
      <el-table-column :label="label('size')" min-width="140"><template #default="{ row }">{{ row.database_size_available ? bytes(row.database_logical_bytes) : label('noSample') }}</template></el-table-column>
      <el-table-column :label="label('delta')" min-width="120"><template #default="{ row }">{{ row.database_logical_delta_available ? signedBytes(row.database_logical_delta) : label('unavailableComparison') }}</template></el-table-column>
      <el-table-column :label="t('systemSettings.resources.databaseDailyCount')" min-width="100"><template #default="{ row }">{{ row.database_count_available ? row.database_count : '—' }}</template></el-table-column>
      <el-table-column :label="label('sampledAt')" min-width="190"><template #default="{ row }">{{ row.database_size_available ? sampleTime(row.collected_at) : '—' }}</template></el-table-column>
      <el-table-column :label="label('comparedAt')" min-width="190"><template #default="{ row }">{{ row.previous_collected_at ? sampleTime(row.previous_collected_at) : label('unavailableComparison') }}</template></el-table-column>
    </el-table>
    <VChart v-else-if="history.some(point => point.database_size_available)" class="history-chart" :option="option" autoresize :aria-label="label('historyTitle')" />
    <el-empty v-else :description="label('noSample')" :image-size="72" />
    <footer>{{ selectedLabel }} <span>{{ label('size') }} · {{ label('delta') }}</span></footer>
  </section>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import type { SystemCapacityDailyPoint, SystemDatabaseSize } from '@/architecture/presentation/context/api/system-settings'
use([CanvasRenderer, LineChart, BarChart, GridComponent, TooltipComponent])
const props = defineProps<{ history: SystemCapacityDailyPoint[]; databases: SystemDatabaseSize[]; days: number; database: string }>()
const emit = defineEmits<{ change: [days: number, database: string] }>()
const { t, locale } = useI18n()
const view = ref('line')
const databaseKey = (item: SystemDatabaseSize) => item.source_id ? `${item.source_id}/${item.name}` : item.name
const databaseLabel = (item: SystemDatabaseSize) => item.source_id ? `${item.name} · ${item.source_id.slice(0, 8)}` : item.name
const selectedLabel = computed(() => {
 const item = props.databases.find(item => databaseKey(item) === props.database)
 return item ? databaseLabel(item) : props.database || label('allDatabases')
})
function changeDays(value: string | number | boolean | undefined) { emit('change', Number(value), props.database) }
function changeDatabase(value: string) { emit('change', props.days, value) }
const label = (key: string) => t(`systemSettings.resources.dashboard.${key}`)
const sampleTime = (value: string) => new Date(value).toLocaleString(locale?.value)
const date = (value: string) => new Date(value).toLocaleDateString(locale?.value, { month: 'short', day: 'numeric' })
const pointDate = (point: SystemCapacityDailyPoint) => date(point.date ? `${point.date}T00:00:00` : point.collected_at)
function bytes(value: number) {
 const units = ['B', 'KB', 'MB', 'GB', 'TB']
 const unit = Math.min(4, Math.max(0, Math.floor(Math.log2(Math.max(1, Math.abs(value))) / 10)))
 return `${(value / 1024 ** unit).toLocaleString(undefined, { maximumFractionDigits: 1 })} ${units[unit]}`
}
const signedBytes = (value: number) => value === 0 ? label('unchanged') : `${value > 0 ? '+' : ''}${bytes(value)}`
const option = computed(() => ({
 animationDuration: 240, grid: { left: 76, right: 24, top: 26, bottom: 38 },
 tooltip: { trigger: 'axis', confine: true, renderMode: 'richText', formatter: (params: { dataIndex: number }[]) => {
  const point = props.history[params[0]?.dataIndex ?? 0]
  if (!point) return ''
  return `${pointDate(point)}\n${label('size')}: ${point.database_size_available ? bytes(point.database_logical_bytes) : label('noSample')}\n${label('delta')}: ${point.database_logical_delta_available ? signedBytes(point.database_logical_delta) : label('unavailableComparison')}\n${label('sampledAt')}: ${point.database_size_available ? sampleTime(point.collected_at) : '—'}\n${label('comparedAt')}: ${point.previous_collected_at ? sampleTime(point.previous_collected_at) : label('unavailableComparison')}`
 } },
 xAxis: { type: 'category', data: props.history.map(point => pointDate(point)), boundaryGap: view.value === 'bar', axisLine: { show: false }, axisTick: { show: false }, axisLabel: { color: '#8892a3', fontSize: 11, hideOverlap: true } },
 yAxis: { type: 'value', min: 0, axisLabel: { color: '#8892a3', fontSize: 11, formatter: bytes }, splitLine: { lineStyle: { color: 'rgba(140,155,180,.15)', type: 'dashed' } } },
 series: [{ name: label('size'), type: view.value, connectNulls: false, smooth: false, showSymbol: props.days === 7, symbolSize: 7, barMaxWidth: 24, lineStyle: { width: 3 }, itemStyle: { color: '#5985ed', borderRadius: [4, 4, 0, 0] }, areaStyle: { color: 'rgba(89,133,237,.10)' }, data: props.history.map(point => point.database_size_available ? point.database_logical_bytes : null) }],
}))
</script>
<style scoped>
.database-history-card { padding: 24px; border: 1px solid var(--border-light); border-radius: 16px; background: var(--bg-primary); margin: 20px 0; }
.history-header, .history-controls { display: flex; justify-content: space-between; gap: 16px; align-items: center; }
.eyebrow { font-size: 11px; color: var(--text-secondary); }
h4 { margin: 5px 0 7px; font-size: 20px; font-weight: 650; letter-spacing: -.4px; color: var(--text-primary); }
p { margin: 0; color: var(--text-secondary); font-size: 12px; }
.history-controls { margin: 24px 0 10px; }
.history-controls .el-select { width: min(360px, 55%); }
.history-chart { height: 300px; width: 100%; }
footer { border-top: 1px solid var(--border-light); padding-top: 16px; margin-top: 8px; display: flex; justify-content: space-between; gap: 12px; font-size: 11px; color: var(--text-secondary); overflow-wrap: anywhere; }
@media (max-width: 720px) { .database-history-card { padding: 16px; } .history-header, .history-controls { align-items: stretch; flex-direction: column; } .history-controls .el-select { width: 100%; } .history-chart { height: 260px; } }
</style>
