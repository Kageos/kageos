<template>
  <div
    v-if="prd"
    ref="previewRoot"
    class="prd-preview"
    @pointerdown.stop
    @pointerup.stop
    @mousedown.stop
    @mouseup.stop
    @click.stop
  >
    <header class="prd-head">
      <div class="prd-head-main">
        <img src="/service-tree/custom-folder.svg" alt="" class="prd-node-img" />
        <div class="prd-head-text">
          <div class="prd-title">{{ display(project.name) }}</div>
          <div class="prd-subtitle">
            <span>{{ t('prdPreview.createNewDirectory') }}</span>
            <span class="prd-dot">·</span>
            <code>{{ display(project.code) }}</code>
          </div>
        </div>
      </div>
      <div class="prd-directory-badge is-new">
        <el-icon :size="12"><Plus /></el-icon>
        <span>{{ t('prdPreview.newDirectory') }}</span>
      </div>
    </header>

    <div class="prd-directory-strip">
      <div class="prd-directory-item">
        <span>{{ t('prdPreview.directoryDescription') }}</span>
        <strong>{{ display(project.summary) }}</strong>
      </div>
    </div>

    <section v-if="activeView" :class="['prd-stage', `is-${fnType(activeView.fn)}`]">
      <div class="prd-stage-head">
        <div class="prd-stage-title">
          <span class="prd-step-badge">{{ formatOrder(activeView.order) }}</span>
          <component
            :is="activeView.iconComponent"
            v-if="activeView.iconComponent"
            class="prd-node-component"
            :size="16"
          />
          <img v-else :src="activeView.iconSrc" alt="" class="prd-node-img" />
          <div>
            <div class="prd-stage-name">{{ activeView.title }}</div>
            <div class="prd-stage-route">{{ activeView.subtitle }}</div>
          </div>
        </div>
        <el-tag size="small" effect="plain">{{ activeView.typeLabel }}</el-tag>
      </div>

      <div :class="['prd-runtime', `is-${fnType(activeView.fn)}`]">
        <div v-if="runtimeNote(activeView.fn)" class="prd-runtime-note">
          {{ runtimeNote(activeView.fn) }}
        </div>

        <template v-if="fnType(activeView.fn) === 'table'">
          <div v-if="tableRequestFields(activeView.fn).length" class="prd-panel prd-panel-search">
            <div class="prd-panel-title">
              <el-icon :size="14"><Search /></el-icon>
              <span>{{ t('prdPreview.searchFields') }}</span>
            </div>
            <div class="prd-field-grid is-compact is-search">
              <div
                v-for="(field, index) in tableRequestFields(activeView.fn)"
                :key="fieldKey(field, index)"
                class="prd-field-card is-search-field"
              >
                <div class="prd-field-head">
                  <span class="prd-field-name">{{ field.name }}</span>
                  <span v-if="field.required" class="prd-required">{{ t('prdPreview.required') }}</span>
                </div>
                <div :class="['prd-fake-control', fieldKindClass(field)]">
                  <span>{{ fieldPreviewText(field) }}</span>
                  <span v-if="fieldOptions(field).length" class="prd-caret">⌄</span>
                </div>
                <div class="prd-field-meta">
                  <span>{{ field.type }}</span>
                  <span v-if="field.desc">{{ field.desc }}</span>
                </div>
                <div v-if="fieldOptions(field).length" class="prd-option-row">
                  <span v-for="option in fieldOptions(field)" :key="option">{{ option }}</span>
                </div>
              </div>
            </div>
          </div>

          <div class="prd-table-toolbar">
            <div class="prd-toolbar-group">
              <span
                v-for="operation in tableOperations(activeView.fn)"
                :key="operation.key"
                :class="['prd-operation-chip', { 'is-disabled': tableReadonly(activeView.fn) && operation.key !== 'list' }]"
              >
                {{ operation.label }}
              </span>
            </div>
            <div v-if="tableReadonly(activeView.fn)" class="prd-readonly-mark">{{ t('prdPreview.readonly') }}</div>
          </div>

          <div class="prd-table-scroll">
            <table class="prd-basic-table">
              <thead>
                <tr>
                  <th v-for="column in tableColumns(activeView.fn)" :key="column">{{ column }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(row, rowIndex) in tableRows(activeView.fn)" :key="rowIndex">
                  <td v-for="column in tableColumns(activeView.fn)" :key="column">
                    {{ display(rowCell(row, column)) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="prd-table-footer">{{ t('prdPreview.sampleRows', { count: Math.max(tableRows(activeView.fn).length, 1) }) }}</div>
        </template>

        <template v-else-if="fnType(activeView.fn) === 'form'">
          <div class="prd-panel prd-form-preview-panel">
            <div class="prd-panel-title">
              <component :is="FormIcon" class="prd-panel-icon" :size="14" />
              <span>{{ t('prdPreview.requestFields') }}</span>
            </div>
            <div class="prd-form-preview">
              <div
                v-for="(field, index) in formRequestFields(activeView.fn)"
                :key="fieldKey(field, index)"
                class="prd-form-linked-row"
              >
                <div class="prd-form-field-row">
                  <label class="prd-form-label">
                    <span>{{ field.name }}</span>
                    <i v-if="field.required">*</i>
                  </label>
                  <div class="prd-form-control-wrap">
                    <div :class="['prd-fake-control', 'is-form-control', fieldKindClass(field)]">
                      <span>{{ fieldPreviewText(field) }}</span>
                      <span v-if="fieldOptions(field).length" class="prd-caret">⌄</span>
                    </div>
                    <div v-if="fieldOptions(field).length" class="prd-option-row is-form-options">
                      <span v-for="option in fieldOptions(field)" :key="option">{{ option }}</span>
                    </div>
                  </div>
                </div>

                <div class="prd-form-arrow" aria-hidden="true">→</div>

                <div :class="['prd-form-inspector-item', { 'is-primary': index === 0 }]">
                  <div class="prd-form-inspector-title">
                    <span>{{ field.name }}</span>
                    <i>{{ field.required ? t('prdPreview.required') : t('prdPreview.optional') }}</i>
                  </div>
                  <div class="prd-form-inspector-meta">
                    <code>{{ field.type }}</code>
                    <span v-if="field.desc">{{ field.desc }}</span>
                    <span v-else>{{ fieldOptions(field).length ? t('prdPreview.optionList', { options: fieldOptions(field).join('、') }) : t('prdPreview.fillByControlType') }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-if="formResponseFields(activeView.fn).length" class="prd-panel">
            <div class="prd-panel-title">
              <span>{{ t('prdPreview.responsePreview') }}</span>
            </div>
            <div class="prd-response-grid is-form-response">
              <div
                v-for="(field, index) in formResponseFields(activeView.fn)"
                :key="fieldKey(field, index)"
                class="prd-response-item is-form-response-row"
              >
                <span>{{ field.name }}</span>
                <strong>{{ display(field.example || field.type) }}</strong>
                <em v-if="field.desc">{{ field.desc }}</em>
              </div>
            </div>
          </div>
        </template>

        <template v-else-if="fnType(activeView.fn) === 'chart'">
          <div v-if="chartFilterFields(activeView.fn).length" class="prd-panel prd-panel-search">
            <div class="prd-panel-title">
              <el-icon :size="14"><Search /></el-icon>
              <span>{{ t('prdPreview.filterFields') }}</span>
            </div>
            <div class="prd-field-grid is-compact is-search">
              <div
                v-for="(field, index) in chartFilterFields(activeView.fn)"
                :key="fieldKey(field, index)"
                class="prd-field-card is-search-field"
              >
                <div class="prd-field-head">
                  <span class="prd-field-name">{{ field.name }}</span>
                  <span v-if="field.required" class="prd-required">{{ t('prdPreview.required') }}</span>
                </div>
                <div :class="['prd-fake-control', fieldKindClass(field)]">
                  <span>{{ fieldPreviewText(field) }}</span>
                </div>
                <div class="prd-field-meta">
                  <span>{{ field.type }}</span>
                  <span v-if="field.desc">{{ field.desc }}</span>
                </div>
              </div>
            </div>
          </div>

          <PrdChartPreview
            :key="`chart-${activeView.key}`"
            class="prd-chart-runtime"
            active
            :title="display(activeView.fn.title)"
            :chart-type="display(activeView.fn.chart?.chart_type)"
            :dimension="display(activeView.fn.chart?.dimension)"
            :metrics="chartMetrics(activeView.fn)"
            :preview-data="chartPreviewData(activeView.fn)"
          />

          <div v-if="chartSummaryFields(activeView.fn).length" class="prd-response-grid is-chart">
            <div
              v-for="(field, index) in chartSummaryFields(activeView.fn)"
              :key="fieldKey(field, index)"
              class="prd-response-item"
            >
              <span>{{ field.name }}</span>
              <strong>{{ display(field.example || field.type) }}</strong>
              <em v-if="field.desc">{{ field.desc }}</em>
            </div>
          </div>
        </template>

        <el-empty v-else :description="t('prdPreview.emptyPreview')" />
      </div>

      <div
        v-if="functionCards.length > 1"
        class="prd-slide-nav"
        role="tablist"
        :aria-label="t('prdPreview.previewSwitchAria')"
        @pointerdown.stop
        @mousedown.stop
        @mouseup.stop
        @click.stop
      >
        <button
          v-for="slide in functionCards"
          :key="slide.key"
          type="button"
          :class="['prd-slide-thumb', { 'is-active': activeView.key === slide.key }]"
          :data-prd-key="slide.key"
          :data-prd-active="activeView.key === slide.key ? 'true' : 'false'"
          role="tab"
          :aria-selected="activeView.key === slide.key"
          :tabindex="activeView.key === slide.key ? 0 : -1"
          @pointerup.stop.prevent="selectFunction(slide.key, $event)"
          @click.stop="selectFunction(slide.key, $event)"
          @keydown.enter.prevent.stop="selectFunction(slide.key, $event)"
          @keydown.space.prevent.stop="selectFunction(slide.key, $event)"
        >
          <span class="prd-slide-step">{{ formatOrder(slide.order) }}</span>
          <component
            :is="slide.iconComponent"
            v-if="slide.iconComponent"
            class="prd-node-component"
            :size="16"
          />
          <img v-else :src="slide.iconSrc" alt="" class="prd-node-img" />
          <span class="prd-slide-thumb-text">{{ slide.shortTitle }}</span>
        </button>
      </div>
    </section>

    <section v-else class="prd-stage prd-stage-empty">
      <el-empty :description="t('prdPreview.emptyPreview')" />
    </section>

    <div v-if="props.showConfirmation && confirmationQuestion" class="prd-confirmation">
      <div class="prd-confirmation-text">{{ confirmationQuestion }}</div>
      <div class="prd-remark-box">
        <textarea
          v-model="remark"
          class="prd-confirmation-remark"
          rows="2"
          :disabled="confirmDisabled || submitted"
          :placeholder="t('prdPreview.remarkPlaceholder')"
        />
      </div>
      <div class="prd-confirmation-actions">
        <el-button
          type="primary"
          size="small"
          :disabled="confirmDisabled || submitted"
          @click="confirmPrd"
        >
          {{ submitted ? t('prdPreview.confirmSubmitted') : t('prdPreview.confirmPrd') }}
        </el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import type { Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, Search } from '@element-plus/icons-vue'
import PrdChartPreview from './PrdChartPreview.vue'
import TableIcon from '@/architecture/presentation/shared/components/icons/TableIcon.vue'
import FormIcon from '@/architecture/presentation/shared/components/icons/FormIcon.vue'
import ChartIcon from '@/architecture/presentation/shared/components/icons/ChartIcon.vue'

type PreviewRow = Record<string, unknown>
type PrdResourceType = 'table' | 'form' | 'chart'
type PrdFieldKind = 'text' | 'textarea' | 'select' | 'multiselect' | 'datetime' | 'number' | 'files' | 'switch' | 'radio' | 'checkbox' | 'table'

interface PrdProject {
  name?: string
  code?: string
  summary?: string
}

interface PrdField {
  name?: string
  widget?: string
  required?: boolean
  desc?: string
  hide?: string
}

interface PreviewField {
  name: string
  type: string
  kind: PrdFieldKind
  required: boolean
  desc: string
  example?: unknown
  options: string[]
}

interface PrdTable {
  name?: string
  title?: string
  desc?: string
  fields?: PrdField[]
  search_fields?: PrdField[]
  handlers?: string[]
  examples?: PreviewRow[]
}

interface PrdForm {
  name?: string
  desc?: string
  target_table?: string
  request_fields?: PrdField[]
  response_fields?: PrdField[]
  example?: {
    request?: PreviewRow
    response?: PreviewRow
  }
}

interface PrdChart {
  name?: string
  desc?: string
  source_table?: string
  chart_type?: string
  dimension?: string
  metrics?: string[]
  filters?: PrdField[]
  examples?: PreviewRow[]
}

interface PrdData {
  kind?: string
  schema_version?: string
  project?: PrdProject
  tables?: PrdTable[]
  forms?: PrdForm[]
  charts?: PrdChart[]
  rules?: string[]
}

interface PreviewFunction {
  type: PrdResourceType
  title: string
  subtitle: string
  description: string
  table?: PrdTable
  form?: PrdForm
  chart?: PrdChart
}

interface PrdFunctionCard {
  key: string
  order: number
  title: string
  shortTitle: string
  subtitle: string
  typeLabel: string
  fn: PreviewFunction
  iconSrc?: string
  iconComponent?: Component
}

interface PrdTableOperation {
  key: 'list' | 'create' | 'edit' | 'delete'
  label: string
}

const props = withDefaults(defineProps<{
  data: unknown
  confirmDisabled?: boolean
  showConfirmation?: boolean
}>(), {
  confirmDisabled: false,
  showConfirmation: false,
})

const emit = defineEmits<{
  (event: 'confirm', payload: { remark: string; prd: PrdData | null }): void
}>()

const { t } = useI18n()
const previewRoot = ref<HTMLElement | null>(null)
const activeFunctionKey = ref('')
const remark = ref('')
const submitted = ref(false)

const prd = computed<PrdData | null>(() => {
  const parsed = parseMaybeJSON(props.data)
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null
  return parsed as PrdData
})

const project = computed<PrdProject>(() => prd.value?.project || {})
const tablesList = computed<PrdTable[]>(() => Array.isArray(prd.value?.tables) ? prd.value?.tables || [] : [])
const formsList = computed<PrdForm[]>(() => Array.isArray(prd.value?.forms) ? prd.value?.forms || [] : [])
const chartsList = computed<PrdChart[]>(() => Array.isArray(prd.value?.charts) ? prd.value?.charts || [] : [])
const confirmationQuestion = computed(() => {
  const refs = functionCards.value.map(item => item.title).filter(item => item !== '—')
  return t('prdPreview.confirmationQuestion', {
    name: display(project.value.name),
    functions: refs.join('、') || t('prdPreview.aboveFunctions'),
  })
})

const functionCards = computed<PrdFunctionCard[]>(() => {
  return derivedPreviewFunctions.value
    .map((fn, index) => {
      const type = fnType(fn)
      const key = `fn-${index}-${type}-${fn.title}`
      return {
        key,
        order: index + 1,
        title: fn.title,
        shortTitle: fn.title,
        subtitle: fn.subtitle,
        typeLabel: typeLabel(type),
        fn,
        iconSrc: type === 'form' ? '/service-tree/编辑.svg' : undefined,
        iconComponent: type === 'table' ? TableIcon : type === 'chart' ? ChartIcon : undefined,
      }
    })
})

const derivedPreviewFunctions = computed<PreviewFunction[]>(() => {
  const targetTables = new Set(
    formsList.value
      .map(form => String(form.target_table || '').trim())
      .filter(Boolean),
  )
  const isFormTargetReadonlyTable = (table: PrdTable) => {
    const name = String(table.name || '').trim()
    const handlers = Array.isArray(table.handlers) ? table.handlers : []
    return targetTables.has(name) && handlers.length === 0
  }
  return [
    ...tablesList.value.filter(table => !isFormTargetReadonlyTable(table)).map(previewFunctionForTable),
    ...formsList.value.map(previewFunctionForForm),
    ...tablesList.value.filter(isFormTargetReadonlyTable).map(previewFunctionForTable),
    ...chartsList.value.map(previewFunctionForChart),
  ]
})

const activeView = computed<PrdFunctionCard | null>(() => {
  return functionCards.value.find(item => item.key === activeFunctionKey.value) || functionCards.value[0] || null
})

watch(functionCards, (items) => {
  if (items.length === 0) {
    activeFunctionKey.value = ''
    return
  }
  if (!activeFunctionKey.value || !items.some(item => item.key === activeFunctionKey.value)) {
    activeFunctionKey.value = items[0]?.key || ''
  }
}, { immediate: true })

watch(activeFunctionKey, () => {
  scheduleFunctionNavDOMSync()
})

function parseMaybeJSON(value: unknown): unknown {
  if (typeof value !== 'string') return value
  const trimmed = value.trim()
  if (!trimmed) return null
  try {
    return JSON.parse(trimmed)
  } catch {
    return value
  }
}

function display(value: unknown): string {
  if (value == null || value === '') return '—'
  if (Array.isArray(value)) return value.length ? value.map(display).join('、') : '—'
  if (typeof value === 'boolean') return value ? t('common.yes') : t('common.no')
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

function previewFunctionForTable(table: PrdTable): PreviewFunction {
  return {
    type: 'table',
    title: display(table.title || table.name),
    subtitle: display(table.name),
    description: display(table.desc),
    table,
  }
}

function previewFunctionForForm(form: PrdForm): PreviewFunction {
  return {
    type: 'form',
    title: display(form.name),
    subtitle: form.target_table ? t('prdPreview.writeTo', { target: form.target_table }) : t('prdPreview.standaloneSubmitEntry'),
    description: display(form.desc),
    form,
  }
}

function previewFunctionForChart(chart: PrdChart): PreviewFunction {
  return {
    type: 'chart',
    title: display(chart.name),
    subtitle: chart.source_table ? t('prdPreview.chartFrom', { source: chart.source_table }) : t('prdPreview.chart'),
    description: display(chart.desc),
    chart,
  }
}

function fnType(fn: PreviewFunction): string {
  return fn.type
}

function typeLabel(type: unknown): string {
  const normalized = String(type || '').toLowerCase()
  if (normalized === 'table') return 'Table'
  if (normalized === 'form') return 'Form'
  if (normalized === 'chart') return 'Chart'
  return display(type)
}

function formatOrder(order: number): string {
  return order < 10 ? `0${order}` : String(order)
}

function runtimeNote(fn: PreviewFunction): string {
  return [
    fn.description,
    fn.form?.target_table ? t('prdPreview.targetTable', { table: fn.form.target_table }) : '',
    fn.chart?.source_table ? t('prdPreview.sourceTable', { table: fn.chart.source_table }) : '',
  ]
    .map(item => String(item || '').trim())
    .filter(Boolean)
    .join(' ')
}

function tableReadonly(_fn: PreviewFunction): boolean {
  return false
}

function tableOperations(fn: PreviewFunction): PrdTableOperation[] {
  const handlers = Array.isArray(fn.table?.handlers) ? fn.table.handlers : []
  const operations: PrdTableOperation[] = [{ key: 'list', label: t('prdPreview.listQuery') }]
  if (handlers.includes('OnTableAddRow')) operations.push({ key: 'create', label: t('common.create') })
  if (handlers.includes('OnTableUpdateRow')) operations.push({ key: 'edit', label: t('common.edit') })
  if (handlers.includes('OnTableDeleteRow')) operations.push({ key: 'delete', label: t('common.delete') })
  return operations
}

function tableColumns(fn: PreviewFunction): string[] {
  const columns = Array.isArray(fn.table?.fields) ? fn.table.fields.map(field => display(field.name)).filter(item => item !== '—') : []
  if (columns.length > 0) return columns
  const firstRow = tableRows(fn)[0]
  return firstRow ? Object.keys(firstRow) : []
}

function tableRows(fn: PreviewFunction): PreviewRow[] {
  return Array.isArray(fn.table?.examples) ? fn.table.examples : []
}

function rowCell(row: PreviewRow, column: string): unknown {
  return row[column]
}

function tableRequestFields(fn: PreviewFunction): PreviewField[] {
  const searchFields = Array.isArray(fn.table?.search_fields) ? fn.table.search_fields : []
  return searchFields.map(field => normalizeField(field, t('prdPreview.searchFields')))
}

function formRequestFields(fn: PreviewFunction): PreviewField[] {
  const requestFields = Array.isArray(fn.form?.request_fields) ? fn.form.request_fields : []
  return requestFields.map(field => normalizeField(field, '', fn.form?.example?.request?.[display(field.name)]))
}

function formResponseFields(fn: PreviewFunction): PreviewField[] {
  const responseFields = Array.isArray(fn.form?.response_fields) ? fn.form.response_fields : []
  return responseFields.map(field => normalizeField(field, '', fn.form?.example?.response?.[display(field.name)]))
}

function chartFilterFields(fn: PreviewFunction): PreviewField[] {
  const filters = Array.isArray(fn.chart?.filters) ? fn.chart.filters : []
  return filters.map(field => normalizeField(field, t('prdPreview.chartQueryFields')))
}

function chartMetrics(fn: PreviewFunction): string[] {
  return Array.isArray(fn.chart?.metrics) && fn.chart.metrics.length > 0 ? fn.chart.metrics : ['数量']
}

function chartPreviewData(fn: PreviewFunction): PreviewRow[] {
  const rows = Array.isArray(fn.chart?.examples) ? fn.chart.examples : []
  return rows.map(row => normalizeChartExampleRow(fn, row))
}

function normalizeChartExampleRow(fn: PreviewFunction, row: PreviewRow): PreviewRow {
  const metricsValue = row.metrics
  if (!metricsValue || typeof metricsValue !== 'object' || Array.isArray(metricsValue)) return row

  const normalized: PreviewRow = {}
  const dimension = display(fn.chart?.dimension)
  if (dimension !== '—') normalized[dimension] = row.dimension

  Object.entries(metricsValue as PreviewRow).forEach(([key, value]) => {
    normalized[key] = value
  })
  Object.entries(row).forEach(([key, value]) => {
    if (key !== 'dimension' && key !== 'metrics') normalized[key] = value
  })
  return normalized
}

function chartSummaryFields(_fn: PreviewFunction): PreviewField[] {
  return []
}

function normalizeField(input: PrdField, fallbackDesc = '', example?: unknown): PreviewField {
  const name = display(input.name)
  const kind = normalizeFieldKind(input.widget || inferFieldKind(`${name} ${input.desc || ''}`))
  const type = fieldTypeLabel(kind)
  const desc = display(input.desc || fallbackDesc)
  return {
    name,
    type,
    kind,
    required: Boolean(input.required),
    desc: desc === '—' ? '' : desc,
    example,
    options: [],
  }
}

function fieldOptions(field: PreviewField): string[] {
  return field.options.slice(0, 8)
}

function fieldPreviewText(field: PreviewField): string {
  if (hasValue(field.example)) return display(field.example)
  const options = fieldOptions(field)
  if (options.length > 0) return options[0] || t('prdPreview.selectField', { field: field.name })
  if (field.kind === 'datetime') return '2025-01-20 11:30'
  if (field.kind === 'number') return '0'
  if (field.kind === 'table') return t('prdPreview.tableDetails')
  if (field.kind === 'files') return t('prdPreview.clickUpload')
  if (field.kind === 'textarea') return t('prdPreview.inputField', { field: field.name })
  if (field.kind === 'switch') return t('common.yes')
  return t('prdPreview.inputField', { field: field.name })
}

function fieldKindClass(field: PreviewField): string {
  if (['select', 'multiselect', 'radio', 'checkbox'].includes(field.kind) || fieldOptions(field).length > 0) return 'is-select'
  if (field.kind === 'datetime') return 'is-datetime'
  if (field.kind === 'number') return 'is-number'
  if (field.kind === 'table') return 'is-table'
  if (field.kind === 'files') return 'is-files'
  if (field.kind === 'textarea') return 'is-textarea'
  return 'is-input'
}

function fieldKey(field: PreviewField, index: number): string {
  return `${index}-${field.name}-${field.type}`
}

function normalizeFieldKind(type: unknown): PrdFieldKind {
  const normalized = String(type || '').trim()
  const lower = normalized.toLowerCase()
  if (lower === 'text_area' || lower === 'textarea') return 'textarea'
  if (lower === 'select') return 'select'
  if (lower === 'multiselect') return 'multiselect'
  if (lower === 'datetime' || lower === 'date') return 'datetime'
  if (lower === 'integer' || lower === 'number' || lower === 'float' || lower === 'int') return 'number'
  if (lower === 'files') return 'files'
  if (lower === 'switch') return 'switch'
  if (lower === 'radio') return 'radio'
  if (lower === 'checkbox') return 'checkbox'
  if (lower === 'table') return 'table'
  return 'text'
}

function inferFieldKind(text: string): PrdFieldKind {
  if (/日期|时间|date|time/i.test(text)) return 'datetime'
  if (/数量|金额|价格|分数|比例|占比|率|integer|number|float/i.test(text)) return 'number'
  if (/状态|分类|类型|选择|下拉|人员|部门|select/i.test(text)) return 'select'
  if (/文件|附件|上传|files/i.test(text)) return 'files'
  if (/是否|启用|开关|switch/i.test(text)) return 'switch'
  return 'text'
}

function fieldTypeLabel(kind: PrdFieldKind): string {
  return t(`prdPreview.fieldTypes.${kind}`)
}

function hasValue(value: unknown): boolean {
  return value !== undefined && value !== null && value !== ''
}

function selectFunction(key: string, event?: Event) {
  if (!functionCards.value.some(item => item.key === key)) return
  const source = event?.currentTarget instanceof HTMLElement ? event.currentTarget : undefined
  activeFunctionKey.value = key
  syncFunctionNavDOM(key, source)
  scheduleFunctionNavDOMSync(key, source)
}

function scheduleFunctionNavDOMSync(activeKey = activeView.value?.key || '', source?: HTMLElement) {
  void nextTick(() => {
    syncFunctionNavDOM(activeKey, source)
    if (typeof window === 'undefined') return
    window.setTimeout(() => syncFunctionNavDOM(activeKey, source), 0)
    window.requestAnimationFrame?.(() => syncFunctionNavDOM(activeKey, source))
  })
}

function syncFunctionNavDOM(activeKey = activeView.value?.key || '', source?: HTMLElement) {
  const root = source?.closest('.prd-slide-nav') || previewRoot.value
  if (!root) return
  root.querySelectorAll<HTMLElement>('.prd-slide-thumb').forEach((button) => {
    const active = button.dataset.prdKey === activeKey
    button.classList.toggle('is-active', active)
    button.dataset.prdActive = active ? 'true' : 'false'
    button.setAttribute('aria-selected', active ? 'true' : 'false')
    button.tabIndex = active ? 0 : -1
  })
}

function confirmPrd() {
  if (props.confirmDisabled || submitted.value) return
  submitted.value = true
  emit('confirm', {
    remark: remark.value.trim(),
    prd: prd.value,
  })
}
</script>

<style scoped lang="scss">
.prd-preview {
  --prd-bg: #101827;
  --prd-surface: #172235;
  --prd-surface-strong: #1d2a3f;
  --prd-control-bg: #0f1724;
  --prd-muted-bg: #26364f;
  --prd-primary-bg: rgb(59 130 246 / 18%);
  --prd-danger-bg: rgb(248 113 113 / 16%);
  --prd-warning-bg: rgb(245 158 11 / 16%);
  --prd-border: rgb(148 163 184 / 28%);
  --prd-border-soft: rgb(148 163 184 / 18%);
  --prd-shadow: 0 16px 38px rgb(2 6 23 / 22%);
  --el-text-color-primary: #e8eef7;
  --el-text-color-regular: #cbd5e1;
  --el-text-color-secondary: #94a3b8;
  --el-text-color-placeholder: #64748b;
  --el-border-color: var(--prd-border);
  --el-border-color-light: var(--prd-border);
  --el-border-color-lighter: var(--prd-border-soft);
  --el-border-color-extra-light: rgb(148 163 184 / 12%);
  --el-fill-color-blank: var(--prd-surface);
  --el-fill-color-extra-light: var(--prd-bg);
  --el-fill-color-light: var(--prd-surface-strong);
  --el-fill-color: var(--prd-muted-bg);

  width: 100%;
  max-width: none;
  overflow: hidden;
  border: 1px solid var(--prd-border);
  border-radius: 10px;
  background: var(--prd-bg);
  color: var(--el-text-color-primary);
  box-shadow: var(--prd-shadow);
}

.prd-preview :deep(.el-tag) {
  border-color: var(--prd-border);
  background: var(--prd-muted-bg);
  color: var(--el-text-color-regular);
}

.prd-preview :deep(.el-empty__description) {
  color: var(--el-text-color-secondary);
}

.prd-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--prd-border-soft);
  background: var(--prd-surface);
}

.prd-head-main,
.prd-stage-title,
.prd-panel-title {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.prd-node-img,
.prd-node-component {
  width: 22px;
  height: 22px;
  flex: 0 0 auto;
}

.prd-step-badge,
.prd-slide-step {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  border: 1px solid rgb(59 130 246 / 35%);
  background: var(--prd-primary-bg);
  color: var(--el-color-primary);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-weight: 800;
  line-height: 1;
}

.prd-step-badge {
  width: 26px;
  height: 26px;
  border-radius: 8px;
  font-size: 12px;
}

.prd-slide-step {
  width: 20px;
  height: 20px;
  border-radius: 6px;
  font-size: 10px;
}

.prd-head-text,
.prd-stage-title > div {
  min-width: 0;
}

.prd-title,
.prd-stage-name {
  overflow: hidden;
  font-size: 14px;
  font-weight: 700;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prd-subtitle,
.prd-stage-route {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  overflow: hidden;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prd-subtitle code {
  padding: 1px 5px;
  border-radius: 5px;
  background: var(--prd-muted-bg);
  color: var(--el-text-color-regular);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}

.prd-dot {
  color: var(--el-text-color-placeholder);
}

.prd-directory-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex: 0 0 auto;
  padding: 4px 8px;
  border: 1px solid var(--prd-border);
  border-radius: 999px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1;
}

.prd-directory-badge.is-new {
  border-color: var(--el-color-primary-light-5);
  background: var(--prd-primary-bg);
  color: var(--el-color-primary);
}

.prd-directory-strip {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 1px;
  border-bottom: 1px solid var(--prd-border-soft);
  background: var(--prd-border-soft);
}

.prd-directory-item {
  min-width: 0;
  padding: 10px 14px;
  background: var(--prd-surface-strong);
}

.prd-directory-item span {
  display: block;
  margin-bottom: 4px;
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

.prd-directory-item strong {
  display: block;
  overflow: hidden;
  color: var(--el-text-color-primary);
  font-size: 12px;
  font-weight: 600;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prd-stage {
  padding: 14px;
  background: var(--prd-bg);
}

.prd-stage-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.prd-runtime {
  display: flex;
  min-height: 330px;
  flex-direction: column;
  gap: 12px;
}

.prd-runtime-note {
  padding: 9px 11px;
  border: 1px solid var(--prd-border-soft);
  border-radius: 8px;
  background: var(--prd-surface);
  color: var(--el-text-color-secondary);
  font-size: 12px;
  line-height: 1.45;
}

.prd-panel {
  border: 1px solid var(--prd-border-soft);
  border-radius: 8px;
  background: var(--prd-surface);
}

.prd-panel-search {
  border-color: rgb(148 163 184 / 10%);
  background: rgb(15 23 36 / 30%);
}

.prd-panel-title {
  padding: 10px 12px 0;
  color: var(--el-text-color-primary);
  font-size: 12px;
  font-weight: 700;
}

.prd-panel-search .prd-panel-title {
  padding: 6px 10px 0;
  color: var(--el-text-color-secondary);
  font-size: 11px;
  font-weight: 600;
}

.prd-panel-icon {
  width: 14px;
  height: 14px;
}

.prd-field-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
  gap: 10px;
  padding: 12px;
}

.prd-field-grid.is-compact {
  grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
}

.prd-field-grid.is-search {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 7px 10px 9px;
}

.prd-field-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
}

.prd-form-preview {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
}

.prd-form-linked-row {
  display: grid;
  grid-template-columns: minmax(320px, 1fr) 28px minmax(260px, 0.72fr);
  align-items: stretch;
  gap: 10px;
}

.prd-form-field-row {
  display: grid;
  grid-template-columns: minmax(110px, 156px) minmax(0, 1fr);
  align-items: start;
  gap: 12px;
  min-width: 0;
  padding: 10px;
  border: 1px solid var(--prd-border-soft);
  border-radius: 8px;
  background: var(--prd-surface-strong);
}

.prd-form-label {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
  min-height: 32px;
  color: var(--el-text-color-regular);
  font-size: 12px;
  font-weight: 600;
  line-height: 1.35;
  text-align: right;
}

.prd-form-label span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prd-form-label i {
  color: var(--el-color-danger);
  font-style: normal;
}

.prd-form-control-wrap {
  min-width: 0;
}

.prd-fake-control.is-form-control {
  min-height: 32px;
  border-color: rgb(148 163 184 / 22%);
  background: rgb(15 23 36 / 78%);
}

.prd-fake-control.is-form-control.is-textarea {
  min-height: 58px;
}

.prd-fake-control.is-form-control.is-files {
  min-height: 38px;
}

.prd-option-row.is-form-options {
  margin-top: 6px;
}

.prd-form-arrow {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--el-color-primary);
  font-size: 16px;
  font-weight: 700;
  opacity: 0.78;
}

.prd-form-inspector-item {
  min-width: 0;
  padding: 10px 11px;
  border: 1px solid var(--prd-border-soft);
  border-radius: 8px;
  background: rgb(15 23 36 / 58%);
}

.prd-form-inspector-item.is-primary {
  border-color: rgb(59 130 246 / 26%);
  background: var(--prd-primary-bg);
}

.prd-form-inspector-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 5px;
}

.prd-form-inspector-title span {
  min-width: 0;
  overflow: hidden;
  color: var(--el-text-color-primary);
  font-size: 12px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prd-form-inspector-title i {
  flex: 0 0 auto;
  color: var(--el-text-color-secondary);
  font-size: 11px;
  font-style: normal;
}

.prd-form-inspector-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  color: var(--el-text-color-secondary);
  font-size: 11px;
  line-height: 1.45;
}

.prd-form-inspector-meta code {
  padding: 1px 5px;
  border-radius: 5px;
  background: var(--prd-control-bg);
  color: var(--el-color-primary);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}

.prd-field-card {
  min-width: 0;
  padding: 10px;
  border: 1px solid var(--prd-border-soft);
  border-radius: 8px;
  background: var(--prd-surface-strong);
}

.prd-field-card.is-search-field {
  display: flex;
  flex: 0 1 240px;
  align-items: center;
  gap: 6px;
  max-width: 320px;
  padding: 0;
  border: 0;
  background: transparent;
}

.prd-field-card.is-form-row {
  display: grid;
  grid-template-columns: minmax(120px, 180px) minmax(220px, 1fr) minmax(180px, 1fr);
  align-items: center;
  gap: 10px;
  padding: 9px 10px;
}

.prd-field-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 7px;
}

.prd-field-card.is-search-field .prd-field-head,
.prd-field-card.is-form-row .prd-field-head {
  margin-bottom: 0;
}

.prd-field-card.is-search-field .prd-field-head {
  gap: 6px;
}

.prd-field-card.is-search-field .prd-field-head {
  flex: 0 0 auto;
  max-width: 96px;
}

.prd-field-card.is-form-row .prd-field-head {
  justify-content: flex-start;
}

.prd-field-name {
  overflow: hidden;
  font-size: 12px;
  font-weight: 700;
  line-height: 1.3;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prd-field-card.is-search-field .prd-field-name {
  color: var(--el-text-color-secondary);
  font-size: 11px;
  font-weight: 600;
}

.prd-field-card.is-search-field .prd-field-name::after {
  content: ':';
}

.prd-field-card.is-search-field .prd-required {
  display: none;
}

.prd-required,
.prd-optional {
  flex: 0 0 auto;
  padding: 2px 5px;
  border-radius: 999px;
  font-size: 11px;
  line-height: 1;
}

.prd-required {
  background: var(--prd-danger-bg);
  color: var(--el-color-danger);
}

.prd-optional {
  background: var(--prd-muted-bg);
  color: var(--el-text-color-secondary);
}

.prd-fake-control {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  min-height: 32px;
  padding: 6px 9px;
  border: 1px solid var(--prd-border);
  border-radius: 6px;
  background: var(--prd-control-bg);
  color: var(--el-text-color-regular);
  font-size: 12px;
  line-height: 1.35;
}

.prd-field-card.is-search-field .prd-fake-control {
  flex: 1 1 auto;
  min-width: 92px;
  min-height: 24px;
  padding: 3px 7px;
  border-color: rgb(148 163 184 / 18%);
  border-radius: 5px;
  background: rgb(15 23 36 / 62%);
  font-size: 11px;
}

.prd-field-card.is-form-row .prd-fake-control {
  min-height: 30px;
}

.prd-field-card.is-form-row .prd-fake-control.is-textarea {
  min-height: 42px;
}

.prd-field-card.is-form-row .prd-fake-control.is-files {
  min-height: 34px;
}

.prd-fake-control span:first-child {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prd-fake-control.is-textarea {
  min-height: 56px;
  align-items: flex-start;
}

.prd-fake-control.is-table {
  border-style: dashed;
}

.prd-caret {
  flex: 0 0 auto;
  color: var(--el-text-color-secondary);
}

.prd-field-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-top: 7px;
  color: var(--el-text-color-secondary);
  font-size: 11px;
  line-height: 1.35;
}

.prd-field-card.is-search-field .prd-field-meta {
  display: none;
}

.prd-field-card.is-form-row .prd-field-meta {
  margin-top: 0;
}

.prd-field-meta span {
  overflow-wrap: anywhere;
}

.prd-field-meta span:first-child {
  color: var(--el-color-primary);
}

.prd-option-row {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  margin-top: 8px;
}

.prd-field-card.is-search-field .prd-option-row {
  display: none;
}

.prd-field-card.is-form-row .prd-option-row {
  grid-column: 2 / -1;
  margin-top: -2px;
}

.prd-option-row span {
  padding: 2px 6px;
  border-radius: 999px;
  background: var(--prd-control-bg);
  color: var(--el-text-color-regular);
  font-size: 11px;
}

.prd-table-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.prd-toolbar-group {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
}

.prd-operation-chip,
.prd-readonly-mark {
  display: inline-flex;
  align-items: center;
  min-height: 26px;
  padding: 4px 9px;
  border: 1px solid var(--prd-border);
  border-radius: 999px;
  background: var(--prd-control-bg);
  color: var(--el-text-color-regular);
  font-size: 12px;
}

.prd-operation-chip.is-disabled {
  opacity: 0.45;
}

.prd-readonly-mark {
  border-color: var(--el-color-warning-light-5);
  background: var(--prd-warning-bg);
  color: var(--el-color-warning-dark-2);
}

.prd-table-scroll {
  overflow: auto;
  border: 1px solid rgb(148 163 184 / 30%);
  border-radius: 8px;
  background: var(--prd-surface);
  box-shadow: 0 12px 26px rgb(2 6 23 / 14%);
}

.prd-basic-table {
  width: 100%;
  min-width: 620px;
  border-collapse: collapse;
  font-size: 12px;
}

.prd-basic-table th,
.prd-basic-table td {
  max-width: 220px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--prd-border-soft);
  overflow: hidden;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prd-basic-table th {
  background: var(--prd-surface-strong);
  color: var(--el-text-color-secondary);
  font-weight: 700;
}

.prd-basic-table tr:last-child td {
  border-bottom: 0;
}

.prd-table-footer {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  text-align: right;
}

.prd-response-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 10px;
  padding: 12px;
}

.prd-response-grid.is-chart {
  padding: 0;
}

.prd-response-grid.is-form-response {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
}

.prd-response-item {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
  padding: 10px;
  border: 1px solid var(--prd-border-soft);
  border-radius: 8px;
  background: var(--prd-surface);
}

.prd-response-item.is-form-response-row {
  display: grid;
  grid-template-columns: minmax(120px, 180px) minmax(180px, 1fr) minmax(180px, 1.2fr);
  align-items: center;
  gap: 10px;
  padding: 9px 10px;
  background: var(--prd-surface-strong);
}

.prd-response-item span {
  color: var(--el-text-color-secondary);
  font-size: 11px;
}

.prd-response-item strong {
  overflow: hidden;
  font-size: 15px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prd-response-item em {
  color: var(--el-text-color-secondary);
  font-size: 11px;
  font-style: normal;
  line-height: 1.35;
}

.prd-response-item.is-form-response-row em {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prd-chart-runtime {
  min-height: 320px;
}

.prd-slide-nav {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(94px, 1fr));
  gap: 8px;
  margin-top: 12px;
}

.prd-slide-thumb {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 0;
  min-height: 36px;
  padding: 7px 9px;
  border: 1px solid var(--prd-border);
  border-radius: 8px;
  background: var(--prd-surface);
  color: var(--el-text-color-regular);
  cursor: pointer;
}

.prd-slide-thumb.is-active {
  border-color: var(--el-color-primary);
  background: var(--prd-primary-bg);
  color: var(--el-color-primary);
}

.prd-slide-thumb-text {
  min-width: 0;
  overflow: hidden;
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prd-confirmation {
  display: grid;
  gap: 10px;
  padding: 14px;
  border-top: 1px solid var(--prd-border-soft);
  background: var(--prd-surface-strong);
}

.prd-confirmation-text {
  color: var(--el-text-color-regular);
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
}

.prd-remark-box {
  border: 1px solid var(--prd-border);
  border-radius: 8px;
  background: var(--prd-control-bg);
}

.prd-confirmation-remark {
  display: block;
  width: 100%;
  resize: vertical;
  border: 0;
  outline: none;
  padding: 9px 10px;
  background: transparent;
  color: var(--el-text-color-primary);
  font: inherit;
  font-size: 12px;
  line-height: 1.45;
}

.prd-confirmation-actions {
  display: flex;
  justify-content: flex-end;
}

@media (max-width: 860px) {
  .prd-form-linked-row {
    grid-template-columns: 1fr;
  }

  .prd-form-arrow {
    display: none;
  }

  .prd-field-card.is-form-row {
    grid-template-columns: minmax(120px, 160px) minmax(180px, 1fr);
  }

  .prd-field-card.is-form-row .prd-field-meta,
  .prd-field-card.is-form-row .prd-option-row {
    grid-column: 2 / -1;
  }

  .prd-response-item.is-form-response-row {
    grid-template-columns: minmax(120px, 160px) minmax(180px, 1fr);
  }

  .prd-response-item.is-form-response-row em {
    grid-column: 2 / -1;
  }
}

@media (max-width: 640px) {
  .prd-head,
  .prd-stage-head,
  .prd-table-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .prd-field-grid,
  .prd-field-grid.is-compact,
  .prd-response-grid {
    grid-template-columns: 1fr;
  }

  .prd-field-grid.is-search {
    display: grid;
  }

  .prd-field-card.is-search-field {
    flex-basis: 100%;
    max-width: none;
  }

  .prd-field-card.is-form-row {
    grid-template-columns: 1fr;
  }

  .prd-form-field-row {
    grid-template-columns: 1fr;
    gap: 6px;
  }

  .prd-form-label {
    justify-content: flex-start;
    min-height: auto;
    text-align: left;
  }

  .prd-field-card.is-form-row .prd-field-meta,
  .prd-field-card.is-form-row .prd-option-row {
    grid-column: auto;
  }

  .prd-response-item.is-form-response-row {
    grid-template-columns: 1fr;
  }

  .prd-response-item.is-form-response-row em {
    grid-column: auto;
  }

  .prd-stage {
    padding: 10px;
  }
}
</style>
