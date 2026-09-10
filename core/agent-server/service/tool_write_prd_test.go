package service

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritePRDToolReturnsV2StructuredPreview(t *testing.T) {
	reg := NewToolRegistry()
	result := reg.CallTool(context.Background(), "write_prd", validNPSPRDArgs(), "/liubeiluo/ccc", "")
	if result.IsError {
		t.Fatalf("write_prd returned error: %s", result.Content)
	}
	data, ok := result.Data.(writePRDResultData)
	if !ok {
		t.Fatalf("write_prd data type = %T, want writePRDResultData", result.Data)
	}
	if data.Kind != "agent_app_prd" || data.SchemaVersion != "prd.v2" {
		t.Fatalf("unexpected kind/version: %#v", data)
	}
	if data.Project.Code != "nps" {
		t.Fatalf("project code = %q, want nps", data.Project.Code)
	}
	if len(data.Tables) != 2 || len(data.Forms) != 1 || len(data.Charts) != 2 {
		t.Fatalf("unexpected resource counts: tables=%d forms=%d charts=%d", len(data.Tables), len(data.Forms), len(data.Charts))
	}
	if got := data.Tables[0].Fields[0].Widget; got != "input" {
		t.Fatalf("widget = %q, want input", got)
	}
	if got := data.Tables[0].Examples[0]["问卷标题"]; got != "Q2 产品满意度调研" {
		t.Fatalf("table example title = %#v", got)
	}
	if got := data.Forms[0].Example.Response["评分类型"]; got != "推荐者" {
		t.Fatalf("form response example = %#v", got)
	}
	if got := data.Charts[0].Examples[0]["NPS分数"]; got != float64(35) {
		t.Fatalf("chart example metric = %#v, want 35", got)
	}
	if !strings.Contains(result.Content, "方案已生成") || !strings.Contains(result.Content, "不阻塞后续对话") {
		t.Fatalf("result content should contain preview notice, got %q", result.Content)
	}
}

func TestWritePRDToolRejectsLegacyPRDShape(t *testing.T) {
	reg := NewToolRegistry()
	result := reg.CallTool(context.Background(), "write_prd", map[string]interface{}{
		"project": map[string]interface{}{
			"name":    "工单管理",
			"code":    "ticket",
			"summary": "管理工单",
		},
		"models": []map[string]interface{}{
			{"name": "工单", "fields": []map[string]interface{}{{"name": "标题", "widget": "name:标题;type:input"}}},
		},
		"functions": []map[string]interface{}{
			{"title": "工单列表", "type": "table", "route": "ticket_list.table"},
		},
	}, "/liubeiluo/ccc", "")
	if !result.IsError {
		t.Fatal("write_prd should reject legacy models/functions shape")
	}
	data, ok := result.Data.(writePRDResultData)
	if !ok {
		t.Fatalf("write_prd data type = %T, want writePRDResultData", result.Data)
	}
	joined := strings.Join(data.Issues, "\n")
	for _, want := range []string{"$.models 不是 PRD v2 字段", "$.functions 不是 PRD v2 字段", "tables/forms/charts 至少需要 1 个业务资源"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("issues should contain %q, got %q", want, joined)
		}
	}
}

func TestWritePRDToolRejectsWidgetTagsAndInvalidExamples(t *testing.T) {
	reg := NewToolRegistry()
	args := validNPSPRDArgs()
	tables := args["tables"].([]map[string]interface{})
	tables[0]["columns"] = []string{"问卷标题"}
	fields := tables[0]["fields"].([]map[string]interface{})
	fields[0]["widget"] = "name:问卷标题;type:input"
	examples := tables[0]["examples"].([]map[string]interface{})
	examples[0]["questionnaire_title"] = "Q2"
	charts := args["charts"].([]map[string]interface{})
	chartExamples := charts[0]["examples"].([]map[string]interface{})
	delete(chartExamples[0], "NPS分数")

	result := reg.CallTool(context.Background(), "write_prd", args, "/liubeiluo/ccc", "")
	if !result.IsError {
		t.Fatal("write_prd should reject widget tag and bad example keys")
	}
	data, ok := result.Data.(writePRDResultData)
	if !ok {
		t.Fatalf("write_prd data type = %T, want writePRDResultData", result.Data)
	}
	joined := strings.Join(data.Issues, "\n")
	for _, want := range []string{
		"tables[0].columns 不是 PRD v2 字段",
		"widget 只能写简单组件类型",
		"包含非业务字段 key：questionnaire_title",
		"缺少示例字段：NPS分数",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("issues should contain %q, got %q", want, joined)
		}
	}
}

