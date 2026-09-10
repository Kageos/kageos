import { mount } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'
import { describe, expect, it } from 'vitest'
import PrdPreview from './PrdPreview.vue'

const SlotStub = defineComponent({
  template: '<div><slot /></div>',
})

const ChartPreviewStub = defineComponent({
  name: 'PrdChartPreview',
  props: {
    previewData: {
      type: Array,
      default: () => [],
    },
  },
  template: '<div class="chart-preview-stub">chart</div>',
})

const ButtonStub = defineComponent({
  emits: ['click'],
  template: '<button type="button" @click="$emit(\'click\', $event)"><slot /></button>',
})

function globalStubs() {
  return {
    PrdChartPreview: ChartPreviewStub,
    TableIcon: SlotStub,
    FormIcon: SlotStub,
    ChartIcon: SlotStub,
    ElIcon: SlotStub,
    ElTag: SlotStub,
    ElButton: ButtonStub,
    ElEmpty: SlotStub,
  }
}

function baseData(): any {
  return {
    kind: 'agent_app_prd',
    schema_version: 'prd.v2',
    project: {
      name: 'NPS 净推荐值系统',
      code: 'nps',
      summary: '收集用户推荐评分，统计 NPS 分数、评分趋势和推荐者、中立者、贬损者分布。',
    },
    tables: [
      {
        name: 'NPS问卷',
        title: 'NPS问卷管理',
        desc: '管理每次 NPS 调研活动。',
        fields: [
          { name: '问卷标题', widget: 'input', required: true, desc: '调研问卷的标题，建议简短清晰。' },
          { name: '状态', widget: 'select', required: true, desc: '问卷状态，有待发送、收集中、已结束三个选项，默认待发送。' },
          { name: '评分数量', widget: 'number', required: false, hide: 'create,update', desc: '该问卷收到的评分记录数量，由系统统计。' },
        ],
        search_fields: [
          { name: '问卷标题', widget: 'input', required: false, desc: '按问卷标题模糊搜索。' },
          { name: '状态', widget: 'select', required: false, desc: '按问卷状态筛选，可选待发送、收集中、已结束。' },
        ],
        handlers: ['OnTableAddRow', 'OnTableUpdateRow', 'OnTableDeleteRow'],
        examples: [
          { 问卷标题: 'Q2 产品满意度调研', 状态: '收集中', 评分数量: 256 },
        ],
      },
      {
        name: 'NPS评分记录',
        title: 'NPS评分记录',
        desc: '查看用户提交的 NPS 分数、评分类型和推荐理由。',
        fields: [
          { name: '问卷', widget: 'select', required: true, desc: '关联的 NPS 问卷，从 NPS问卷 中选择。' },
          { name: '评分人', widget: 'input', required: false, desc: '提交评分的用户姓名或客户标识。' },
          { name: 'NPS分数', widget: 'number', required: true, desc: '0 到 10 的整数评分。' },
          { name: '评分类型', widget: 'select', required: false, hide: 'create,update', desc: '根据 NPS分数 自动计算。' },
        ],
        search_fields: [
          { name: '问卷', widget: 'select', required: false, desc: '按问卷筛选评分记录。' },
          { name: '评分类型', widget: 'select', required: false, desc: '按推荐者、中立者、贬损者筛选。' },
        ],
        handlers: [],
        examples: [
          { 问卷: 'Q2 产品满意度调研', 评分人: '张三', NPS分数: 9, 评分类型: '推荐者' },
        ],
      },
    ],
    forms: [
      {
        name: 'NPS评分提交',
        desc: '客户自助提交 NPS 评分。',
        target_table: 'NPS评分记录',
        request_fields: [
          { name: '问卷', widget: 'select', required: true, desc: '选择要评价的 NPS 问卷。' },
          { name: 'NPS分数', widget: 'number', required: true, desc: '0 到 10 的整数评分。' },
        ],
        response_fields: [
          { name: '评分类型', widget: 'input', required: false, desc: '提交后返回自动计算出的评分类型。' },
          { name: '提交结果', widget: 'input', required: false, desc: '提交成功或失败的提示信息。' },
        ],
        example: {
          request: { 问卷: 'Q2 产品满意度调研', NPS分数: 9 },
          response: { 评分类型: '推荐者', 提交结果: '提交成功，感谢您的反馈。' },
        },
      },
    ],
    charts: [
      {
        name: 'NPS趋势分析',
        desc: '按日期统计 NPS 分数、评分数量、推荐者占比和贬损者占比。',
        source_table: 'NPS评分记录',
        chart_type: 'line',
        dimension: '日期',
        metrics: ['NPS分数', '评分数量'],
        filters: [
          { name: '开始时间', widget: 'datetime', required: false, desc: '统计开始时间。' },
          { name: '结束时间', widget: 'datetime', required: false, desc: '统计结束时间。' },
        ],
        examples: [
          { dimension: '2026-05-01', metrics: { NPS分数: 35, 评分数量: 80 } },
          { dimension: '2026-05-02', metrics: { NPS分数: 42, 评分数量: 96 } },
        ],
      },
    ],
    rules: [
      '0-6 分为贬损者，7-8 分为中立者，9-10 分为推荐者。',
    ],
  }
}

