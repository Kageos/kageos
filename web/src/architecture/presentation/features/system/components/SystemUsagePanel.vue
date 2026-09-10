<template>
  <div v-loading="loading" class="usage-panel">
    <header class="usage-toolbar">
      <div><h4>{{ t('systemSettings.resources.usage.title') }}</h4><p>{{ t('systemSettings.resources.usage.description') }}</p></div>
      <el-radio-group v-model="periodDays" size="small" @change="handlePeriodChange">
        <el-radio-button :value="7">{{ t('systemSettings.resources.usage.days7') }}</el-radio-button>
        <el-radio-button :value="30">{{ t('systemSettings.resources.usage.days30') }}</el-radio-button>
      </el-radio-group>
    </header>

    <template v-if="overview?.available">
      <div class="usage-summary-grid">
        <article><span>{{ t('systemSettings.resources.usage.operationsToday') }}</span><strong>{{ formatCount(overview.operations_today) }}</strong><small>{{ t('systemSettings.resources.usage.auditedHint') }}</small></article>
        <article><span>{{ t('systemSettings.resources.usage.operationsPeriod', { days: periodDays }) }}</span><strong>{{ formatCount(overview.operations_period) }}</strong><small>{{ averagePerDayLabel }}</small></article>
        <article><span>{{ rankingLabel }}</span><strong>{{ formatCount(overview.successful_calls) }}</strong><small>{{ t('systemSettings.resources.usage.successfulCallsHint') }}</small></article>
        <article><span>{{ t('systemSettings.resources.usage.failedPeriod', { days: periodDays }) }}</span><strong :class="{ danger: overview.failed_operations > 0 }">{{ formatCount(overview.failed_operations) }}</strong><small>{{ failureRateLabel }}</small></article>
      </div>

      <el-alert v-if="overview.ranking_basis === 'cumulative'" :title="t('systemSettings.resources.usage.collectingTitle')" :description="t('systemSettings.resources.usage.collectingDesc', { time: overview.snapshot_schedule_local })" type="info" show-icon :closable="false" />

      <section class="usage-card trend-card">
        <div class="section-heading">
          <div><h5>{{ t('systemSettings.resources.usage.trendTitle') }}</h5><p>{{ t('systemSettings.resources.usage.trendDesc') }}</p></div>
          <el-radio-group v-model="chartView" size="small"><el-radio-button value="line">{{ t('systemSettings.resources.dashboard.line') }}</el-radio-button><el-radio-button value="bar">{{ t('systemSettings.resources.dashboard.bar') }}</el-radio-button></el-radio-group>
        </div>
        <VChart v-if="overview.daily_history.length" class="usage-echart" :option="chartOption" autoresize :aria-label="t('systemSettings.resources.usage.trendTitle')" />
        <el-empty v-else :description="t('systemSettings.resources.usage.noHistory')" :image-size="72" />
      </section>

      <p class="usage-collected">{{ collectedLabel }}</p>
      <section class="usage-card ranking-card">
        <div class="section-heading ranking-heading">
          <div>
            <h5>{{ rankingMode === 'directories' ? t('systemSettings.resources.usage.topDirectories') : t('systemSettings.resources.usage.topFunctions') }}</h5>
            <p>{{ rankingMode === 'directories' ? t('systemSettings.resources.usage.topDirectoriesDesc') : t('systemSettings.resources.usage.topFunctionsDesc') }}</p>
          </div>
          <el-radio-group v-model="rankingMode" size="small" @change="handleRankingModeChange">
            <el-radio-button value="directories">{{ t('systemSettings.resources.usage.byDirectory') }}</el-radio-button>
            <el-radio-button value="functions">{{ t('systemSettings.resources.usage.byFunction') }}</el-radio-button>
          </el-radio-group>
        </div>
        <div v-if="rankingRows.length" class="ranking-table">
          <div class="ranking-table-head"><span>{{ t('systemSettings.resources.usage.rank') }}</span><span>{{ t('systemSettings.resources.usage.nameAndPath') }}</span><span>{{ rankingMode === 'directories' ? t('systemSettings.resources.usage.functionCount') : t('systemSettings.resources.usage.type') }}</span><span>{{ t('systemSettings.resources.usage.periodCalls') }}</span></div>
          <article v-for="(item, index) in rankingRows" :key="item.path">
            <span class="rank-number" :class="{ top: rankingOffset + index < 3 }">{{ rankingOffset + index + 1 }}</span>
            <div class="rank-name">
              <span class="rank-resource">
                <span class="rank-resource-icon">
                  <img v-if="rankingMode === 'directories'" src="/service-tree/custom-folder.svg" :alt="t('systemSettings.resources.usage.directoryIconAlt')" />
                  <img v-else-if="item.template_type === 'form'" src="/service-tree/编辑.svg" :alt="templateLabel(item.template_type)" />
                  <el-icon v-else><component :is="functionIcon(item.template_type)" /></el-icon>
                </span>
                <strong>{{ item.name || item.path }}</strong>
              </span>
              <small :title="item.path">{{ item.path }}</small><div class="rank-track"><i :style="{ width: `${rankingPercent(item.period_calls, rankingPeak)}%` }" /></div>
            </div>
            <span class="rank-meta"><template v-if="rankingMode === 'directories'">{{ t('systemSettings.resources.usage.functionsUnit', { count: item.function_count || 0 }) }}</template><el-tag v-else size="small" effect="plain">{{ templateLabel(item.template_type) }}</el-tag></span>
            <strong class="rank-calls">{{ formatCount(item.period_calls) }}<small>{{ t('systemSettings.resources.usage.totalCalls', { count: formatCount(item.total_calls) }) }}</small></strong>
          </article>
        </div>
        <el-empty v-else :description="t('systemSettings.resources.usage.noRanking')" :image-size="64" />
        <footer v-if="rankingTotal > rankingPageSize" class="ranking-footer">
          <span>{{ t('systemSettings.resources.usage.rankingTotal', { count: formatCount(rankingTotal) }) }}</span>
          <el-pagination v-model:current-page="rankingPage" background layout="prev, pager, next" :page-size="rankingPageSize" :total="rankingTotal" @current-change="load" />
        </footer>
      </section>
    </template>
    <el-empty v-else-if="!loading" :description="t('systemSettings.resources.usage.unavailable')" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Document } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart, LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { getSystemResourceUsage, type SystemUsageOverview } from '@/architecture/presentation/context/api/system-settings'