func TestWritePRDToolAcceptsRichTextWidget(t *testing.T) {
	reg := NewToolRegistry()
	args := validNPSPRDArgs()
	tables := args["tables"].([]map[string]interface{})
	tableFields := tables[0]["fields"].([]map[string]interface{})
	tableFields[1]["widget"] = "richtext"
	forms := args["forms"].([]map[string]interface{})
	requestFields := forms[0]["request_fields"].([]map[string]interface{})
	requestFields[3]["widget"] = "richtext"

	result := reg.CallTool(context.Background(), "write_prd", args, "/liubeiluo/ccc", "")
	if result.IsError {
		t.Fatalf("write_prd should accept richtext widget: %s", result.Content)
	}
	data, ok := result.Data.(writePRDResultData)
	if !ok {
		t.Fatalf("write_prd data type = %T, want writePRDResultData", result.Data)
	}
	if got := data.Tables[0].Fields[1].Widget; got != "richtext" {
		t.Fatalf("table rich text widget = %q, want richtext", got)
	}
	if got := data.Forms[0].RequestFields[3].Widget; got != "richtext" {
		t.Fatalf("form rich text widget = %q, want richtext", got)
	}
}

func TestWritePRDToolRejectsInvalidReferences(t *testing.T) {
	reg := NewToolRegistry()
	args := validNPSPRDArgs()
	forms := args["forms"].([]map[string]interface{})
	forms[0]["target_table"] = "不存在的表"

	result := reg.CallTool(context.Background(), "write_prd", args, "/liubeiluo/ccc", "")
	if !result.IsError {
		t.Fatal("write_prd should reject invalid references")
	}
	data, ok := result.Data.(writePRDResultData)
	if !ok {
		t.Fatalf("write_prd data type = %T, want writePRDResultData", result.Data)
	}
	joined := strings.Join(data.Issues, "\n")
	for _, want := range []string{
		"target_table 必须引用 tables 中已定义的 name",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("issues should contain %q, got %q", want, joined)
		}
	}
}

func TestWritePRDToolAcceptsNestedChartExamplesAndNormalizes(t *testing.T) {
	reg := NewToolRegistry()
	args := validNPSPRDArgs()
	charts := args["charts"].([]map[string]interface{})
	charts[0]["dimension"] = "日期（按天/周/月）"
	charts[0]["examples"] = []map[string]interface{}{
		{
			"dimension": "2026-05-01",
			"metrics": map[string]interface{}{
				"NPS分数": 45,
				"评分数量":  80,
				"推荐者占比": 62,
				"贬损者占比": 17,
			},
		},
		{
			"dimension": "2026-05-02",
			"metrics": map[string]interface{}{
				"NPS分数": 48,
				"评分数量":  96,
				"推荐者占比": 64,
				"贬损者占比": 16,
			},
		},
		{
			"dimension": "2026-05-03",
			"metrics": map[string]interface{}{
				"NPS分数": 50,
				"评分数量":  102,
				"推荐者占比": 65,
				"贬损者占比": 15,
			},
		},
		{
			"dimension": "2026-05-04",
			"metrics": map[string]interface{}{
				"NPS分数": 52,
				"评分数量":  108,
				"推荐者占比": 66,
				"贬损者占比": 14,
			},
		},
	}

	result := reg.CallTool(context.Background(), "write_prd", args, "/liubeiluo/ccc", "")
	if result.IsError {
		t.Fatalf("write_prd should accept nested chart examples and normalize them: %s", result.Content)
	}
	data, ok := result.Data.(writePRDResultData)
	if !ok {
		t.Fatalf("write_prd data type = %T, want writePRDResultData", result.Data)
	}
	if got := data.Charts[0].Dimension; got != "日期" {
		t.Fatalf("dimension = %q, want 日期", got)
	}
	if got := len(data.Charts[0].Examples); got != 4 {
		t.Fatalf("normalized chart examples length = %d, want 4", got)
	}
	example := data.Charts[0].Examples[0]
	if got := example["日期"]; got != "2026-05-01" {
		t.Fatalf("normalized dimension value = %#v, want 2026-05-01", got)
	}
	if got := example["NPS分数"]; got != float64(45) {
		t.Fatalf("normalized NPS metric = %#v, want 45", got)
	}
	if _, ok := example["dimension"]; ok {
		t.Fatalf("normalized chart example should not keep dimension key: %#v", example)
	}
	if _, ok := example["metrics"]; ok {
		t.Fatalf("normalized chart example should not keep metrics key: %#v", example)
	}
}