function mountPreview(data: any = baseData(), extraProps: Record<string, unknown> = {}) {
  return mount(PrdPreview, {
    props: { data, ...extraProps },
    global: {
      stubs: globalStubs(),
    },
  })
}

describe('PrdPreview', () => {
  it('switches feature previews by thumbnail index', async () => {
    const wrapper = mountPreview()

    expect(wrapper.find('.prd-stage-name').text()).toBe('NPS问卷管理')
    expect(wrapper.find('.prd-basic-table').exists()).toBe(true)

    const thumbs = wrapper.findAll('.prd-slide-thumb')
    expect(thumbs).toHaveLength(4)
    expect(thumbs[0]!.classes()).toContain('is-active')

    await thumbs[1]!.trigger('pointerup')
    await nextTick()
    expect(wrapper.find('.prd-stage-name').text()).toBe('NPS评分提交')
    expect(wrapper.text()).toContain('请求字段')
    expect(wrapper.text()).toContain('提交成功，感谢您的反馈。')

    await wrapper.findAll('.prd-slide-thumb')[3]!.trigger('pointerup')
    await nextTick()
    expect(wrapper.find('.prd-stage-name').text()).toBe('NPS趋势分析')
    expect(wrapper.find('.chart-preview-stub').exists()).toBe(true)
  })

  it('renders table search fields, handlers and inline examples', () => {
    const wrapper = mountPreview()

    expect(wrapper.text()).toContain('搜索条件')
    expect(wrapper.text()).toContain('按问卷标题模糊搜索')
    expect(wrapper.text()).toContain('新增')
    expect(wrapper.text()).toContain('编辑')
    expect(wrapper.text()).toContain('删除')
    expect(wrapper.find('.prd-basic-table').text()).toContain('Q2 产品满意度调研')
    expect(wrapper.find('.prd-basic-table').text()).toContain('收集中')
  })

  it('passes chart examples as preview data', async () => {
    const wrapper = mountPreview()
    await wrapper.findAll('.prd-slide-thumb')[3]!.trigger('pointerup')
    await nextTick()

    const chartPreview = wrapper.findComponent(ChartPreviewStub)
    expect(chartPreview.props('previewData')).toEqual([
      { 日期: '2026-05-01', NPS分数: 35, 评分数量: 80 },
      { 日期: '2026-05-02', NPS分数: 42, 评分数量: 96 },
    ])
    expect(wrapper.text()).toContain('开始时间')
    expect(wrapper.text()).toContain('结束时间')
  })

  it('keeps confirmation controls hidden by default', () => {
    const wrapper = mountPreview()

    expect(wrapper.text()).not.toContain('请确认是否按以上 PRD 创建服务目录和生成代码')
    expect(wrapper.find('textarea').exists()).toBe(false)
    expect(wrapper.findAll('button').some(button => button.text().includes('按此方案开始'))).toBe(false)
  })

  it('emits confirm payload when confirmation controls are explicitly enabled', async () => {
    const wrapper = mountPreview(baseData(), { showConfirmation: true })

    await wrapper.find('textarea').setValue('按这个做')
    const confirmButton = wrapper.findAll('button').find(button => button.text().includes('按此方案开始'))
    expect(confirmButton).toBeTruthy()
    await confirmButton!.trigger('click')

    expect(wrapper.emitted('confirm')?.[0]?.[0]).toMatchObject({
      remark: '按这个做',
    })
  })
})