import TableIcon from '@/architecture/presentation/shared/components/icons/TableIcon.vue'
import ChartIcon from '@/architecture/presentation/shared/components/icons/ChartIcon.vue'

use([CanvasRenderer, BarChart, LineChart, GridComponent, LegendComponent, TooltipComponent])

const { t } = useI18n()
const loading = ref(false)
const periodDays = ref(7)
const chartView = ref('line')
let requestSequence = 0
const rankingMode = ref<'directories' | 'functions'>('directories')
const rankingPage = ref(1)
const rankingPageSize = 10
const overview = ref<SystemUsageOverview | null>(null)

const rankingLabel = computed(() => overview.value?.ranking_basis === 'period' ? t('systemSettings.resources.usage.successfulCallsPeriod', { days: periodDays.value }) : t('systemSettings.resources.usage.successfulCallsTotal'))
const failureRateLabel = computed(() => !overview.value?.operations_period ? t('systemSettings.resources.usage.noFailedOperations') : t('systemSettings.resources.usage.failureRate', { value: (overview.value.failed_operations / overview.value.operations_period * 100).toFixed(1) }))
const averagePerDayLabel = computed(() => t('systemSettings.resources.usage.dailyAverage', { count: formatCount(Math.round((overview.value?.operations_period || 0) / periodDays.value)) }))
const collectedLabel = computed(() => overview.value?.collected_at ? t('systemSettings.resources.usage.collectedAt', { time: new Date(overview.value.collected_at).toLocaleString() }) : '-')
const rankingRows = computed<any[]>(() => rankingMode.value === 'directories' ? overview.value?.top_directories || [] : overview.value?.top_functions || [])
const rankingTotal = computed(() => rankingMode.value === 'directories' ? overview.value?.directory_total || 0 : overview.value?.function_total || 0)
const rankingOffset = computed(() => (rankingPage.value - 1) * rankingPageSize)
const rankingPeak = computed(() => Math.max(1, ...rankingRows.value.map(item => item.period_calls)))