func TestWritePRDToolAllowsFormWithoutResponseFields(t *testing.T) {
	reg := NewToolRegistry()
	args := validNPSPRDArgs()
	forms := args["forms"].([]map[string]interface{})
	forms[0]["response_fields"] = []map[string]interface{}{}
	example := forms[0]["example"].(map[string]interface{})
	example["response"] = map[string]interface{}{}

	result := reg.CallTool(context.Background(), "write_prd", args, "/liubeiluo/ccc", "")
	if result.IsError {
		t.Fatalf("write_prd should allow empty response when response_fields is empty: %s", result.Content)
	}
}

func TestWritePRDToolAllowsFormOnlyProcessingPRD(t *testing.T) {
	reg := NewToolRegistry()
	result := reg.CallTool(context.Background(), "write_prd", map[string]interface{}{
		"project": map[string]interface{}{
			"name":    "PDF 工具",
			"code":    "pdf_tool",
			"summary": "上传 PDF 后提取文本或生成处理结果。",
		},
		"forms": []map[string]interface{}{
			{
				"name": "PDF文本提取",
				"desc": "上传 PDF 文件后提取文本内容，不写入业务表。",
				"request_fields": []map[string]interface{}{
					{"name": "上传PDF文件", "widget": "files", "required": true, "desc": "上传一个或多个 PDF 文件。"},
				},
				"response_fields": []map[string]interface{}{
					{"name": "提取文本", "widget": "text_area", "required": false, "desc": "展示提取出的文本内容。"},
				},
				"example": map[string]interface{}{
					"request":  map[string]interface{}{"上传PDF文件": "合同.pdf"},
					"response": map[string]interface{}{"提取文本": "合同正文摘要..."},
				},
			},
		},
	}, "/liubeiluo/ccc", "")
	if result.IsError {
		t.Fatalf("write_prd should allow form-only processing PRD: %s", result.Content)
	}
}

func TestWritePRDCaseCatalogJSONFilesAreValidV2(t *testing.T) {
	root := filepath.Join("..", "prompt", "system", "prompt", "case_catalog")
	reg := NewToolRegistry()
	var checked int
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "prd.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var args map[string]interface{}
		if err := json.Unmarshal(data, &args); err != nil {
			t.Fatalf("%s is not valid JSON: %v", path, err)
		}
		result := reg.CallTool(context.Background(), "write_prd", args, "/liubeiluo/ccc", "")
		if result.IsError {
			t.Fatalf("%s should be valid PRD v2: %s", path, result.Content)
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatalf("walk case catalog prd.json: %v", err)
	}
	if checked == 0 {
		t.Fatal("expected at least one case catalog prd.json")
	}
}

