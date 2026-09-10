package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

func workspaceJSONMap(raw string) map[string]interface{} {
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func workspaceMapField(m map[string]interface{}, key string) map[string]interface{} {
	return workspaceAsMap(m[key])
}

func workspaceSliceField(m map[string]interface{}, key string) []interface{} {
	if v, ok := m[key].([]interface{}); ok {
		return v
	}
	return nil
}

func workspaceAsMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

func workspaceStringField(m map[string]interface{}, key string) string {
	return workspaceStringValue(m[key])
}

func workspaceStringValue(v interface{}) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case fmt.Stringer:
		return strings.TrimSpace(x.String())
	case map[string]interface{}:
		for _, key := range []string{"name", "field", "metric", "label", "title", "desc", "summary", "value", "code"} {
			if s := workspaceStringField(x, key); s != "" {
				return s
			}
		}
	}
	return ""
}

func workspaceNamedItems(m map[string]interface{}, key string) []string {
	items := workspaceSliceField(m, key)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s := workspaceStringValue(item); s != "" {
			out = appendUniqueWorkspaceString(out, s, 24)
		}
	}
	return out
}

func workspaceStringItems(m map[string]interface{}, key string) []string {
	items := workspaceSliceField(m, key)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s := workspaceStringValue(item); s != "" {
			out = appendUniqueWorkspaceString(out, s, 24)
		}
	}
	return out
}

func appendUniqueWorkspaceString(items []string, item string, limit int) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return items
	}
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	items = append(items, item)
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func trimWorkspaceStrings(items []string, limit int) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = appendUniqueWorkspaceString(out, strings.TrimSpace(item), limit)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func firstNonEmptyString(items ...string) string {
	for _, item := range items {
		if s := strings.TrimSpace(item); s != "" {
			return s
		}
	}
	return ""
}

func normalizeWorkspaceHandoffContextPolicy(policy string) string {
	return ContextPolicyFull
}

func defaultWorkspaceHandoffDisplayContent(artifactKind, targetRole, remark string) string {
	switch artifactKind {
	case "agent_app_prd":
		if strings.TrimSpace(remark) != "" {
			return "已按此方案开始制作。\n\n补充说明：\n" + strings.TrimSpace(remark)
		}
		return "已按此方案开始制作。"
	case workspaceBuildArtifactKind:
		if strings.TrimSpace(remark) != "" {
			return "已构建成功，进入自动测试验证。\n\n补充备注：\n" + strings.TrimSpace(remark)
		}
		return "已构建成功，进入自动测试验证。"
	case workspaceBuildFailureKind:
		if strings.TrimSpace(remark) != "" {
			return "构建失败，交接构建修复。\n\n补充备注：\n" + strings.TrimSpace(remark)
		}
		return "构建失败，交接构建修复。"
	default:
		label := strings.TrimSpace(artifactKind)
		if label == "" {
			label = "阶段产物"
		}
		return fmt.Sprintf("已确认 %s，进入 %s 阶段。", label, workspaceRoleDisplayName(targetRole))
	}
}