const chartOption = computed(() => {
  const rows = overview.value?.daily_history || []
  return {
    animationDuration: 350,
    grid: { left: 64, right: 24, top: 52, bottom: 42 },
    legend: { top: 5, right: 12, itemWidth: 12, itemHeight: 8, textStyle: { color: '#8d93a6', fontSize: 12 } },
    tooltip: {
      trigger: 'axis', confine: true, backgroundColor: 'rgba(25, 28, 43, .96)', borderColor: 'rgba(148, 163, 184, .24)', borderWidth: 1,
      textStyle: { color: '#eef0f7', fontSize: 12 }, axisPointer: { type: 'line', lineStyle: { color: 'rgba(129, 140, 248, .55)', width: 1 } },
      formatter: (params: any[]) => {
        const row = rows[Number(params?.[0]?.dataIndex || 0)]
        if (!row) return ''
        return `<div style="min-width:150px"><strong>${formatDate(row.date)}</strong><div style="display:flex;justify-content:space-between;gap:24px;margin-top:8px"><span>${t('systemSettings.resources.usage.operations')}</span><b>${formatCount(row.operations)}</b></div><div style="display:flex;justify-content:space-between;gap:24px;margin-top:5px"><span>${t('systemSettings.resources.usage.failed')}</span><b>${formatCount(row.failed)}</b></div></div>`
      },
    },
    xAxis: { type: 'category', boundaryGap: chartView.value === 'bar', data: rows.map(item => shortDate(item.date)), axisLine: { lineStyle: { color: 'rgba(148, 163, 184, .24)' } }, axisTick: { show: false }, axisLabel: { color: '#8d93a6', fontSize: 11, interval: rows.length > 14 ? 4 : rows.length > 7 ? 1 : 0 } },
    yAxis: { type: 'value', min: 0, minInterval: 1, name: t('systemSettings.resources.usage.axisUnit'), nameTextStyle: { color: '#8d93a6', fontSize: 11, padding: [0, 0, 8, -34] }, axisLabel: { color: '#8d93a6', fontSize: 11, formatter: (value: number) => compactCount(value) }, splitLine: { lineStyle: { color: 'rgba(148, 163, 184, .13)', type: 'dashed' } } },
    series: [
      { name: t('systemSettings.resources.usage.operations'), type: chartView.value, smooth: false, symbol: 'circle', symbolSize: 7, showSymbol: rows.length <= 14, lineStyle: { width: 2.5, color: '#818cf8' }, itemStyle: { color: '#818cf8', borderColor: '#171a29', borderWidth: 2 }, areaStyle: { color: 'rgba(129, 140, 248, .16)' }, emphasis: { focus: 'series' }, data: rows.map(item => item.operations) },
      { name: t('systemSettings.resources.usage.failed'), type: 'bar', barMaxWidth: 16, itemStyle: { color: '#fb7185', borderRadius: [3, 3, 0, 0] }, emphasis: { focus: 'series' }, data: rows.map(item => item.failed) },
    ],
  }
})

async function load() {
 const sequence = ++requestSequence
 loading.value = true
 try {
  const result = await getSystemResourceUsage(periodDays.value, rankingPage.value, rankingPageSize)
  if (sequence === requestSequence) overview.value = result
 } catch (error: any) {
  if (sequence === requestSequence) {
   overview.value = null
   ElMessage.error(error?.response?.data?.msg || error?.message || t('systemSettings.resources.usage.loadFailed'))
  }
 } finally { if (sequence === requestSequence) loading.value = false }
}
function handlePeriodChange() { rankingPage.value = 1; overview.value = null; void load() }

function handleRankingModeChange() { rankingPage.value = 1; void load() }
function formatCount(value: number | string) { return Number(value || 0).toLocaleString() }
function compactCount(value: number) { return Intl.NumberFormat(undefined, { notation: value >= 1000 ? 'compact' : 'standard', maximumFractionDigits: 1 }).format(value) }
function shortDate(value: string) { const date = new Date(`${value}T00:00:00`); return `${date.getMonth() + 1}/${date.getDate()}` }
function formatDate(value: string) { return new Date(`${value}T00:00:00`).toLocaleDateString() }
function rankingPercent(value: number, peak: number) { return Math.max(value > 0 ? 3 : 0, Math.min(100, value / peak * 100)) }
function templateLabel(value: string) { return value ? value.charAt(0).toUpperCase() + value.slice(1) : '-' }
function functionIcon(value: string) { return value === 'table' ? TableIcon : value === 'chart' ? ChartIcon : Document }
onMounted(load)
</script>