func TestWritePRDToolSchemaExposesV2Shape(t *testing.T) {
	reg := NewToolRegistry()
	def := reg.tools["write_prd"].Definition()
	if def.Name != "write_prd" {
		t.Fatalf("tool name = %q", def.Name)
	}
	inputProps := def.InputSchema["properties"].(map[string]interface{})
	for _, name := range []string{"project", "tables", "forms", "charts", "rules"} {
		if _, ok := inputProps[name]; !ok {
			t.Fatalf("write_prd input schema should expose %q", name)
		}
	}
	for _, legacy := range []string{"models", "functions", "workflow", "acceptance_cases", "confirmation"} {
		if _, ok := inputProps[legacy]; ok {
			t.Fatalf("write_prd input schema should not expose legacy %q", legacy)
		}
	}
	rawInput, err := json.Marshal(def.InputSchema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	for _, forbidden := range []string{"go_source", "go_name", "json_name", "gorm", "sample_rows", "preview_data", "route", "method", "order"} {
		if strings.Contains(string(rawInput), forbidden) {
			t.Fatalf("write_prd input schema should not expose %q: %s", forbidden, string(rawInput))
		}
	}

	outputProps := def.OutputSchema["properties"].(map[string]interface{})
	data, ok := outputProps["data"].(map[string]interface{})
	if !ok {
		t.Fatal("write_prd output schema should expose data")
	}
	dataProps := data["properties"].(map[string]interface{})
	for _, name := range []string{"kind", "schema_version", "project", "tables", "forms", "charts", "rules"} {
		if _, ok := dataProps[name]; !ok {
			t.Fatalf("write_prd data schema should expose %q", name)
		}
	}
	if _, ok := dataProps["interaction"]; ok {
		t.Fatalf("write_prd data schema should not expose a pending interaction")
	}
}

func validNPSPRDArgs() map[string]interface{} {
	return map[string]interface{}{
		"project": map[string]interface{}{
			"name":    "NPS 净推荐值系统",
			"code":    "nps",
			"summary": "收集用户推荐评分，统计 NPS 分数、评分趋势和推荐者、中立者、贬损者分布。",
		},
		"tables": []map[string]interface{}{
			{
				"name":  "NPS问卷",
				"title": "NPS问卷管理",
				"desc":  "管理每次 NPS 调研活动。",
				"fields": []map[string]interface{}{
					{"name": "问卷标题", "widget": "input", "required": true, "desc": "调研问卷的标题，建议简短清晰。"},
					{"name": "问卷描述", "widget": "text_area", "required": false, "desc": "说明本次调研目的和填写说明。"},
					{"name": "状态", "widget": "select", "required": true, "desc": "问卷状态，有待发送、收集中、已结束三个选项，默认待发送。"},
					{"name": "评分数量", "widget": "integer", "required": false, "hide": "create,update", "desc": "该问卷收到的评分记录数量，由系统统计。"},
				},
				"search_fields": []map[string]interface{}{
					{"name": "问卷标题", "widget": "input", "required": false, "desc": "按问卷标题模糊搜索。"},
					{"name": "状态", "widget": "select", "required": false, "desc": "按问卷状态筛选，可选待发送、收集中、已结束。"},
					{"name": "创建人", "widget": "user", "required": false, "desc": "按系统记录的创建人筛选。"},
					{"name": "创建开始时间", "widget": "datetime", "required": false, "desc": "按记录创建时间范围查询的开始时间。"},
					{"name": "创建结束时间", "widget": "datetime", "required": false, "desc": "按记录创建时间范围查询的结束时间。"},
				},
				"handlers": []string{"OnTableAddRow", "OnTableUpdateRow", "OnTableDeleteRow"},
				"examples": []map[string]interface{}{
					{"问卷标题": "Q2 产品满意度调研", "问卷描述": "了解用户对产品功能和服务体验的推荐意愿。", "状态": "收集中", "评分数量": 256},
				},
			},
			{
				"name":  "NPS评分记录",
				"title": "NPS评分记录",
				"desc":  "查看用户提交的 NPS 分数、评分类型和推荐理由。",
				"fields": []map[string]interface{}{
					{"name": "问卷", "widget": "select", "required": true, "desc": "关联的 NPS 问卷，从 NPS问卷 中选择。"},
					{"name": "评分人", "widget": "input", "required": false, "desc": "提交评分的用户。"},
					{"name": "NPS分数", "widget": "integer", "required": true, "desc": "0 到 10 的整数评分，0 表示完全不推荐，10 表示非常愿意推荐。"},
					{"name": "评分类型", "widget": "select", "required": false, "hide": "create,update", "desc": "根据 NPS分数 自动计算，有推荐者、中立者、贬损者三个类型：0-6 为贬损者，7-8 为中立者，9-10 为推荐者。"},
					{"name": "推荐理由", "widget": "text_area", "required": false, "desc": "用户推荐或不推荐的具体原因。"},
					{"name": "评分时间", "widget": "datetime", "required": false, "hide": "create,update", "desc": "评分提交时间，由系统记录。"},
				},
				"search_fields": []map[string]interface{}{
					{"name": "问卷", "widget": "select", "required": false, "desc": "按问卷筛选评分记录。"},
					{"name": "评分类型", "widget": "select", "required": false, "desc": "按推荐者、中立者、贬损者筛选。"},
					{"name": "评分人", "widget": "user", "required": false, "desc": "按提交评分的用户筛选。"},
					{"name": "评分开始时间", "widget": "datetime", "required": false, "desc": "评分时间开始。"},
					{"name": "评分结束时间", "widget": "datetime", "required": false, "desc": "评分时间结束。"},
				},
				"handlers": []string{},
				"examples": []map[string]interface{}{
					{"问卷": "Q2 产品满意度调研", "评分人": "张三", "NPS分数": 9, "评分类型": "推荐者", "推荐理由": "产品稳定，服务响应快。", "评分时间": "2026-05-09 10:30"},
				},
			},
		},
		"forms": []map[string]interface{}{
			{
				"name":         "NPS评分提交",
				"desc":         "客户自助提交 NPS 评分，只提交评分和推荐理由，不通过评分记录表手工新增。",
				"target_table": "NPS评分记录",
				"request_fields": []map[string]interface{}{
					{"name": "问卷", "widget": "select", "required": true, "desc": "选择要评价的 NPS 问卷。"},
					{"name": "评分人", "widget": "input", "required": false, "desc": "提交评分的用户，默认当前用户。"},
					{"name": "NPS分数", "widget": "integer", "required": true, "desc": "0 到 10 的整数评分，提交后系统按评分计算评分类型。"},
					{"name": "推荐理由", "widget": "text_area", "required": false, "desc": "填写推荐或不推荐的原因。"},
				},
				"response_fields": []map[string]interface{}{
					{"name": "评分类型", "widget": "input", "required": false, "desc": "提交后返回自动计算出的评分类型。"},
					{"name": "提交结果", "widget": "input", "required": false, "desc": "提交成功或失败的提示信息。"},
				},
				"example": map[string]interface{}{
					"request":  map[string]interface{}{"问卷": "Q2 产品满意度调研", "评分人": "张三", "NPS分数": 9, "推荐理由": "产品稳定，服务响应快。"},
					"response": map[string]interface{}{"评分类型": "推荐者", "提交结果": "提交成功，感谢您的反馈。"},
				},
			},
		},
		"charts": []map[string]interface{}{
			{
				"name":         "NPS趋势分析",
				"desc":         "按日期统计 NPS 分数、评分数量、推荐者占比和贬损者占比。",
				"source_table": "NPS评分记录",
				"chart_type":   "line",
				"dimension":    "日期",
				"metrics":      []string{"NPS分数", "评分数量", "推荐者占比", "贬损者占比"},
				"filters": []map[string]interface{}{
					{"name": "开始时间", "widget": "datetime", "required": false, "desc": "统计开始时间。"},
					{"name": "结束时间", "widget": "datetime", "required": false, "desc": "统计结束时间。"},
				},
				"examples": []map[string]interface{}{
					{"日期": "2026-05-01", "NPS分数": 35, "评分数量": 80, "推荐者占比": "52%", "贬损者占比": "17%"},
					{"日期": "2026-05-02", "NPS分数": 42, "评分数量": 96, "推荐者占比": "56%", "贬损者占比": "14%"},
				},
			},
			{
				"name":         "NPS评分分布",
				"desc":         "按评分类型展示推荐者、中立者、贬损者分布。",
				"source_table": "NPS评分记录",
				"chart_type":   "pie",
				"dimension":    "评分类型",
				"metrics":      []string{"评分人数"},
				"filters": []map[string]interface{}{
					{"name": "问卷", "widget": "select", "required": false, "desc": "选择问卷，不选则统计全部问卷。"},
					{"name": "评分开始时间", "widget": "datetime", "required": false, "desc": "评分时间开始。"},
					{"name": "评分结束时间", "widget": "datetime", "required": false, "desc": "评分时间结束。"},
				},
				"examples": []map[string]interface{}{
					{"评分类型": "推荐者", "评分人数": 98},
					{"评分类型": "中立者", "评分人数": 52},
					{"评分类型": "贬损者", "评分人数": 26},
				},
			},
		},
		"rules": []string{
			"0-6 分为贬损者，7-8 分为中立者，9-10 分为推荐者。",
			"NPS分数 = 推荐者占比 - 贬损者占比。",
			"评分类型根据 NPS分数 自动计算。",
			"NPS评分记录由 NPS评分提交产生，评分记录表只负责查询和筛选。",
		},
	}
}