func buildWorkspaceHandoffContent(input workspaceHandoffContentInput) string {
	artifactLabel := input.ArtifactKind
	if artifactLabel == "" {
		artifactLabel = "artifact"
	}
	lines := []string{
		"已确认阶段交接产物，进入下一阶段。",
		"",
		fmt.Sprintf("这是当前会话内的阶段注入消息。历史对话仍然是模型上下文的一部分；请先调用 change_role，target_role 固定为 %s。", input.TargetRole),
		fmt.Sprintf("change_role.execute_directory 必须固定为 %s；后续读取、构建、测试、运行只能围绕该目录或该目录下函数。", firstNonEmptyString(input.ExecuteDirectory, "当前工作台目录")),
		"change_role 只携带四块交接信息：execute_directory、task_context、key_information、references。",
		fmt.Sprintf("上下文策略：%s。本消息中的 HANDOFF_PACKET JSON 和 HANDOFF_CONTEXT JSON 是对当前历史的结构化补充；不要丢弃用户在前文给过的关键背景、限制和偏好。", input.ContextPolicy),
		"不要重复产出已确认的设计文档；除非产物本身缺失关键字段，否则直接执行目标阶段任务。",
	}
	if input.ArtifactKind == "agent_app_prd" && normalizeWorkspaceRole(input.TargetRole) == WorkspaceRoleAppDeveloper {
		lines = append(lines,
			"生成阶段要求：不要重新输出 PRD，不要再次询问确认；change_role.references 必须包含 agent_app_prd JSON（本消息）、/system/prompt/roles/app-developer、/system/prompt/sdk/agent-app-sdk-readme、/system/prompt/case_catalog 和匹配案例路径；先读取 1 到多个匹配案例，再根据 PRD tables/forms/charts/rules 创建目录、写代码文件、注册路由并 build。tables.fields 是业务模型字段，tables.search_fields 是查询请求字段；创建开始时间/创建结束时间/创建人等系统搜索字段不要生成业务列。route、method、widget tag、列表列和预览数据均从 PRD 派生。非常简单的需求才可跳过额外案例。",
		)
	}
	if input.ArtifactKind == workspaceBuildArtifactKind && normalizeWorkspaceRole(input.TargetRole) == WorkspaceRoleQAEngineer {
		lines = append(lines,
			"测试阶段要求：不要修改代码，不要重新 build；先调用 change_role 进入 qa_engineer，并把 execute_directory 固定为本目录；read_dir/search 必须显式使用该目录，禁止测试整个空间。按业务操作顺序验证：先主数据/配置表，再 Form 提交，再目标记录表，再 Chart；重点覆盖创建开始时间/创建结束时间和用户筛选。测试失败时判断是测试数据问题、业务 bug 还是构建/schema 问题，并交接给 maintenance_engineer 或 build_engineer。",
		)
	}
	if input.ArtifactKind == workspaceBuildFailureKind && normalizeWorkspaceRole(input.TargetRole) == WorkspaceRoleBuildEngineer {
		lines = append(lines,
			"构建修复阶段要求：不要重新设计 PRD，不要进入测试，不要继续同一方案反复重写；先调用 change_role 进入 build_engineer，并把 execute_directory 固定为本目录。",
			"change_role.task_context/key_information 必须携带 HANDOFF_CONTEXT.build_diagnostics 或 agent_app_build_failure.build_diagnostics 中的错误类别、router、字段问题、必读资料和修复策略。",
			"修复前先读 /system/prompt/sdk/reference/build-validation、SDK 主文档或匹配案例；按 router/字段/文件归类同类错误，小范围批量修复后再 build_workspace。",
		)
	}
	if input.ArtifactKind == "agent_app_prd" && strings.TrimSpace(input.PRDExecutionMarkdown) != "" {
		lines = append(lines,
			"",
			"PRD_EXECUTION_MARKDOWN:",
			"```markdown",
			strings.TrimSpace(input.PRDExecutionMarkdown),
			"```",
		)
	}
	lines = append(lines,
		"",
		"HANDOFF_PACKET JSON:",
		"```json",
		nonEmptyWorkspaceHandoffJSON(input.HandoffPacketJSON),
		"```",
		"",
		"HANDOFF_CONTEXT JSON:",
		"```json",
		nonEmptyWorkspaceHandoffJSON(input.HandoffContextJSON),
		"```",
		"",
		strings.ToUpper(artifactLabel)+" JSON:",
		"```json",
		input.ArtifactJSON,
		"```",
	)
	if remark := strings.TrimSpace(input.Remark); remark != "" {
		lines = append(lines, "", "补充备注：", remark)
	}
	return strings.Join(lines, "\n")
}

func nonEmptyWorkspaceHandoffJSON(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "{}"
	}
	return s
}