<style scoped>
.usage-panel { display: grid; gap: 16px; }
.usage-toolbar, .section-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.usage-toolbar h4, .usage-toolbar p, .section-heading h5, .section-heading p { margin: 0; }
.usage-toolbar h4 { color: var(--text-primary); font-size: 16px; }
.usage-toolbar p, .section-heading p, .section-heading > span { margin-top: 5px; color: var(--text-secondary); font-size: 12px; }
.usage-summary-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.usage-summary-grid article { display: grid; gap: 5px; min-width: 0; padding: 15px 16px; border: 1px solid var(--border-light); border-radius: var(--border-radius-lg); background: var(--bg-tertiary); }
.usage-summary-grid span, .usage-summary-grid small { overflow: hidden; color: var(--text-secondary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.usage-summary-grid strong { color: var(--text-primary); font-size: 22px; }
.usage-summary-grid strong.danger { color: var(--el-color-danger); }
.usage-card { min-width: 0; padding: 18px; border: 1px solid var(--border-light); border-radius: var(--border-radius-lg); background: var(--bg-tertiary); }
.section-heading h5 { color: var(--text-primary); font-size: 14px; }
.usage-echart { width: 100%; height: 310px; margin-top: 8px; }
.ranking-heading { align-items: center; }
.ranking-table { margin-top: 12px; }
.ranking-table-head, .ranking-table article { display: grid; grid-template-columns: 48px minmax(260px, 1fr) 150px 120px; align-items: center; gap: 14px; }
.ranking-table-head { padding: 0 12px 9px; color: var(--text-secondary); font-size: 11px; }
.ranking-table-head span:last-child { text-align: right; }
.ranking-table article { min-height: 62px; padding: 9px 12px; border-top: 1px solid var(--border-light); }
.rank-number { display: grid; width: 26px; height: 26px; place-items: center; border-radius: 7px; background: var(--bg-secondary); color: var(--text-secondary); font-size: 11px; }
.rank-number.top { background: var(--el-color-primary-light-9); color: var(--el-color-primary); font-weight: 700; }
.rank-name { display: grid; gap: 3px; min-width: 0; }
.rank-resource { display: flex; align-items: center; gap: 8px; min-width: 0; }
.rank-resource-icon { display: grid; flex: 0 0 25px; width: 25px; height: 25px; place-items: center; border: 1px solid var(--border-light); border-radius: 7px; background: var(--bg-secondary); }
.rank-resource-icon img, .rank-resource-icon :deep(svg) { width: 16px; height: 16px; }
.rank-name strong, .rank-name small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.rank-name strong { color: var(--text-primary); font-size: 13px; }
.rank-name small, .rank-meta { color: var(--text-secondary); font-size: 11px; }
.rank-track { height: 3px; margin-top: 4px; overflow: hidden; border-radius: 999px; background: var(--bg-secondary); }
.rank-track i { display: block; height: 100%; border-radius: inherit; background: var(--el-color-primary); }
.rank-calls { display: grid; color: var(--text-primary); font-size: 15px; text-align: right; }
.rank-calls small { margin-top: 2px; color: var(--text-secondary); font-size: 10px; font-weight: 400; }
.ranking-footer { display: flex; align-items: center; justify-content: space-between; min-height: 48px; margin-top: 8px; padding-top: 10px; border-top: 1px solid var(--border-light); color: var(--text-secondary); font-size: 12px; }
@media (max-width: 1050px) { .usage-summary-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .ranking-table-head, .ranking-table article { grid-template-columns: 40px minmax(220px, 1fr) 110px 100px; } }
@media (max-width: 720px) { .usage-toolbar, .section-heading { flex-direction: column; } .usage-summary-grid { grid-template-columns: 1fr; } .ranking-table { overflow-x: auto; } .ranking-table-head, .ranking-table article { min-width: 650px; } }

.usage-panel { gap:22px; }
.usage-toolbar { padding:4px 0 2px; }
.usage-toolbar h4 { font-size:22px; letter-spacing:-.5px; }
.usage-summary-grid { gap:16px; }
.usage-summary-grid article { position:relative; padding:24px; gap:12px; border-radius:14px; background:var(--bg-primary); overflow:hidden; }
.usage-summary-grid article::before { content:''; position:absolute; inset:0 auto 0 0; width:3px; background:#5985ed; }
.usage-summary-grid article:nth-child(2)::before { background:#38aa99; }
.usage-summary-grid article:nth-child(3)::before { background:#9780d5; }
.usage-summary-grid article:nth-child(4)::before { background:#dd9670; }
.usage-summary-grid strong { font-size:34px; letter-spacing:-1px; font-variant-numeric:tabular-nums; font-weight:650; }
.usage-summary-grid small { font-size:11px; }
.usage-card { padding:24px; border-radius:16px; background:var(--bg-primary); }
.section-heading h5 { font-size:17px; letter-spacing:-.25px; }
.usage-echart { height:330px; margin-top:20px; }
.usage-collected { margin:-12px 0 0; color:var(--text-secondary); font-size:11px; text-align:right; }
.ranking-table article { min-height:76px; }
.rank-track { height:5px; max-width:320px; margin-top:8px; }
.rank-number { width:30px; height:30px; border-radius:50%; }
.rank-name strong { font-size:14px; }
.rank-resource-icon { width:30px; height:30px; flex-basis:30px; }
@media (max-width:720px) { .usage-summary-grid { grid-template-columns:repeat(2,minmax(0,1fr)); gap:10px; } .usage-summary-grid article { padding:16px; } .usage-summary-grid strong { font-size:26px; } .usage-card { padding:16px; } }
</style>
