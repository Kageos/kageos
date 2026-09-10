package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kageos/kageos/core/agent-server/model"
	"github.com/kageos/kageos/core/agent-server/prompt"
	"github.com/kageos/kageos/core/agent-server/repository"
	"github.com/kageos/kageos/core/agent-server/streamloop"
	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/contextx"
	"github.com/kageos/kageos/pkg/llms"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWorkspaceCtxToEnvInputIncludesDirectoryContacts(t *testing.T) {
	input := workspaceCtxToEnvInput(&dto.GetWorkspaceContextResp{
		Directory: dto.WorkspaceContextDirectory{
			Name:         "工单巡检",
			Code:         "ticket_watch",
			FullCodePath: "/alice/ops/ticket_watch",
			Type:         "package",
			Owner:        "alice",
			Admins:       "bob,carol",
		},
	})
	if input == nil {
		t.Fatal("workspaceCtxToEnvInput returned nil")
	}
	if input.DirOwner != "alice" || input.DirAdmins != "bob,carol" {
		t.Fatalf("directory contacts = owner:%q admins:%q, want alice and bob,carol", input.DirOwner, input.DirAdmins)
	}
}

func TestGuideDocPathsFromReadDocArgsSupportsCommaSeparatedPaths(t *testing.T) {
	paths := guideDocPathsFromReadDocArgs(map[string]interface{}{
		"directory": "/system/prompt/platform-capability-boundaries,/system/prompt/sdk/agent-app-sdk-readme/",
	})
	loaded := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		loaded[path] = struct{}{}
	}

	if _, ok := loaded["/system/prompt/platform-capability-boundaries"]; !ok {
		t.Fatalf("expected platform capability boundaries to be marked loaded: %#v", loaded)
	}
	if _, ok := loaded["/system/prompt/sdk/agent-app-sdk-readme"]; !ok {
		t.Fatalf("expected sdk readme to be marked loaded: %#v", loaded)
	}
}

func TestWithAgentToolExecutionContextMarksSource(t *testing.T) {
	base := contextx.WithClientSource(context.Background(), "browser")

	ctx := withAgentToolExecutionContext(base, "session-1", "测试会话", "app_operator")

	if got := contextx.GetClientSource(ctx); got != "agent" {
		t.Fatalf("client source = %q, want agent", got)
	}
	if got := contextx.GetSourceType(ctx); got != contextx.SourceTypeAgentTool {
		t.Fatalf("source type = %q, want %s", got, contextx.SourceTypeAgentTool)
	}
	if got := contextx.GetSourceRef(ctx); got != "session-1" {
		t.Fatalf("source ref = %q, want session-1", got)
	}
}

func TestWorkspaceContextWithSessionRequestUserFallsBackToSessionOwner(t *testing.T) {
	session := &model.AgentChatSession{User: "alice"}

	gotCtx, gotUser := workspaceContextWithSessionRequestUser(context.Background(), session)

	if gotUser != "alice" {
		t.Fatalf("user = %q, want alice", gotUser)
	}
	if got := contextx.GetRequestUser(gotCtx); got != "alice" {
		t.Fatalf("request user = %q, want alice", got)
	}
}

func TestWorkspaceContextWithSessionRequestUserKeepsSystemRequestUser(t *testing.T) {
	session := &model.AgentChatSession{User: "alice"}
	ctx := contextx.WithRequestUser(context.Background(), "system")

	gotCtx, gotUser := workspaceContextWithSessionRequestUser(ctx, session)

	if gotUser != "system" {
		t.Fatalf("user = %q, want system", gotUser)
	}
	if got := contextx.GetRequestUser(gotCtx); got != "system" {
		t.Fatalf("request user = %q, want system", got)
	}
}

func TestWorkspaceContextWithSessionRequestUserKeepsRealRequestUser(t *testing.T) {
	session := &model.AgentChatSession{User: "alice"}
	ctx := contextx.WithRequestUser(context.Background(), "bob")

	gotCtx, gotUser := workspaceContextWithSessionRequestUser(ctx, session)

	if gotUser != "bob" {
		t.Fatalf("user = %q, want bob", gotUser)
	}
	if got := contextx.GetRequestUser(gotCtx); got != "bob" {
		t.Fatalf("request user = %q, want bob", got)
	}
}

func TestCancelSessionCancelsRegisteredRunEvenWhenStatusIsActive(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate sessions: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	session := &model.AgentChatSession{
		TreeID:        1,
		FullCodePath:  "/system/x_world/vote",
		Source:        SourceWorkspace,
		SessionID:     "stale-running-session",
		Title:         "运行中会话",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusActive,
		ContextPolicy: ContextPolicyFull,
		User:          "alice",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := &WorkspaceChatService{sessionRepo: sessionRepo}
	svc.runningCancels.Store("stale-running-session", cancel)
	ctx := context.WithValue(context.Background(), contextx.RequestUserHeader, "alice")

	if err := svc.CancelSession(ctx, "stale-running-session"); err != nil {
		t.Fatalf("cancel session: %v", err)
	}
	if runCtx.Err() == nil {
		t.Fatal("registered run context should be cancelled")
	}
	latest, err := sessionRepo.GetBySessionID("stale-running-session")
	if err != nil {
		t.Fatalf("get latest session: %v", err)
	}
	if latest.Status != model.ChatSessionStatusCancelled {
		t.Fatalf("status = %s, want cancelled", latest.Status)
	}
	if err := svc.CancelSession(ctx, "stale-running-session"); err != nil {
		t.Fatalf("second cancel should be idempotent, got %v", err)
	}
}

func TestParseToolCallArgsRejectsInvalidJSON(t *testing.T) {
	svc := &WorkspaceChatService{}
	call := llms.ToolCall{ID: "call_bad", Type: "function"}
	call.Function.Name = "write_file"
	call.Function.Arguments = `{"content":"unterminated`

	_, err := svc.parseToolCallArgs(context.Background(), call)
	if err == nil {
		t.Fatal("expected invalid tool arguments to return an error")
	}
	res := invalidToolArgumentsResult(call, err)
	if !res.IsError || !strings.Contains(res.Content, "参数不是合法 JSON") {
		t.Fatalf("unexpected invalid arguments result: %#v", res)
	}
}

func TestSaveAssistantMessageStoresLLMMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	messageRepo := repository.NewChatMessageRepository(db)
	svc := &WorkspaceChatService{messageRepo: messageRepo}
	meta := messageLLMMetadata{
		ConfigID:   12,
		ConfigName: "OpenAI Mini",
		Provider:   model.LLMProviderOpenAI,
		Model:      "gpt-4o-mini",
	}

	usage := &llms.Usage{PromptTokens: 1200, CompletionTokens: 80, TotalTokens: 1280, CachedTokens: 1024, CachedTokensReported: true}
	if err := svc.saveAssistantMessage(context.Background(), "session-llm", "ok", "内部思考", "tester", meta, nil, usage); err != nil {
		t.Fatalf("save assistant message: %v", err)
	}
	messages, err := messageRepo.ListBySessionID("session-llm")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	got := messages[0]
	if got.LLMConfigID != meta.ConfigID || got.LLMConfigName != meta.ConfigName || got.LLMProvider != meta.Provider || got.LLMModel != meta.Model {
		t.Fatalf("LLM metadata not stored: %#v", got)
	}
	if got.LLMUsage == nil || !strings.Contains(*got.LLMUsage, `"cached_tokens":1024`) || !strings.Contains(*got.LLMUsage, `"cached_tokens_reported":true`) {
		t.Fatalf("LLM usage not stored: %#v", got.LLMUsage)
	}
	if got.ThinkingContent != "内部思考" {
		t.Fatalf("thinking content = %q, want persisted thinking", got.ThinkingContent)
	}
}

func TestBuildLLMMessagesWithPlanReportsContextPolicyAndHandoff(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate sessions: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:                      7,
		FullCodePath:                "/liubeiluo/vote",
		Source:                      SourceWorkspace,
		SessionID:                   "model-plan-session",
		Title:                       "模型上下文计划",
		ModeCode:                    "dev",
		Status:                      model.ChatSessionStatusActive,
		RoleID:                      WorkspaceRoleQAEngineer,
		RoleDisplayName:             "测试工程师",
		ParentSessionID:             "source-session",
		ContextPolicy:               ContextPolicyArtifactOnly,
		ModelContextAnchorMessageID: 1,
		User:                        "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	oldMsg := &model.AgentChatMessage{SessionID: session.SessionID, Role: RoleUser, Content: "旧开发讨论", User: "tester"}
	if err := messageRepo.Create(oldMsg); err != nil {
		t.Fatalf("create old message: %v", err)
	}
	packet := workspaceRoleHandoffPacket{
		Version:          workspaceRoleHandoffPacketVersion,
		SourceSessionID:  "source-session",
		SourceRole:       WorkspaceRoleBuildEngineer,
		TargetRole:       WorkspaceRoleQAEngineer,
		ArtifactKind:     workspaceBuildArtifactKind,
		ExecuteDirectory: "/liubeiluo/vote",
		TaskContext:      []string{"build 已通过，进入自动测试"},
		KeyInformation:   []string{"重点验证投票提交"},
		References:       []string{"/system/prompt/roles/qa-engineer"},
		ContextPolicy:    ContextPolicyArtifactOnly,
	}
	normalizeAndValidateWorkspaceRoleHandoffPacket(&packet)
	handoffMsg := &model.AgentChatMessage{
		SessionID:    session.SessionID,
		Role:         RoleUser,
		Content:      "HANDOFF_PACKET JSON:\n```json\n" + formatWorkspaceRoleHandoffPacketJSON(&packet) + "\n```",
		ContextUsage: MessageContextArtifact,
		ArtifactKind: workspaceBuildArtifactKind,
		User:         "tester",
	}
	if err := messageRepo.Create(handoffMsg); err != nil {
		t.Fatalf("create handoff message: %v", err)
	}
	displayOnlyMsg := &model.AgentChatMessage{SessionID: session.SessionID, Role: RoleUser, Content: "只展示", ContextUsage: MessageContextDisplayOnly, User: "tester"}
	if err := messageRepo.Create(displayOnlyMsg); err != nil {
		t.Fatalf("create display only message: %v", err)
	}
	svc := &WorkspaceChatService{
		toolReg:     NewToolRegistry(),
		sessionRepo: sessionRepo,
		messageRepo: messageRepo,
	}
	workspaceCtx := &dto.GetWorkspaceContextResp{}
	workspaceCtx.Directory.Name = "投票系统"
	workspaceCtx.Directory.Code = "vote"
	workspaceCtx.Directory.Type = "package"

	msgs, tools, plan, err := svc.buildLLMMessagesWithPlan(context.Background(), session.SessionID, "/liubeiluo/vote", "投票系统", workspaceCtx, nil, []string{"read_doc", "search"}, "fallback", 2)
	if err != nil {
		t.Fatalf("build messages: %v", err)
	}
	if plan == nil {
		t.Fatal("plan is nil")
	}
	if len(msgs) != 3 {
		t.Fatalf("llm messages = %d, want system + old + handoff without display-only history", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "/system/prompt/platform-introduction") ||
		!strings.Contains(msgs[0].Content, "/system/prompt/platform-usage-and-philosophy") ||
		!strings.Contains(msgs[0].Content, "/system/prompt/platform-capability-boundaries") {
		t.Fatalf("system message should route kageos introduction, usage and boundary questions to docs:\n%s", msgs[0].Content)
	}
	if strings.Contains(msgs[0].Content, "恰研智能（qiayan.ai）") ||
		strings.Contains(msgs[0].Content, "转 Apache-2.0") {
		t.Fatalf("system message should not inline detailed identity/license posture:\n%s", msgs[0].Content)
	}
	modeIdx := strings.Index(msgs[0].Content, "fallback")
	envIdx := strings.Index(msgs[0].Content, "# 工作环境信息")
	if modeIdx < 0 || envIdx < 0 || modeIdx > envIdx {
		t.Fatalf("stable mode/system prompt should appear before dynamic workspace env, modeIdx=%d envIdx=%d", modeIdx, envIdx)
	}
	if len(tools) != 2 {
		t.Fatalf("tools = %d, want 2", len(tools))
	}
	if plan.ProtocolVersion != workspaceModelContextPlanVersion || plan.Round != 2 {
		t.Fatalf("bad plan identity: %#v", plan)
	}
	if plan.Role.ID != WorkspaceRoleQAEngineer || plan.Role.Source != "session" {
		t.Fatalf("bad role plan: %#v", plan.Role)
	}
	if plan.Messages.ContextPolicy != ContextPolicyFull || plan.Messages.ExcludedByAnchor != 0 || plan.Messages.ExcludedDisplayOnly != 1 {
		t.Fatalf("bad message policy: %#v", plan.Messages)
	}
	if plan.Messages.SourceHistoryPolicy != "same_session_full_with_parent_reference" {
		t.Fatalf("source history policy = %q", plan.Messages.SourceHistoryPolicy)
	}
	if plan.Handoff == nil || plan.Handoff.TargetRole != WorkspaceRoleQAEngineer || plan.Handoff.ExecuteDirectory != "/liubeiluo/vote" {
		t.Fatalf("bad handoff plan: %#v", plan.Handoff)
	}
	if !containsWorkspaceRoleString(plan.Docs.MissingDocs, "/system/prompt/roles/qa-engineer") {
		t.Fatalf("missing docs should include qa guide: %#v", plan.Docs.MissingDocs)
	}
	if !containsWorkspaceRoleString(plan.Tools.LLMTools, "read_doc") || !containsWorkspaceRoleString(plan.Tools.LLMTools, "search") {
		t.Fatalf("bad tool plan: %#v", plan.Tools)
	}
}

func TestBuildLLMMessagesExposesStableModeToolsAndKeepsRoleGatePlan(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate sessions: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:          7,
		FullCodePath:    "/liubeiluo/vote",
		Source:          SourceWorkspace,
		SessionID:       "stable-tool-plan-session",
		Title:           "稳定工具集合",
		ModeCode:        "dev",
		Status:          model.ChatSessionStatusActive,
		RoleID:          WorkspaceRoleQAEngineer,
		RoleDisplayName: "测试工程师",
		User:            "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	provider := prompt.GetModeProvider("dev")
	if provider == nil {
		t.Fatal("dev mode provider is nil")
	}
	svc := &WorkspaceChatService{
		toolReg:     NewToolRegistry(),
		sessionRepo: sessionRepo,
		messageRepo: messageRepo,
	}
	workspaceCtx := &dto.GetWorkspaceContextResp{}
	workspaceCtx.Directory.Name = "投票系统"
	workspaceCtx.Directory.Code = "vote"
	workspaceCtx.Directory.Type = "package"

	_, tools, plan, err := svc.buildLLMMessagesWithPlan(context.Background(), session.SessionID, "/liubeiluo/vote", "投票系统", workspaceCtx, provider, nil, "", 1)
	if err != nil {
		t.Fatalf("build messages: %v", err)
	}
	toolNames := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolNames = append(toolNames, tool.Function.Name)
	}
	if containsWorkspaceRoleString(toolNames, "write_file") || containsWorkspaceRoleString(toolNames, "build_workspace") {
		t.Fatalf("LLM tools should hide role-forbidden write/build tools for QA role: %v", toolNames)
	}
	if !containsWorkspaceRoleString(toolNames, "run_form_submit") || !containsWorkspaceRoleString(toolNames, "run_chart_query") {
		t.Fatalf("LLM tools should expose QA role tools: %v", toolNames)
	}
	if !containsWorkspaceRoleString(plan.Tools.RoleAllowedTools, "run_form_submit") {
		t.Fatalf("QA role allowed tools should include run_form_submit: %#v", plan.Tools)
	}
	if containsWorkspaceRoleString(plan.Tools.RoleAllowedTools, "write_file") ||
		containsWorkspaceRoleString(plan.Tools.RoleAllowedTools, "build_workspace") {
		t.Fatalf("QA role allowed tools should still exclude write/build tools: %#v", plan.Tools)
	}
	if plan.Tools.LLMToolCount != len(toolNames) {
		t.Fatalf("LLM tool count = %d, want visible tool count %d", plan.Tools.LLMToolCount, len(toolNames))
	}
	if plan.Tools.RoleAllowedToolCount != plan.Tools.LLMToolCount {
		t.Fatalf("LLM tools should match role allowed tools for QA: %#v", plan.Tools)
	}
}

func TestBuildLLMMessagesWithPlanCurrentTurnMessageDoesNotPolluteFutureContext(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate sessions: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/vote",
		Source:        SourceWorkspace,
		SessionID:     "current-turn-session",
		Title:         "移动端处理",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusActive,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	oldMobileMsg := &model.AgentChatMessage{
		SessionID:    session.SessionID,
		Role:         RoleUser,
		Content:      "旧移动端通知约束",
		ContextUsage: MessageContextCurrentTurn,
		User:         "tester",
	}
	if err := messageRepo.Create(oldMobileMsg); err != nil {
		t.Fatalf("create old current turn message: %v", err)
	}
	oldMobileAssistantMsg := &model.AgentChatMessage{
		SessionID: session.SessionID,
		Role:      RoleAssistant,
		Content:   "旧移动端处理结果",
		User:      "tester",
	}
	if err := messageRepo.Create(oldMobileAssistantMsg); err != nil {
		t.Fatalf("create old current turn assistant message: %v", err)
	}
	currentMobileMsg := &model.AgentChatMessage{
		SessionID:    session.SessionID,
		Role:         RoleUser,
		Content:      "本轮移动端通知约束",
		ContextUsage: MessageContextCurrentTurn,
		User:         "tester",
	}
	if err := messageRepo.Create(currentMobileMsg); err != nil {
		t.Fatalf("create current turn message: %v", err)
	}
	svc := &WorkspaceChatService{
		toolReg:     NewToolRegistry(),
		sessionRepo: sessionRepo,
		messageRepo: messageRepo,
	}
	workspaceCtx := &dto.GetWorkspaceContextResp{}
	workspaceCtx.Directory.Name = "投票系统"
	workspaceCtx.Directory.Code = "vote"
	workspaceCtx.Directory.Type = "package"

	msgs, _, plan, err := svc.buildLLMMessagesWithPlan(context.Background(), session.SessionID, "/liubeiluo/vote", "投票系统", workspaceCtx, nil, nil, "fallback", 0, currentMobileMsg.ID)
	if err != nil {
		t.Fatalf("build messages for current turn: %v", err)
	}
	joined := joinLLMMessageContents(msgs)
	if strings.Contains(joined, "旧移动端通知约束") || strings.Contains(joined, "旧移动端处理结果") {
		t.Fatalf("old current_turn message should be excluded from current model context:\n%s", joined)
	}
	if !strings.Contains(joined, "本轮移动端通知约束") {
		t.Fatalf("current current_turn message should be included in current model context:\n%s", joined)
	}
	if plan == nil || plan.Messages.IncludedStoredMessages != 1 || plan.Messages.ExcludedStoredMessages != 2 {
		t.Fatalf("bad current-turn plan: %#v", plan)
	}

	msgs, _, plan, err = svc.buildLLMMessagesWithPlan(context.Background(), session.SessionID, "/liubeiluo/vote", "投票系统", workspaceCtx, nil, nil, "fallback", 1)
	if err != nil {
		t.Fatalf("build messages for future turn: %v", err)
	}
	joined = joinLLMMessageContents(msgs)
	if strings.Contains(joined, "旧移动端通知约束") ||
		strings.Contains(joined, "旧移动端处理结果") ||
		strings.Contains(joined, "本轮移动端通知约束") {
		t.Fatalf("current_turn messages should be excluded from future model context:\n%s", joined)
	}
	if plan == nil || plan.Messages.IncludedStoredMessages != 0 || plan.Messages.ExcludedStoredMessages != 3 {
		t.Fatalf("bad future-turn plan: %#v", plan)
	}
}

func TestBuildLLMMessagesWithPlanKeepsLargeHistoricalToolPayloadsVerbatim(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate sessions: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/assets",
		Source:        SourceWorkspace,
		SessionID:     "large-tool-history-session",
		Title:         "大工具历史",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusActive,
		RoleID:        WorkspaceRoleMaintenanceEngineer,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleUser, Content: "修改应用", User: "tester"}); err != nil {
		t.Fatalf("create user message: %v", err)
	}
	largeArgs := `{"full_code_path":"/liubeiluo/assets/main.go","content":"` + strings.Repeat("参数内容", 1200) + `"}`
	call := llms.ToolCall{ID: "call_write", Type: "function"}
	call.Function.Name = "write_file"
	call.Function.Arguments = largeArgs
	rawToolCalls, err := json.Marshal([]llms.ToolCall{call})
	if err != nil {
		t.Fatalf("marshal tool calls: %v", err)
	}
	toolCalls := string(rawToolCalls)
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleAssistant, ToolCalls: &toolCalls, User: "tester"}); err != nil {
		t.Fatalf("create assistant message: %v", err)
	}
	largeResult := strings.Repeat("工具结果", 1200)
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleTool, ToolCallID: "call_write", Content: largeResult, User: "tester"}); err != nil {
		t.Fatalf("create tool message: %v", err)
	}
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleUser, Content: "只展示给前端", ContextUsage: MessageContextDisplayOnly, User: "tester"}); err != nil {
		t.Fatalf("create display-only message: %v", err)
	}
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleUser, Content: "继续处理", User: "tester"}); err != nil {
		t.Fatalf("create current user message: %v", err)
	}
	svc := &WorkspaceChatService{sessionRepo: sessionRepo, messageRepo: messageRepo}
	workspaceCtx := &dto.GetWorkspaceContextResp{}
	workspaceCtx.Directory.Name = "资产管理"
	workspaceCtx.Directory.Code = "assets"
	workspaceCtx.Directory.Type = "package"

	msgs, _, plan, err := svc.buildLLMMessagesWithPlan(context.Background(), session.SessionID, "/liubeiluo/assets", "资产管理", workspaceCtx, nil, nil, "fallback", 0)
	if err != nil {
		t.Fatalf("build messages: %v", err)
	}
	joined := joinLLMMessageContents(msgs)
	if strings.Contains(joined, "只展示给前端") {
		t.Fatalf("display-only message should not enter model context:\n%s", joined)
	}
	if !strings.Contains(joined, largeResult) || strings.Contains(joined, "历史内容已截断") {
		t.Fatalf("large tool result should remain verbatim:\n%s", joined)
	}
	var gotArgs string
	for _, msg := range msgs {
		if msg.Role == RoleAssistant && len(msg.ToolCalls) > 0 {
			gotArgs = msg.ToolCalls[0].Function.Arguments
			break
		}
	}
	if gotArgs != largeArgs {
		t.Fatalf("large tool arguments should remain verbatim, got length=%d", len(gotArgs))
	}
	if plan == nil || plan.Messages.ExcludedDisplayOnly != 1 {
		t.Fatalf("display-only exclusion should be reported: %#v", plan)
	}
}

func TestBuildLLMMessagesWithPlanKeepsUserArtifactVerbatim(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate sessions: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/vote",
		Source:        SourceWorkspace,
		SessionID:     "artifact-user-ref-session",
		Title:         "artifact 引用会话",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusActive,
		RoleID:        WorkspaceRoleAppDeveloper,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	artifactJSON := `{"kind":"agent_app_prd","project":{"name":"投票系统","code":"vote","summary":"创建投票主题和结果统计"},"tables":[{"name":"投票主题","code":"topics","fields":[{"name":"主题标题"}],"examples":[{"主题标题":"artifact-secret-marker"}]}],"forms":[{"name":"提交投票","request_fields":[{"name":"选项"}],"response_fields":[{"name":"提交结果"}]}],"rules":["每人只能提交一次"]}`
	artifactMsg := &model.AgentChatMessage{
		SessionID:    session.SessionID,
		Role:         RoleUser,
		Content:      "AGENT_APP_PRD JSON:\n```json\n" + artifactJSON + "\n```",
		ContextUsage: MessageContextArtifact,
		ArtifactKind: "agent_app_prd",
		User:         "tester",
	}
	if err := messageRepo.Create(artifactMsg); err != nil {
		t.Fatalf("create artifact message: %v", err)
	}
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleUser, Content: "继续开发", User: "tester"}); err != nil {
		t.Fatalf("create current user message: %v", err)
	}
	svc := &WorkspaceChatService{toolReg: NewToolRegistry(), sessionRepo: sessionRepo, messageRepo: messageRepo}
	workspaceCtx := &dto.GetWorkspaceContextResp{}
	workspaceCtx.Directory.Name = "投票系统"
	workspaceCtx.Directory.Code = "vote"
	workspaceCtx.Directory.Type = "package"

	msgs, _, _, err := svc.buildLLMMessagesWithPlan(context.Background(), session.SessionID, "/liubeiluo/vote", "投票系统", workspaceCtx, nil, nil, "fallback", 0)
	if err != nil {
		t.Fatalf("build messages: %v", err)
	}
	joined := joinLLMMessageContents(msgs)
	if !strings.Contains(joined, "artifact-secret-marker") {
		t.Fatalf("full artifact JSON should enter model context:\n%s", joined)
	}
	if strings.Contains(joined, "workspace_artifact_ref") || strings.Contains(joined, "read_workspace_artifact") {
		t.Fatalf("artifact references should be disabled:\n%s", joined)
	}
}

func TestBuildLLMMessagesWithPlanKeepsWritePRDToolResultVerbatim(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate sessions: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/vote",
		Source:        SourceWorkspace,
		SessionID:     "artifact-tool-ref-session",
		Title:         "write_prd 引用会话",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusActive,
		RoleID:        WorkspaceRoleProductManager,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleUser, Content: "做一个投票系统", User: "tester"}); err != nil {
		t.Fatalf("create user message: %v", err)
	}
	call := llms.ToolCall{ID: "call_prd", Type: "function"}
	call.Function.Name = "write_prd"
	call.Function.Arguments = `{"project":{"name":"投票系统","code":"vote","summary":"创建投票"}}`
	rawToolCalls, err := json.Marshal([]llms.ToolCall{call})
	if err != nil {
		t.Fatalf("marshal tool calls: %v", err)
	}
	toolCalls := string(rawToolCalls)
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleAssistant, ToolCalls: &toolCalls, User: "tester"}); err != nil {
		t.Fatalf("create assistant message: %v", err)
	}
	artifactJSON := `{"kind":"agent_app_prd","schema_version":"prd.v2","project":{"name":"投票系统","code":"vote","summary":"创建投票"},"tables":[{"name":"投票主题","fields":[{"name":"主题标题"}],"examples":[{"主题标题":"tool-secret-marker"}]}]}`
	if err := messageRepo.Create(&model.AgentChatMessage{
		SessionID:  session.SessionID,
		Role:       RoleTool,
		ToolCallID: "call_prd",
		ToolStatus: ToolCallStatusOK,
		Content:    "PRD 已生成，请确认。\n\n" + artifactJSON,
		ResultData: &artifactJSON,
		User:       "tester",
	}); err != nil {
		t.Fatalf("create tool message: %v", err)
	}
	svc := &WorkspaceChatService{toolReg: NewToolRegistry(), sessionRepo: sessionRepo, messageRepo: messageRepo}
	workspaceCtx := &dto.GetWorkspaceContextResp{}
	workspaceCtx.Directory.Name = "投票系统"
	workspaceCtx.Directory.Code = "vote"
	workspaceCtx.Directory.Type = "package"

	msgs, _, _, err := svc.buildLLMMessagesWithPlan(context.Background(), session.SessionID, "/liubeiluo/vote", "投票系统", workspaceCtx, nil, nil, "fallback", 0)
	if err != nil {
		t.Fatalf("build messages: %v", err)
	}
	joined := joinLLMMessageContents(msgs)
	if !strings.Contains(joined, "tool-secret-marker") {
		t.Fatalf("full write_prd result should enter model context:\n%s", joined)
	}
	if strings.Contains(joined, "workspace_artifact_ref") || strings.Contains(joined, "read_workspace_artifact") {
		t.Fatalf("write_prd result should not be replaced by an artifact reference:\n%s", joined)
	}
}

func TestReadWorkspaceArtifactReturnsPrimaryArtifactJSONAndChecksSession(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate sessions: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/vote",
		Source:        SourceWorkspace,
		SessionID:     "artifact-read-session",
		Title:         "artifact 读取会话",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusActive,
		RoleID:        WorkspaceRoleAppDeveloper,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	otherSession := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/other",
		Source:        SourceWorkspace,
		SessionID:     "other-artifact-session",
		Title:         "其他会话",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusActive,
		RoleID:        WorkspaceRoleAppDeveloper,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := sessionRepo.Create(otherSession); err != nil {
		t.Fatalf("create other session: %v", err)
	}
	artifactJSON := `{"kind":"agent_app_prd","project":{"name":"投票系统","code":"vote","summary":"创建投票"},"tables":[{"name":"投票主题","fields":[{"name":"主题标题"}],"examples":[{"主题标题":"read-secret-marker"}]}]}`
	msg := &model.AgentChatMessage{
		SessionID:    session.SessionID,
		Role:         RoleUser,
		Content:      "HANDOFF_PACKET JSON:\n```json\n{\"artifact\":{\"kind\":\"agent_app_prd\"}}\n```\n\nAGENT_APP_PRD JSON:\n```json\n" + artifactJSON + "\n```",
		ContextUsage: MessageContextArtifact,
		ArtifactKind: "agent_app_prd",
		User:         "tester",
	}
	if err := messageRepo.Create(msg); err != nil {
		t.Fatalf("create artifact message: %v", err)
	}
	svc := &WorkspaceChatService{toolReg: NewToolRegistry(), sessionRepo: sessionRepo, messageRepo: messageRepo}
	ctx := contextx.WithWorkspaceSession(contextx.WithRequestUser(context.Background(), "tester"), session.SessionID, "artifact 读取会话", WorkspaceRoleAppDeveloper)

	result := svc.readWorkspaceArtifactTool(ctx, map[string]interface{}{"message_id": float64(msg.ID)})
	if result.IsError {
		t.Fatalf("read workspace artifact returned error: %s", result.Content)
	}
	data, ok := result.Data.(readWorkspaceArtifactResultData)
	if !ok {
		t.Fatalf("result data type = %T, want readWorkspaceArtifactResultData", result.Data)
	}
	if data.Source != "artifact_json" || !strings.Contains(data.Text, "read-secret-marker") || strings.Contains(data.Text, "HANDOFF_PACKET") {
		t.Fatalf("read should return primary artifact JSON, got source=%s text=%s", data.Source, data.Text)
	}
	blocked := svc.readWorkspaceArtifactTool(contextx.WithWorkspaceSession(context.Background(), otherSession.SessionID, "其他会话", WorkspaceRoleAppDeveloper), map[string]interface{}{"message_id": float64(msg.ID)})
	if !blocked.IsError || !strings.Contains(blocked.Content, "当前工作台会话") {
		t.Fatalf("cross-session read should be blocked, got %#v", blocked)
	}
}

func TestBuildLLMMessagesWithPlanKeepsLongHistoryDespiteBudget(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}, &model.AgentChatContextCheckpoint{}); err != nil {
		t.Fatalf("migrate sessions: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/assets",
		Source:        SourceWorkspace,
		SessionID:     "budget-reduction-session",
		Title:         "预算裁剪会话",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusActive,
		RoleID:        WorkspaceRoleMaintenanceEngineer,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	for i := 0; i < 80; i++ {
		content := strings.Repeat("旧历史内容", 320)
		if i == 0 {
			content += " old-marker-00"
		}
		if i == 79 {
			content = strings.Repeat("较新的历史内容", 320) + " recent-marker-79"
		}
		if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleUser, Content: content, User: "tester"}); err != nil {
			t.Fatalf("create old message %d: %v", i, err)
		}
	}
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleUser, Content: "current-marker 保留当前需求", User: "tester"}); err != nil {
		t.Fatalf("create current user message: %v", err)
	}
	svc := &WorkspaceChatService{sessionRepo: sessionRepo, messageRepo: messageRepo}
	workspaceCtx := &dto.GetWorkspaceContextResp{}
	workspaceCtx.Directory.Name = "资产管理"
	workspaceCtx.Directory.Code = "assets"
	workspaceCtx.Directory.Type = "package"

	msgs, _, plan, err := svc.buildLLMMessagesWithPlanAndOptions(context.Background(), session.SessionID, "/liubeiluo/assets", "资产管理", workspaceCtx, nil, nil, "fallback", 0, workspaceLLMContextBuildOptions{
		ContextWindowTokens: 32000,
	}, 0)
	if err != nil {
		t.Fatalf("build messages: %v", err)
	}
	if plan == nil || plan.Budget == nil {
		t.Fatalf("budget should be reported: %#v", plan)
	}
	if plan.Messages.ExcludedByCheckpoint != 0 || plan.Messages.RecoverableHistory || plan.Messages.IncludedStoredMessages != 81 {
		t.Fatalf("all stored history should remain in model context: %#v", plan.Messages)
	}
	if plan.Messages.ExcludedByReduction != 0 {
		t.Fatalf("checkpoint compaction should not drop messages by count: %#v", plan.Messages)
	}
	joined := joinLLMMessageContents(msgs)
	if strings.Contains(joined, "<conversation_checkpoint") {
		t.Fatalf("model context should not contain a conversation checkpoint")
	}
	for _, marker := range []string{"old-marker-00", "recent-marker-79", "current-marker"} {
		if !strings.Contains(joined, marker) {
			t.Fatalf("history marker %q should remain in model context", marker)
		}
	}
	if _, err := messageRepo.GetLatestContextCheckpoint(session.SessionID); err == nil {
		t.Fatal("building model context should not create a checkpoint")
	}
	oldest, err := messageRepo.GetByID(1)
	if err != nil || !strings.Contains(oldest.Content, "old-marker-00") {
		t.Fatalf("raw oldest message must remain unchanged: %#v err=%v", oldest, err)
	}
}

func TestBuildLLMMessagesWithPlanSynthesizesMissingToolResultAfterCancel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate sessions: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/assets",
		Source:        SourceWorkspace,
		SessionID:     "cancelled-tool-session",
		Title:         "中断工具会话",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusActive,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleUser, Content: "测试资产列表", User: "tester"}); err != nil {
		t.Fatalf("create user message: %v", err)
	}
	call := llms.ToolCall{ID: "call_search", Type: "function"}
	call.Function.Name = "run_table_search"
	call.Function.Arguments = `{"full_code_path":"/system/demos/ai_asset_manager/asset_list.table"}`
	rawToolCalls, err := json.Marshal([]llms.ToolCall{call})
	if err != nil {
		t.Fatalf("marshal tool calls: %v", err)
	}
	toolCalls := string(rawToolCalls)
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleAssistant, ToolCalls: &toolCalls, User: "tester"}); err != nil {
		t.Fatalf("create assistant message: %v", err)
	}
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleUser, Content: "继续测试", User: "tester"}); err != nil {
		t.Fatalf("create next user message: %v", err)
	}
	svc := &WorkspaceChatService{sessionRepo: sessionRepo, messageRepo: messageRepo}
	workspaceCtx := &dto.GetWorkspaceContextResp{}
	workspaceCtx.Directory.Name = "资产管理"
	workspaceCtx.Directory.Code = "assets"
	workspaceCtx.Directory.Type = "package"

	msgs, _, _, err := svc.buildLLMMessagesWithPlan(context.Background(), session.SessionID, "/liubeiluo/assets", "资产管理", workspaceCtx, nil, nil, "fallback", 0)
	if err != nil {
		t.Fatalf("build messages: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("llm messages = %d, want system/user/assistant/synthetic-tool/user: %#v", len(msgs), msgs)
	}
	if msgs[2].Role != RoleAssistant || len(msgs[2].ToolCalls) != 1 {
		t.Fatalf("assistant tool_calls missing: %#v", msgs[2])
	}
	if msgs[3].Role != RoleTool || msgs[3].ToolCallID != "call_search" || !strings.Contains(msgs[3].Content, "被用户中断") {
		t.Fatalf("synthetic tool result = %#v, want interrupted tool result", msgs[3])
	}
	if msgs[4].Role != RoleUser || !strings.Contains(msgs[4].Content, "继续测试") {
		t.Fatalf("next user message = %#v", msgs[4])
	}
}

func TestBuildLLMMessagesWithPlanPreservesHistoryDespiteLegacyAnchor(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate sessions: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/assets",
		Source:        SourceWorkspace,
		SessionID:     "anchor-orphan-tool-session",
		Title:         "历史保留工具会话",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusActive,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleUser, Content: "旧消息", User: "tester"}); err != nil {
		t.Fatalf("create old user message: %v", err)
	}
	call := llms.ToolCall{ID: "call_search", Type: "function"}
	call.Function.Name = "run_table_search"
	call.Function.Arguments = `{}`
	rawToolCalls, err := json.Marshal([]llms.ToolCall{call})
	if err != nil {
		t.Fatalf("marshal tool calls: %v", err)
	}
	toolCalls := string(rawToolCalls)
	assistantMsg := &model.AgentChatMessage{SessionID: session.SessionID, Role: RoleAssistant, ToolCalls: &toolCalls, User: "tester"}
	if err := messageRepo.Create(assistantMsg); err != nil {
		t.Fatalf("create assistant message: %v", err)
	}
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleTool, ToolCallID: "call_search", Content: "{}", User: "tester"}); err != nil {
		t.Fatalf("create tool message: %v", err)
	}
	if err := messageRepo.Create(&model.AgentChatMessage{SessionID: session.SessionID, Role: RoleUser, Content: "锚点后继续", User: "tester"}); err != nil {
		t.Fatalf("create current user message: %v", err)
	}
	session.ModelContextAnchorMessageID = assistantMsg.ID
	if err := sessionRepo.Update(session); err != nil {
		t.Fatalf("update anchor: %v", err)
	}
	svc := &WorkspaceChatService{sessionRepo: sessionRepo, messageRepo: messageRepo}
	workspaceCtx := &dto.GetWorkspaceContextResp{}
	workspaceCtx.Directory.Name = "资产管理"
	workspaceCtx.Directory.Code = "assets"
	workspaceCtx.Directory.Type = "package"

	msgs, _, _, err := svc.buildLLMMessagesWithPlan(context.Background(), session.SessionID, "/liubeiluo/assets", "资产管理", workspaceCtx, nil, nil, "fallback", 0)
	if err != nil {
		t.Fatalf("build messages: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("llm messages = %d, want system + full historical tool sequence + current user: %#v", len(msgs), msgs)
	}
	joined := joinLLMMessageContents(msgs)
	if !strings.Contains(joined, "旧消息") || !strings.Contains(joined, "锚点后继续") {
		t.Fatalf("history should be preserved despite legacy anchor: %#v", msgs)
	}
	if msgs[2].Role != RoleAssistant || len(msgs[2].ToolCalls) != 1 || msgs[3].Role != RoleTool {
		t.Fatalf("historical tool sequence should remain valid: %#v", msgs)
	}
}

func TestAttachLLMUsageToWorkspaceModelContextPlanReportsCacheResult(t *testing.T) {
	plan := &dto.WorkspaceModelContextPlan{
		CachePlan: dto.WorkspaceModelContextCachePlan{
			StablePrefixStrategy: "stable_mode_tools_static_system_before_workspace_env_with_prompt_cache_key",
			ActualUsageField:     "assistant.llm_usage.cached_tokens",
		},
	}

	attachLLMUsageToWorkspaceModelContextPlan(plan, &llms.Usage{
		PromptTokens:         1200,
		CompletionTokens:     80,
		TotalTokens:          1280,
		CachedTokens:         1024,
		CachedTokensReported: true,
	})

	if plan.CachePlan.Result == nil {
		t.Fatal("cache result is nil")
	}
	if plan.CachePlan.Result.Status != "hit" ||
		plan.CachePlan.Result.CachedTokens != 1024 ||
		plan.CachePlan.Result.CacheHitRatePercent != 85 ||
		!plan.CachePlan.Result.CachedTokensReported {
		t.Fatalf("unexpected cache result: %#v", plan.CachePlan.Result)
	}

	attachLLMUsageToWorkspaceModelContextPlan(plan, &llms.Usage{
		PromptTokens:         1200,
		CompletionTokens:     80,
		TotalTokens:          1280,
		CachedTokensReported: false,
	})
	if plan.CachePlan.Result.Status != "not_reported" || plan.CachePlan.Result.CachedTokensReported {
		t.Fatalf("not_reported cache result mismatch: %#v", plan.CachePlan.Result)
	}
}

func TestCreateWorkspaceHandoffInjectsIntoCurrentSessionAndPreservesHistory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	if err := db.AutoMigrate(&model.WorkspaceHandoffPacket{}); err != nil {
		t.Fatalf("migrate handoff packets: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	handoffRepo := repository.NewWorkspaceHandoffPacketRepository(db)
	source := &model.AgentChatSession{
		TreeID:          7,
		FullCodePath:    "/liubeiluo/demo",
		Source:          SourceWorkspace,
		SessionID:       "source-session",
		Title:           "PRD 讨论",
		ModeCode:        "dev",
		Status:          model.ChatSessionStatusActive,
		RoleID:          WorkspaceRoleProductManager,
		RoleDisplayName: "产品经理",
		ContextPolicy:   ContextPolicyFull,
		User:            "tester",
	}
	if err := sessionRepo.Create(source); err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := messageRepo.Create(&model.AgentChatMessage{
		SessionID: "source-session",
		Role:      RoleUser,
		Content:   "帮我做一个 NPS 回访问卷系统，优先让门店经理能快速提交，不要复杂审批。",
		User:      "tester",
	}); err != nil {
		t.Fatalf("create source user message: %v", err)
	}
	if err := messageRepo.Create(&model.AgentChatMessage{
		SessionID: "source-session",
		Role:      RoleUser,
		Content:   "字段需要评分、原因和门店；图表要按日期看趋势，后续可能按门店筛选。",
		User:      "tester",
	}); err != nil {
		t.Fatalf("create source user message: %v", err)
	}

	svc := &WorkspaceChatService{sessionRepo: sessionRepo, messageRepo: messageRepo}
	ctx := context.WithValue(context.Background(), contextx.RequestUserHeader, "tester")
	artifact := []byte(`{
		"kind":"agent_app_prd",
		"project":{"name":"NPS 回访","code":"nps_followup","summary":"记录门店回访评分并查看趋势"},
		"tables":[{"name":"满意度记录","fields":[{"name":"门店"},{"name":"评分"},{"name":"原因"}],"search_fields":[{"name":"创建开始时间"},{"name":"创建结束时间"},{"name":"门店"}],"handlers":[]}],
		"forms":[{"name":"提交评分","target_table":"满意度记录","request_fields":[{"name":"门店"},{"name":"评分"},{"name":"原因"}]}],
		"charts":[{"name":"NPS 趋势","source_table":"满意度记录","chart_type":"line","dimension":"日期","metrics":["平均评分"]}],
		"rules":["记录表默认只读，不要补人工新增编辑删除"]
	}`)
	resp, err := svc.CreateWorkspaceHandoff(ctx, &dto.WorkspaceHandoffReq{
		SourceSessionID: "source-session",
		FullCodePath:    "/liubeiluo/demo",
		TargetRole:      WorkspaceRoleAppDeveloper,
		ArtifactKind:    "agent_app_prd",
		Artifact:        artifact,
		Remark:          "优先做列表",
	})
	if err != nil {
		t.Fatalf("handoff: %v", err)
	}
	if resp.SessionID != source.SessionID {
		t.Fatalf("handoff should stay in source session, got %q want %q", resp.SessionID, source.SessionID)
	}
	if resp.ContextPolicy != ContextPolicyFull {
		t.Fatalf("context policy=%q want %q", resp.ContextPolicy, ContextPolicyFull)
	}
	if resp.MessageID == 0 {
		t.Fatal("expected handoff response to include initial message id")
	}
	if resp.HandoffPacketID == 0 {
		t.Fatal("expected handoff response to include handoff packet id")
	}
	if !strings.Contains(resp.Content, "target_role 固定为 app_developer") {
		t.Fatalf("content should include target role, got %q", resp.Content)
	}
	if !strings.Contains(resp.Content, "tables.search_fields 是查询请求字段") {
		t.Fatalf("content should include PRD v2 search field handoff rule, got %q", resp.Content)
	}
	if !strings.Contains(resp.Content, "PRD_EXECUTION_MARKDOWN") ||
		!strings.Contains(resp.Content, "| 字段 | 组件 | 必填 | 说明 | 展示限制 |") ||
		!strings.Contains(resp.Content, "## 业务规则与复杂逻辑") {
		t.Fatalf("content should include rendered PRD execution markdown, got %q", resp.Content)
	}
	if strings.Contains(resp.Content, `"prd_execution_markdown"`) {
		t.Fatalf("content should not duplicate PRD execution markdown inside context JSON, got %q", resp.Content)
	}
	if !strings.Contains(resp.Content, `"kind": "agent_app_prd"`) {
		t.Fatalf("content should include formatted artifact JSON, got %q", resp.Content)
	}
	if !strings.Contains(resp.Content, "HANDOFF_CONTEXT JSON") || !strings.Contains(resp.Content, "latest_user_notes") {
		t.Fatalf("content should include rich handoff context, got %q", resp.Content)
	}
	if !strings.Contains(resp.Content, "HANDOFF_PACKET JSON") ||
		!strings.Contains(resp.Content, `"version": "role_handoff.v1"`) ||
		!strings.Contains(resp.Content, `"execute_directory": "/liubeiluo/demo"`) {
		t.Fatalf("content should include typed handoff packet, got %q", resp.Content)
	}
	if !strings.Contains(resp.Content, "不要复杂审批") || !strings.Contains(resp.Content, "满意度记录") {
		t.Fatalf("handoff context should preserve source constraints and artifact digest, got %q", resp.Content)
	}
	if !strings.Contains(resp.HandoffContext, "implementation_focus") || !strings.Contains(resp.HandoffContext, "图表要按日期看趋势") {
		t.Fatalf("response should include rich handoff context, got %q", resp.HandoffContext)
	}
	if !strings.Contains(resp.HandoffContext, "prd_execution_markdown") ||
		!strings.Contains(resp.HandoffContext, "记录表默认只读") {
		t.Fatalf("handoff context should carry PRD execution markdown, got %q", resp.HandoffContext)
	}
	if !strings.Contains(resp.HandoffContext, `"executed_hooks"`) ||
		!strings.Contains(resp.HandoffContext, workspaceRoleHookProductManagerToDeveloper) {
		t.Fatalf("handoff context should expose executed role hooks, got %q", resp.HandoffContext)
	}
	if !strings.Contains(resp.HandoffContext, `"target_app_directory": "/liubeiluo/demo/nps_followup"`) ||
		!strings.Contains(resp.HandoffContext, `"execute_directory": "/liubeiluo/demo"`) ||
		!strings.Contains(resp.HandoffContext, `"artifact_included": true`) {
		t.Fatalf("handoff context should expose target app dir, execute dir and artifact status, got %q", resp.HandoffContext)
	}
	var parsedContext workspaceHandoffContext
	if err := json.Unmarshal([]byte(resp.HandoffContext), &parsedContext); err != nil {
		t.Fatalf("handoff context should be valid JSON: %v\n%s", err, resp.HandoffContext)
	}
	if parsedContext.HandoffPacket == nil {
		t.Fatalf("handoff context should include typed handoff packet, got %#v", parsedContext)
	}
	typedPacket := parsedContext.HandoffPacket
	if typedPacket.Version != workspaceRoleHandoffPacketVersion ||
		typedPacket.SourceRole != WorkspaceRoleProductManager ||
		typedPacket.TargetRole != WorkspaceRoleAppDeveloper ||
		typedPacket.ExecuteDirectory != "/liubeiluo/demo" ||
		typedPacket.TargetAppDirectory != "/liubeiluo/demo/nps_followup" {
		t.Fatalf("typed handoff packet metadata wrong: %#v", typedPacket)
	}
	if typedPacket.Artifact == nil || typedPacket.Artifact.Kind != "agent_app_prd" || typedPacket.Artifact.Source != "AGENT_APP_PRD JSON" {
		t.Fatalf("typed handoff packet should reference full artifact block, got %#v", typedPacket.Artifact)
	}
	if typedPacket.ArtifactDigest == nil || typedPacket.ArtifactDigest.ProjectCode != "nps_followup" || len(typedPacket.ArtifactDigest.Tables) != 1 {
		t.Fatalf("typed handoff packet should carry compact artifact digest, got %#v", typedPacket.ArtifactDigest)
	}
	if typedPacket.BuildDiagnostics != nil {
		t.Fatalf("PRD to developer packet should not carry build diagnostics, got %#v", typedPacket.BuildDiagnostics)
	}
	typedPacketJSON := formatWorkspaceRoleHandoffPacketJSON(typedPacket)
	if strings.Contains(typedPacketJSON, "prd_execution_markdown") ||
		strings.Contains(typedPacketJSON, `"project":`) {
		t.Fatalf("typed packet should not inline rendered PRD markdown or full artifact JSON, got %s", typedPacketJSON)
	}
	if !strings.Contains(resp.HandoffContext, "reference_docs") || !strings.Contains(resp.HandoffContext, "/system/prompt/sdk/agent-app-sdk-readme") || !strings.Contains(resp.HandoffContext, "/system/prompt/case_catalog") {
		t.Fatalf("handoff context should include recommended reference docs, got %q", resp.HandoffContext)
	}

	updatedSource, err := sessionRepo.GetBySessionID("source-session")
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if updatedSource.ArchivedForModel || updatedSource.ContextPolicy != ContextPolicyFull || updatedSource.ModelContextAnchorMessageID != 0 {
		t.Fatalf("source should keep full model context: %#v", updatedSource)
	}
	if updatedSource.Status != model.ChatSessionStatusActive {
		t.Fatalf("source status = %q, want %q", updatedSource.Status, model.ChatSessionStatusActive)
	}
	if updatedSource.ParentSessionID != "" || updatedSource.HandoffKind != "agent_app_prd" || updatedSource.HandoffTargetRole != WorkspaceRoleAppDeveloper {
		t.Fatalf("source handoff metadata wrong: %#v", updatedSource)
	}
	if updatedSource.RoleID != WorkspaceRoleAppDeveloper || updatedSource.RoleDisplayName != "应用开发工程师" {
		t.Fatalf("source role metadata wrong: %#v", updatedSource)
	}
	messages, err := messageRepo.ListBySessionID(resp.SessionID)
	if err != nil {
		t.Fatalf("list session messages: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected two historical messages plus one handoff message, got %d", len(messages))
	}
	injected := messages[len(messages)-1]
	if injected.ID != resp.MessageID || injected.Role != RoleUser || injected.ContextUsage != MessageContextArtifact || injected.ArtifactKind != "agent_app_prd" {
		t.Fatalf("injected handoff message metadata wrong: %#v", injected)
	}
	if !strings.Contains(injected.Content, `"kind": "agent_app_prd"`) || !strings.Contains(injected.DisplayContent, "优先做列表") {
		t.Fatalf("injected handoff message content wrong: %#v", injected)
	}
	packet, err := handoffRepo.GetByTargetSessionID(resp.SessionID)
	if err != nil {
		t.Fatalf("get handoff packet: %v", err)
	}
	if packet.ID != resp.HandoffPacketID || packet.SourceSessionID != source.SessionID || packet.TargetSessionID != source.SessionID || packet.InitialMessageID != resp.MessageID {
		t.Fatalf("handoff packet metadata wrong: %#v", packet)
	}
	if packet.TargetRole != WorkspaceRoleAppDeveloper || packet.ArtifactKind != "agent_app_prd" || !strings.Contains(packet.ArtifactJSON, `"project"`) {
		t.Fatalf("handoff packet payload wrong: %#v", packet)
	}
	if !strings.Contains(packet.HandoffContextJSON, "product_manager_to_app_developer") || !strings.Contains(packet.HandoffContextJSON, "记录表默认只读") {
		t.Fatalf("handoff packet context wrong: %#v", packet)
	}
	if !strings.Contains(packet.HandoffContextJSON, `"handoff_packet"`) || !strings.Contains(packet.HandoffContextJSON, `"version": "role_handoff.v1"`) {
		t.Fatalf("stored handoff context should preserve typed packet: %#v", packet)
	}
	if !strings.Contains(packet.HandoffContextJSON, "reference_files") || !strings.Contains(packet.HandoffContextJSON, "/liubeiluo/demo") {
		t.Fatalf("handoff packet should preserve reference files: %#v", packet)
	}

	dup, err := svc.CreateWorkspaceHandoff(ctx, &dto.WorkspaceHandoffReq{
		SourceSessionID: "source-session",
		FullCodePath:    "/liubeiluo/demo",
		TargetRole:      WorkspaceRoleAppDeveloper,
		ArtifactKind:    "agent_app_prd",
		Artifact:        artifact,
		Remark:          "重复确认",
	})
	if err != nil {
		t.Fatalf("duplicate handoff should return existing target: %v", err)
	}
	if dup.SessionID != resp.SessionID || dup.HandoffPacketID != resp.HandoffPacketID || dup.MessageID != resp.MessageID {
		t.Fatalf("duplicate handoff should be idempotent, first=%#v dup=%#v", resp, dup)
	}
	var handoffCount int64
	if err := db.Model(&model.WorkspaceHandoffPacket{}).Where("source_session_id = ?", "source-session").Count(&handoffCount).Error; err != nil {
		t.Fatalf("count handoff packets: %v", err)
	}
	if handoffCount != 1 {
		t.Fatalf("duplicate handoff should not create another packet, got %d", handoffCount)
	}
	var currentSessionCount int64
	if err := db.Model(&model.AgentChatSession{}).Where("session_id = ?", "source-session").Count(&currentSessionCount).Error; err != nil {
		t.Fatalf("count current sessions: %v", err)
	}
	if currentSessionCount != 1 {
		t.Fatalf("handoff should keep one current session row, got %d", currentSessionCount)
	}
	var childCount int64
	if err := db.Model(&model.AgentChatSession{}).Where("parent_session_id = ?", "source-session").Count(&childCount).Error; err != nil {
		t.Fatalf("count child sessions: %v", err)
	}
	if childCount != 0 {
		t.Fatalf("handoff should not create child sessions, got %d", childCount)
	}
}

func TestWorkspaceSessionAccessIsScopedToCurrentUser(t *testing.T) {
	const fullCodePath = "/system/x_world/vote"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/health":
			w.WriteHeader(http.StatusOK)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/workspace/api/v1/workspace/context":
		default:
			t.Fatalf("unexpected workspace context request: %s %s", r.Method, r.URL.String())
		}
		if got := r.URL.Query().Get("full_code_path"); got != fullCodePath {
			t.Fatalf("full_code_path = %q, want %q", got, fullCodePath)
		}
		writeToolAPIResponse(t, w, map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": dto.GetWorkspaceContextResp{
				Directory: dto.WorkspaceContextDirectory{
					Name:         "投票",
					Code:         "vote",
					FullCodePath: fullCodePath,
					Type:         "package",
				},
			},
		})
	}))
	defer server.Close()
	t.Setenv("GATEWAY_URL", server.URL)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	for _, session := range []*model.AgentChatSession{
		{
			TreeID:        1,
			FullCodePath:  "/system/x_world/vote",
			Source:        SourceWorkspace,
			SessionID:     "alice-session",
			Title:         "Alice 会话",
			ModeCode:      "dev",
			Status:        model.ChatSessionStatusActive,
			ContextPolicy: ContextPolicyFull,
			User:          "alice",
		},
		{
			TreeID:        1,
			FullCodePath:  "/system/x_world/vote",
			Source:        SourceWorkspace,
			SessionID:     "bob-session",
			Title:         "Bob 会话",
			ModeCode:      "dev",
			Status:        model.ChatSessionStatusGenerating,
			ContextPolicy: ContextPolicyFull,
			User:          "bob",
		},
	} {
		if err := sessionRepo.Create(session); err != nil {
			t.Fatalf("create session: %v", err)
		}
	}
	if err := messageRepo.Create(&model.AgentChatMessage{
		SessionID: "alice-session",
		Role:      RoleUser,
		Content:   "查询投票主题",
		User:      "alice",
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}
	svc := &WorkspaceChatService{sessionRepo: sessionRepo, messageRepo: messageRepo}
	ctx := context.WithValue(context.Background(), contextx.RequestUserHeader, "alice")

	sessions, total, err := svc.ListSessions(ctx, fullCodePath, 1, 20)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if total != 1 || len(sessions) != 1 || sessions[0].SessionID != "alice-session" {
		t.Fatalf("list sessions should only return current user's sessions, total=%d sessions=%#v", total, sessions)
	}
	if _, err := svc.ListMessages(ctx, "alice-session"); err != nil {
		t.Fatalf("list own messages: %v", err)
	}
	if _, err := svc.ListMessages(ctx, "bob-session"); err == nil || !strings.Contains(err.Error(), "不能操作其他用户的会话") {
		t.Fatalf("list other user's messages should fail, got %v", err)
	}
	if err := svc.CancelSession(ctx, "bob-session"); err == nil || !strings.Contains(err.Error(), "不能操作其他用户的会话") {
		t.Fatalf("cancel other user's session should fail, got %v", err)
	}

	marked, err := sessionRepo.TryMarkGenerating("alice-session", "alice", "dev")
	if err != nil || !marked {
		t.Fatalf("first TryMarkGenerating should succeed, marked=%v err=%v", marked, err)
	}
	marked, err = sessionRepo.TryMarkGenerating("alice-session", "alice", "dev")
	if err != nil {
		t.Fatalf("second TryMarkGenerating returned error: %v", err)
	}
	if marked {
		t.Fatal("second TryMarkGenerating should not mark an already generating session")
	}
}

func TestPersistWorkspaceSessionInteractionStatusKeepsPRDAsOutput(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/demo",
		Source:        SourceWorkspace,
		SessionID:     "pending-session",
		Title:         "PRD 讨论",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusGenerating,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc := &WorkspaceChatService{sessionRepo: sessionRepo}
	svc.persistWorkspaceSessionInteractionStatus(context.Background(), "pending-session", []streamloop.ToolCallSummary{
		{
			Name:   "write_prd",
			Status: ToolCallStatusOK,
			ResultData: map[string]interface{}{
				"kind": "agent_app_prd",
				"interaction": map[string]interface{}{
					"status":    model.ChatSessionStatusPendingConfirmation,
					"card_type": "prd_confirmation",
				},
			},
		},
	}, "tester")

	updated, err := sessionRepo.GetBySessionID("pending-session")
	if err != nil {
		t.Fatalf("get updated session: %v", err)
	}
	if updated.Status != model.ChatSessionStatusOutput {
		t.Fatalf("status = %q, want %q", updated.Status, model.ChatSessionStatusOutput)
	}
}

func TestPersistWorkspaceSessionInteractionStatusIgnoresBuildRepairFromErrorTool(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/demo",
		Source:        SourceWorkspace,
		SessionID:     "pending-build-repair-session",
		Title:         "构建失败",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusGenerating,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc := &WorkspaceChatService{sessionRepo: sessionRepo}
	svc.persistWorkspaceSessionInteractionStatus(context.Background(), "pending-build-repair-session", []streamloop.ToolCallSummary{
		{
			Name:   "build_workspace",
			Status: ToolCallStatusError,
			ResultData: map[string]interface{}{
				"kind": "agent_app_build_failure",
				"interaction": map[string]interface{}{
					"status":    model.ChatSessionStatusPendingBuildRepair,
					"card_type": "build_repair",
				},
			},
		},
	}, "tester")

	updated, err := sessionRepo.GetBySessionID("pending-build-repair-session")
	if err != nil {
		t.Fatalf("get updated session: %v", err)
	}
	if updated.Status != model.ChatSessionStatusGenerating {
		t.Fatalf("status = %q, want unchanged %q", updated.Status, model.ChatSessionStatusGenerating)
	}
}

func TestPersistWorkspaceSessionInteractionStatusMarksOutput(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/demo",
		Source:        SourceWorkspace,
		SessionID:     "output-session",
		Title:         "生成代码",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusGenerating,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc := &WorkspaceChatService{sessionRepo: sessionRepo}
	svc.persistWorkspaceSessionInteractionStatus(context.Background(), "output-session", []streamloop.ToolCallSummary{
		{Name: "write_file", Status: ToolCallStatusOK},
	}, "tester")

	updated, err := sessionRepo.GetBySessionID("output-session")
	if err != nil {
		t.Fatalf("get updated session: %v", err)
	}
	if updated.Status != model.ChatSessionStatusOutput {
		t.Fatalf("status = %q, want %q", updated.Status, model.ChatSessionStatusOutput)
	}
}

func TestResolveWorkspacePendingInteractionClearsPending(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/demo",
		Source:        SourceWorkspace,
		SessionID:     "pending-test-session",
		Title:         "构建结果",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusPendingTest,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc := &WorkspaceChatService{sessionRepo: sessionRepo}
	ctx := context.WithValue(context.Background(), contextx.RequestUserHeader, "tester")
	if err := svc.ResolveWorkspacePendingInteraction(ctx, "pending-test-session"); err != nil {
		t.Fatalf("resolve pending interaction: %v", err)
	}

	updated, err := sessionRepo.GetBySessionID("pending-test-session")
	if err != nil {
		t.Fatalf("get updated session: %v", err)
	}
	if updated.Status != model.ChatSessionStatusActive {
		t.Fatalf("status = %q, want %q", updated.Status, model.ChatSessionStatusActive)
	}
}

func TestWorkspacePendingInteractionFromMessagesBuildsGenericInteraction(t *testing.T) {
	resultData := `{
		"kind": "agent_app_prd",
		"project": {"name": "NPS", "code": "nps"},
		"interaction": {
			"card_type": "prd_confirmation",
			"artifact_kind": "agent_app_prd",
			"status": "pending_confirmation",
			"blocking": true,
			"title": "PRD 等待确认",
			"description": "确认后进入开发",
			"target_role_on_confirm": "app_developer",
			"allowed_actions": ["confirm_prd", "revise_prd", "cancel_prd", "view_prd"],
			"confirm_text": "确认 PRD"
		}
	}`
	messages := []*model.AgentChatMessage{
		{Role: RoleTool, ResultData: &resultData},
	}

	interaction := workspacePendingInteractionFromMessages(model.ChatSessionStatusPendingConfirmation, messages)
	if interaction == nil {
		t.Fatal("interaction should be derived")
	}
	if interaction.CardType != "prd_confirmation" || interaction.ArtifactKind != "agent_app_prd" || !interaction.Blocking {
		t.Fatalf("unexpected interaction: %+v", interaction)
	}
	if !workspaceInteractionAllowsAction(interaction, "revise_prd") {
		t.Fatal("revise_prd should be allowed")
	}
	if workspaceInteractionActionCanRunModel("confirm_prd") {
		t.Fatal("confirm_prd should not enter model loop directly")
	}
	if !workspaceInteractionActionCanRunModel("revise_prd") {
		t.Fatal("revise_prd should be allowed to enter model loop")
	}
}

func TestWorkspaceBuildRepairInteractionIsIgnored(t *testing.T) {
	raw := []byte(`{
		"kind": "agent_app_build_failure",
		"interaction": {
			"status": "pending_build_repair",
			"allowed_actions": ["start_build_repair", "continue_development", "skip_build_repair", "view_build_diagnostics"]
		}
	}`)
	interaction := workspaceInteractionFromResultData(raw)
	if interaction != nil {
		t.Fatalf("build failures must not produce a pending interaction: %+v", interaction)
	}
}

func TestRecordWorkspaceInteractionEventCreatesDisplayOnlyMessage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("create messages table: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/demo",
		Source:        SourceWorkspace,
		SessionID:     "interaction-event-session",
		Title:         "PRD 讨论",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusPendingConfirmation,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc := &WorkspaceChatService{sessionRepo: sessionRepo, messageRepo: messageRepo}
	ctx := context.WithValue(context.Background(), contextx.RequestUserHeader, "tester")
	if err := svc.RecordWorkspaceInteractionEvent(ctx, &dto.RecordWorkspaceInteractionEventReq{
		SessionID:    "interaction-event-session",
		Action:       "confirm_prd",
		CardType:     "prd_confirmation",
		Status:       model.ChatSessionStatusPendingConfirmation,
		ArtifactKind: "agent_app_prd",
	}); err != nil {
		t.Fatalf("record interaction event: %v", err)
	}

	messages, err := messageRepo.ListBySessionID("interaction-event-session")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	msg := messages[0]
	if msg.Role != RoleUser || msg.ContextUsage != MessageContextDisplayOnly || msg.ArtifactKind != "workspace_interaction_event" {
		t.Fatalf("unexpected audit message: %#v", msg)
	}
	if !strings.Contains(msg.DisplayContent, "按此方案开始") {
		t.Fatalf("display content should mention action, got %q", msg.DisplayContent)
	}
	updated, err := sessionRepo.GetBySessionID("interaction-event-session")
	if err != nil {
		t.Fatalf("get updated session: %v", err)
	}
	if updated.Status != model.ChatSessionStatusActive {
		t.Fatalf("confirm_prd should clear pending status, got %q", updated.Status)
	}
}

func TestRecordWorkspaceInteractionEventViewDoesNotClearPending(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("create messages table: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/demo",
		Source:        SourceWorkspace,
		SessionID:     "interaction-view-session",
		Title:         "PRD 讨论",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusPendingConfirmation,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc := &WorkspaceChatService{sessionRepo: sessionRepo, messageRepo: messageRepo}
	ctx := context.WithValue(context.Background(), contextx.RequestUserHeader, "tester")
	if err := svc.RecordWorkspaceInteractionEvent(ctx, &dto.RecordWorkspaceInteractionEventReq{
		SessionID:    "interaction-view-session",
		Action:       "view_prd",
		CardType:     "prd_confirmation",
		Status:       model.ChatSessionStatusPendingConfirmation,
		ArtifactKind: "agent_app_prd",
	}); err != nil {
		t.Fatalf("record interaction event: %v", err)
	}

	updated, err := sessionRepo.GetBySessionID("interaction-view-session")
	if err != nil {
		t.Fatalf("get updated session: %v", err)
	}
	if updated.Status != model.ChatSessionStatusPendingConfirmation {
		t.Fatalf("view_prd should keep pending status, got %q", updated.Status)
	}
}

func TestBuildWorkspaceHandoffContentForQA(t *testing.T) {
	got := buildWorkspaceHandoffContent(workspaceHandoffContentInput{
		TargetRole:    WorkspaceRoleQAEngineer,
		ArtifactKind:  workspaceBuildArtifactKind,
		ArtifactJSON:  `{"kind":"agent_app_build","workspace_path":"/liubeiluo/nps","new_version":"v4"}`,
		ContextPolicy: ContextPolicyArtifactOnly,
	})
	for _, want := range []string{
		"target_role 固定为 qa_engineer",
		"change_role.execute_directory 必须固定",
		"测试阶段要求",
		"read_dir/search",
		"禁止测试整个空间",
		"创建开始时间/创建结束时间",
		`"kind":"agent_app_build"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("content should include %q, got %q", want, got)
		}
	}

	ctx := buildWorkspaceHandoffContext(workspaceHandoffContextInput{
		TargetRole:    WorkspaceRoleQAEngineer,
		ArtifactKind:  workspaceBuildArtifactKind,
		ArtifactJSON:  `{"kind":"agent_app_build","workspace_path":"/liubeiluo/nps","new_version":"v4"}`,
		ContextPolicy: ContextPolicyArtifactOnly,
	})
	decisionText := strings.Join(ctx.KeyDecisions, "；")
	constraintText := strings.Join(ctx.Constraints, "；")
	if !strings.Contains(decisionText, "已确认构建产物") || strings.Contains(decisionText, "已确认 PRD") {
		t.Fatalf("build handoff decisions should be build-specific, got %q", decisionText)
	}
	if !strings.Contains(constraintText, "测试阶段只验证") || strings.Contains(constraintText, "PRD v2") {
		t.Fatalf("build handoff constraints should be QA-specific, got %q", constraintText)
	}
}

func TestWorkspaceFirstTurnDirectoryRAGHint(t *testing.T) {
	workspaceCtx := &dto.GetWorkspaceContextResp{
		Directory: dto.WorkspaceContextDirectory{
			Name:         "投票系统",
			Code:         "vote",
			FullCodePath: "/system/x_world/vote",
			Type:         "package",
		},
	}
	got := workspaceFirstTurnDirectoryRAGHint([]*model.AgentChatMessage{
		{Role: RoleUser, Content: "帮我创建一个四大古都投票"},
	}, workspaceCtx)
	for _, want := range []string{
		"首轮目录理解要求",
		"函数描述和 Schema 摘要",
		"使用当前软件",
		"app_operator",
		"run_python",
		"复杂、专项或多步骤文件处理",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("first turn hint should contain %q, got:\n%s", want, got)
		}
	}
	if got := workspaceFirstTurnDirectoryRAGHint([]*model.AgentChatMessage{
		{Role: RoleUser, Content: "第一轮"},
		{Role: RoleUser, Content: "第二轮"},
	}, workspaceCtx); got != "" {
		t.Fatalf("second turn should not include first-turn hint, got:\n%s", got)
	}
}

func TestChangeRoleKeepsCurrentModuleWhenModelBroadensToParent(t *testing.T) {
	data := buildChangeRole(context.Background(), changeRoleArgs{
		TargetRole:       WorkspaceRoleProductManager,
		ExecuteDirectory: "/system/ticket_sys",
		TaskContext:      []string{"用户要在 /system/ticket_sys/v1 目录下新建一个简单工单系统"},
		KeyInformation:   []string{"系统位置：/system/ticket_sys/v1"},
	}, "/system/ticket_sys/v1")
	if data.ExecuteDirectory != "/system/ticket_sys/v1" {
		t.Fatalf("execute directory = %q, want /system/ticket_sys/v1", data.ExecuteDirectory)
	}
	if !containsWorkspaceRoleString(data.Handoff.KeyInformation, "执行目录已固定为当前工作台模块：/system/ticket_sys/v1") {
		t.Fatalf("handoff should explain fixed current module, got %#v", data.Handoff.KeyInformation)
	}
}

func TestBuildWorkspaceHandoffContextCreatesProjectDirectoryUnderSelectedDirectory(t *testing.T) {
	ctx := buildWorkspaceHandoffContext(workspaceHandoffContextInput{
		FullCodePath:  "/system/ticket_sys/v1",
		TargetRole:    WorkspaceRoleAppDeveloper,
		ArtifactKind:  "agent_app_prd",
		ArtifactJSON:  `{"kind":"agent_app_prd","project":{"name":"工单管理系统","code":"ticket","summary":"创建工单、处理和统计"},"tables":[{"name":"工单","code":"ticket_list","fields":[{"name":"工单标题"},{"name":"开始时间"},{"name":"结束时间"}],"search_fields":[{"name":"工单标题"},{"name":"状态"}],"handlers":["OnTableAddRow","OnTableUpdateRow","OnTableDeleteRow"]},{"name":"工单处理记录","code":"ticket_record","fields":[{"name":"处理人"},{"name":"处理意见"}],"search_fields":[{"name":"处理人"}],"handlers":[]}],"forms":[{"name":"提交工单","code":"submit_ticket","target_table":"工单","request_fields":[{"name":"标题"},{"name":"描述"}],"response_fields":[{"name":"提交结果"}]}],"rules":["工单状态流转必须记录处理人"]}`,
		ContextPolicy: ContextPolicyArtifactOnly,
	})
	if ctx.WorkspaceDirectory != "/system/ticket_sys" {
		t.Fatalf("workspace directory = %q", ctx.WorkspaceDirectory)
	}
	if ctx.TargetAppDirectory != "/system/ticket_sys/v1/ticket" {
		t.Fatalf("target app directory = %q", ctx.TargetAppDirectory)
	}
	if ctx.ExecuteDirectory != "/system/ticket_sys/v1" {
		t.Fatalf("execute directory = %q", ctx.ExecuteDirectory)
	}
	if !containsWorkspaceRoleString(ctx.ReferenceFiles, "/system/ticket_sys") ||
		!containsWorkspaceRoleString(ctx.ReferenceFiles, "/system/ticket_sys/v1/ticket") {
		t.Fatalf("reference files should include workspace root and target app directory: %#v", ctx.ReferenceFiles)
	}
	if !containsWorkspaceRoleString(ctx.ReferenceDocs, "/system/prompt/roles/app-developer") ||
		!containsWorkspaceRoleString(ctx.ReferenceDocs, "/system/prompt/sdk/agent-app-sdk-readme") ||
		!containsWorkspaceRoleString(ctx.ReferenceDocs, "/system/prompt/case_catalog") {
		t.Fatalf("app developer handoff should carry development docs: %#v", ctx.ReferenceDocs)
	}
	if ctx.ArtifactDigest == nil || ctx.ArtifactDigest.ProjectCode != "ticket" || len(ctx.ArtifactDigest.Tables) != 2 || len(ctx.ArtifactDigest.Forms) != 1 {
		t.Fatalf("handoff should carry rich PRD digest, got %#v", ctx.ArtifactDigest)
	}
	if !strings.Contains(ctx.PRDExecutionMarkdown, "# 已确认 PRD：工单管理系统") ||
		!strings.Contains(ctx.PRDExecutionMarkdown, "## Table：工单") ||
		!strings.Contains(ctx.PRDExecutionMarkdown, "工单状态流转必须记录处理人") {
		t.Fatalf("handoff should carry rendered PRD execution markdown, got:\n%s", ctx.PRDExecutionMarkdown)
	}
	if len(ctx.ExecutedHooks) != 1 ||
		ctx.ExecutedHooks[0].ID != workspaceRoleHookProductManagerToDeveloper ||
		ctx.ExecutedHooks[0].Stage != workspaceRoleHookStageBeforeHandoff ||
		ctx.ExecutedHooks[0].Status != "ok" {
		t.Fatalf("handoff should record executed PRD hook, got %#v", ctx.ExecutedHooks)
	}
	if ctx.HandoffPacket == nil {
		t.Fatal("handoff context should expose typed handoff packet")
	}
	if ctx.HandoffPacket.Version != workspaceRoleHandoffPacketVersion ||
		ctx.HandoffPacket.TargetRole != WorkspaceRoleAppDeveloper ||
		ctx.HandoffPacket.ExecuteDirectory != "/system/ticket_sys/v1" ||
		ctx.HandoffPacket.TargetAppDirectory != "/system/ticket_sys/v1/ticket" {
		t.Fatalf("typed packet metadata wrong: %#v", ctx.HandoffPacket)
	}
	if ctx.HandoffPacket.Artifact == nil ||
		ctx.HandoffPacket.Artifact.Kind != "agent_app_prd" ||
		ctx.HandoffPacket.Artifact.Source != "AGENT_APP_PRD JSON" {
		t.Fatalf("typed packet should reference PRD artifact block, got %#v", ctx.HandoffPacket.Artifact)
	}
	if ctx.HandoffPacket.ArtifactDigest == nil || ctx.HandoffPacket.ArtifactDigest.ProjectCode != "ticket" {
		t.Fatalf("typed packet should carry compact PRD digest, got %#v", ctx.HandoffPacket.ArtifactDigest)
	}
	if ctx.HandoffPacket.BuildDiagnostics != nil {
		t.Fatalf("new app PRD handoff packet should not carry build diagnostics: %#v", ctx.HandoffPacket.BuildDiagnostics)
	}
	if !containsWorkspaceRoleString(ctx.HandoffPacket.References, "AGENT_APP_PRD JSON（本消息完整产物块）") ||
		!containsWorkspaceRoleString(ctx.HandoffPacket.References, "/system/ticket_sys/v1/ticket") {
		t.Fatalf("typed packet references should include artifact block and target app directory: %#v", ctx.HandoffPacket.References)
	}
	if !strings.Contains(strings.Join(ctx.HandoffPacket.KeyInformation, "；"), "需要创建目标应用目录 /system/ticket_sys/v1/ticket") {
		t.Fatalf("typed packet should explain directory placement, got %#v", ctx.HandoffPacket.KeyInformation)
	}
	if strings.Contains(formatWorkspaceRoleHandoffPacketJSON(ctx.HandoffPacket), `"prd_execution_markdown"`) {
		t.Fatalf("typed packet should not inline rendered PRD markdown: %s", formatWorkspaceRoleHandoffPacketJSON(ctx.HandoffPacket))
	}
	if got := workspaceHandoffTargetSessionFullCodePath("/system/ticket_sys/v1", ctx.ExecuteDirectory, WorkspaceRoleAppDeveloper, "agent_app_prd"); got != "/system/ticket_sys/v1" {
		t.Fatalf("target session full code path = %q, want /system/ticket_sys/v1", got)
	}
}

func TestBuildWorkspaceHandoffContextUsesCurrentDirectoryWhenProjectCodeMatches(t *testing.T) {
	ctx := buildWorkspaceHandoffContext(workspaceHandoffContextInput{
		FullCodePath:  "/system/ticket_sys/v1/ticket",
		TargetRole:    WorkspaceRoleAppDeveloper,
		ArtifactKind:  "agent_app_prd",
		ArtifactJSON:  `{"kind":"agent_app_prd","project":{"name":"工单管理系统","code":"ticket","summary":"创建工单、处理和统计"},"tables":[{"name":"工单","code":"ticket_list","fields":[{"name":"工单标题"}],"search_fields":[{"name":"工单标题"}],"handlers":["OnTableAddRow"]}]}`,
		ContextPolicy: ContextPolicyArtifactOnly,
	})
	if ctx.TargetAppDirectory != "/system/ticket_sys/v1/ticket" {
		t.Fatalf("target app directory = %q", ctx.TargetAppDirectory)
	}
	if ctx.ExecuteDirectory != "/system/ticket_sys/v1/ticket" {
		t.Fatalf("execute directory = %q", ctx.ExecuteDirectory)
	}
	if ctx.HandoffPacket == nil || ctx.HandoffPacket.ExecuteDirectory != "/system/ticket_sys/v1/ticket" {
		t.Fatalf("typed packet should keep current directory, got %#v", ctx.HandoffPacket)
	}
	if !strings.Contains(strings.Join(ctx.HandoffPacket.KeyInformation, "；"), "无需创建子目录") {
		t.Fatalf("typed packet should explain no child directory is needed, got %#v", ctx.HandoffPacket.KeyInformation)
	}
}

func TestBuildWorkspaceHandoffContextForWorkspaceRootCreatesProjectDirectory(t *testing.T) {
	ctx := buildWorkspaceHandoffContext(workspaceHandoffContextInput{
		FullCodePath:  "/system/x_world",
		TargetRole:    WorkspaceRoleAppDeveloper,
		ArtifactKind:  "agent_app_prd",
		ArtifactJSON:  `{"kind":"agent_app_prd","project":{"name":"投票系统","code":"vote","summary":"创建投票主题、选项和记录"},"tables":[{"name":"投票主题","code":"vote_topic","fields":[{"name":"主题标题"}],"search_fields":[{"name":"主题标题"}],"handlers":["OnTableAddRow"]}]}`,
		ContextPolicy: ContextPolicyArtifactOnly,
	})
	if ctx.WorkspaceDirectory != "/system/x_world" {
		t.Fatalf("workspace directory = %q", ctx.WorkspaceDirectory)
	}
	if ctx.TargetAppDirectory != "/system/x_world/vote" {
		t.Fatalf("target app directory = %q", ctx.TargetAppDirectory)
	}
	if ctx.ExecuteDirectory != "/system/x_world" {
		t.Fatalf("execute directory = %q", ctx.ExecuteDirectory)
	}
	if ctx.HandoffPacket == nil ||
		ctx.HandoffPacket.ExecuteDirectory != "/system/x_world" ||
		ctx.HandoffPacket.TargetAppDirectory != "/system/x_world/vote" {
		t.Fatalf("typed packet should create project directory from workspace root, got %#v", ctx.HandoffPacket)
	}
}

func TestBuildWorkspaceHandoffContextNarrowsQAExecuteDirectoryFromSourceMessages(t *testing.T) {
	resultData := `{"handoff":{"execute_directory":"/system/x_world","key_information":["改动文件：/system/x_world/ticket_management/ticket.go"]},"changed_files":["/system/x_world/ticket_management/ticket.go"],"artifact_refs":["/system/x_world/ticket_management/ticket_list.table"]}`
	ctx := buildWorkspaceHandoffContext(workspaceHandoffContextInput{
		FullCodePath:  "/system/x_world",
		TargetRole:    WorkspaceRoleQAEngineer,
		ArtifactKind:  workspaceBuildArtifactKind,
		ArtifactJSON:  `{"kind":"agent_app_build","workspace_path":"/system/x_world","new_version":"v4"}`,
		ContextPolicy: ContextPolicyArtifactOnly,
		Messages: []*model.AgentChatMessage{
			{
				Role:         RoleUser,
				ContextUsage: MessageContextArtifact,
				ArtifactKind: "agent_app_prd",
				Content: "AGENT_APP_PRD JSON:\n```json\n" +
					`{"kind":"agent_app_prd","project":{"name":"工单管理系统","code":"ticket_management"},"tables":[{"name":"工单","code":"ticket","fields":[{"name":"工单标题"},{"name":"工单状态"}],"search_fields":[{"name":"工单标题"},{"name":"创建开始时间"},{"name":"创建结束时间"}],"handlers":["OnTableAddRow","OnTableUpdateRow","OnTableDeleteRow"]}]}` +
					"\n```",
				User: "tester",
			},
			{
				Role:       RoleTool,
				ToolStatus: ToolCallStatusOK,
				ResultData: &resultData,
				User:       "tester",
			},
		},
	})
	if ctx.WorkspaceDirectory != "/system/x_world" {
		t.Fatalf("workspace directory = %q", ctx.WorkspaceDirectory)
	}
	if ctx.TargetAppDirectory != "/system/x_world/ticket_management" {
		t.Fatalf("target app directory = %q", ctx.TargetAppDirectory)
	}
	if ctx.ExecuteDirectory != "/system/x_world/ticket_management" {
		t.Fatalf("execute directory = %q", ctx.ExecuteDirectory)
	}
	if !containsWorkspaceRoleString(ctx.ReferenceFiles, "/system/x_world/ticket_management") {
		t.Fatalf("reference files should include target app directory: %#v", ctx.ReferenceFiles)
	}
	if ctx.ArtifactDigest == nil || ctx.ArtifactDigest.ProjectCode != "ticket_management" || len(ctx.ArtifactDigest.Tables) != 1 {
		t.Fatalf("QA context should carry previous PRD digest, got %#v", ctx.ArtifactDigest)
	}
	if !containsWorkspaceRoleString(ctx.ArtifactDigest.Tables[0].SearchFields, "创建开始时间") {
		t.Fatalf("QA context should preserve PRD search fields, got %#v", ctx.ArtifactDigest.Tables[0])
	}
	if ctx.HandoffPacket == nil ||
		ctx.HandoffPacket.TargetRole != WorkspaceRoleQAEngineer ||
		ctx.HandoffPacket.ExecuteDirectory != "/system/x_world/ticket_management" ||
		ctx.HandoffPacket.TargetAppDirectory != "/system/x_world/ticket_management" {
		t.Fatalf("QA typed packet should narrow to target app directory, got %#v", ctx.HandoffPacket)
	}
	if ctx.HandoffPacket.BuildDiagnostics != nil {
		t.Fatalf("QA build-success packet should not carry build diagnostics, got %#v", ctx.HandoffPacket.BuildDiagnostics)
	}
	if !strings.Contains(strings.Join(ctx.HandoffPacket.KeyInformation, "；"), "测试重点") ||
		!strings.Contains(strings.Join(ctx.HandoffPacket.KeyInformation, "；"), "创建开始时间") {
		t.Fatalf("QA typed packet should carry verification focus, got %#v", ctx.HandoffPacket.KeyInformation)
	}
}

func TestBuildWorkspaceHandoffContextForBuildFailureIncludesDiagnosticsInPacket(t *testing.T) {
	ctx := buildWorkspaceHandoffContext(workspaceHandoffContextInput{
		FullCodePath:  "/system/x_world/inventory",
		TargetRole:    WorkspaceRoleBuildEngineer,
		ArtifactKind:  workspaceBuildFailureKind,
		ArtifactJSON:  `{"kind":"agent_app_build_failure","workspace_path":"/system/x_world/inventory","error":"app startup failed: SDK schema compile failed: router /inventory/purchase_inbound_list.table table schema decode failed\nfield SupplierName (supplier_name): widget \"select\" requires options or OnSelectFuzzyMap entry\nfield CreatedBy (created_by): audit field \"created_by\" hide tag must be \"create,update\", got \"\""}`,
		ContextPolicy: ContextPolicyArtifactOnly,
	})
	if ctx.HandoffPacket == nil {
		t.Fatal("build failure context should expose typed handoff packet")
	}
	packet := ctx.HandoffPacket
	if packet.TargetRole != WorkspaceRoleBuildEngineer ||
		packet.ExecuteDirectory != "/system/x_world/inventory" ||
		packet.BuildDiagnostics == nil {
		t.Fatalf("build failure packet should carry build diagnostics in target directory, got %#v", packet)
	}
	for _, want := range []string{"schema_validation", "select_options", "audit_field"} {
		if !containsWorkspaceRoleString(packet.BuildDiagnostics.Categories, want) {
			t.Fatalf("packet diagnostics should include category %q, got %#v", want, packet.BuildDiagnostics.Categories)
		}
	}
	if !containsWorkspaceRoleString(packet.BuildDiagnostics.Routers, "/inventory/purchase_inbound_list.table") {
		t.Fatalf("packet diagnostics should include router, got %#v", packet.BuildDiagnostics.Routers)
	}
	if !containsWorkspaceRoleString(packet.References, "/system/prompt/sdk/reference/build-validation") {
		t.Fatalf("packet references should include diagnostic required docs, got %#v", packet.References)
	}
	keyInfo := strings.Join(packet.KeyInformation, "；")
	for _, want := range []string{"构建错误类别", "构建错误 router", "构建修复策略"} {
		if !strings.Contains(keyInfo, want) {
			t.Fatalf("build failure packet key info should contain %q, got %#v", want, packet.KeyInformation)
		}
	}
}

func TestExecuteToolCallsPersistsRoleAfterChangeRole(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/liubeiluo/demo",
		Source:        SourceWorkspace,
		SessionID:     "role-session",
		Title:         "阶段交接",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusGenerating,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc := &WorkspaceChatService{
		toolReg:     NewToolRegistry(),
		sessionRepo: sessionRepo,
		messageRepo: messageRepo,
	}
	call := llms.ToolCall{ID: "call-change-role", Type: "function"}
	call.Function.Name = "change_role"
	call.Function.Arguments = `{"target_role":"product_manager","execute_directory":"/liubeiluo/demo","user_input":"帮我做个系统"}`

	summaries, nextFullCodePath, err := svc.executeToolCalls(context.Background(), []llms.ToolCall{call}, "role-session", "/liubeiluo/demo", "tester", "", 0, func(string, interface{}) {})
	if err != nil {
		t.Fatalf("execute tool calls: %v", err)
	}
	if nextFullCodePath != "/liubeiluo/demo" {
		t.Fatalf("next full code path = %q, want /liubeiluo/demo", nextFullCodePath)
	}
	if len(summaries) != 1 || summaries[0].Status != ToolCallStatusOK {
		t.Fatalf("unexpected summaries: %#v", summaries)
	}
	updated, err := sessionRepo.GetBySessionID("role-session")
	if err != nil {
		t.Fatalf("get updated session: %v", err)
	}
	if updated.RoleID != WorkspaceRoleProductManager || updated.RoleDisplayName != "产品经理" {
		t.Fatalf("session role not persisted: %#v", updated)
	}
}

func TestExecuteToolCallsCompactsDuplicateChangeRoleInSameBatch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:        7,
		FullCodePath:  "/system/democase",
		Source:        SourceWorkspace,
		SessionID:     "duplicate-role-session",
		Title:         "重复切角色",
		ModeCode:      "dev",
		Status:        model.ChatSessionStatusGenerating,
		ContextPolicy: ContextPolicyFull,
		User:          "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc := &WorkspaceChatService{
		toolReg:     NewToolRegistry(),
		sessionRepo: sessionRepo,
		messageRepo: messageRepo,
	}
	args := `{"target_role":"reviewer","execute_directory":"/system/democase","task_context":["用户询问是否能看懂当前项目。"],"key_information":["当前目录为 /system/democase。"],"references":[],"reset_context":false}`
	first := llms.ToolCall{ID: "call-change-role-1", Type: "function"}
	first.Function.Name = "change_role"
	first.Function.Arguments = args
	second := llms.ToolCall{ID: "call-change-role-2", Type: "function"}
	second.Function.Name = "change_role"
	second.Function.Arguments = args

	summaries, _, err := svc.executeToolCalls(context.Background(), []llms.ToolCall{first, second}, "duplicate-role-session", "/system/democase", "tester", "", 0, func(string, interface{}) {})
	if err != nil {
		t.Fatalf("execute tool calls: %v", err)
	}
	if len(summaries) != 2 || summaries[0].Status != ToolCallStatusOK || summaries[1].Status != ToolCallStatusOK {
		t.Fatalf("unexpected summaries: %#v", summaries)
	}
	if !strings.Contains(summaries[1].Result, "重复 change_role 调用已跳过") {
		t.Fatalf("second change_role should be compact duplicate result: %#v", summaries[1])
	}
	messages, err := messageRepo.ListBySessionID("duplicate-role-session")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	var firstTool, secondTool *model.AgentChatMessage
	for _, msg := range messages {
		switch msg.ToolCallID {
		case "call-change-role-1":
			firstTool = msg
		case "call-change-role-2":
			secondTool = msg
		}
	}
	if firstTool == nil || secondTool == nil {
		t.Fatalf("expected both tool messages to be saved: %#v", messages)
	}
	if firstTool.ResultData == nil || !strings.Contains(*firstTool.ResultData, "loaded_docs") {
		t.Fatalf("first change_role should keep full structured data: %#v", firstTool.ResultData)
	}
	if secondTool.ResultData == nil || strings.Contains(*secondTool.ResultData, "loaded_docs") {
		t.Fatalf("duplicate change_role should not persist loaded_docs again: %#v", secondTool.ResultData)
	}
}

func TestChangeRolePreservesModelContextOnRoleSwitch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AgentChatSession{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := createSQLiteAgentChatMessagesTable(db); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	sessionRepo := repository.NewChatSessionRepository(db)
	messageRepo := repository.NewChatMessageRepository(db)
	session := &model.AgentChatSession{
		TreeID:          7,
		FullCodePath:    "/liubeiluo/demo",
		Source:          SourceWorkspace,
		SessionID:       "anchor-session",
		Title:           "开发转测试",
		ModeCode:        "dev",
		Status:          model.ChatSessionStatusGenerating,
		RoleID:          WorkspaceRoleAppDeveloper,
		RoleDisplayName: "应用开发工程师",
		ContextPolicy:   ContextPolicyFull,
		User:            "tester",
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	oldMsg := &model.AgentChatMessage{SessionID: "anchor-session", Role: RoleUser, Content: "旧开发讨论，仍应保留给测试模型", User: "tester"}
	if err := messageRepo.Create(oldMsg); err != nil {
		t.Fatalf("create old message: %v", err)
	}
	currentMsg := &model.AgentChatMessage{SessionID: "anchor-session", Role: RoleUser, Content: "build 已通过，进入自动测试", User: "tester"}
	if err := messageRepo.Create(currentMsg); err != nil {
		t.Fatalf("create current message: %v", err)
	}
	svc := &WorkspaceChatService{
		toolReg:     NewToolRegistry(),
		sessionRepo: sessionRepo,
		messageRepo: messageRepo,
	}
	call := llms.ToolCall{ID: "call-change-role-anchor", Type: "function"}
	call.Function.Name = "change_role"
	call.Function.Arguments = `{"target_role":"qa_engineer","execute_directory":"/liubeiluo/demo","task_context":["build 已通过","验证 Form 写入和图表统计"],"key_information":["重点检查记录表和图表"],"references":["/system/prompt/roles/qa-engineer","nps_submit.go"]}`

	summaries, nextFullCodePath, err := svc.executeToolCalls(context.Background(), []llms.ToolCall{call}, "anchor-session", "/liubeiluo/demo", "tester", "", 0, func(string, interface{}) {})
	if err != nil {
		t.Fatalf("execute tool calls: %v", err)
	}
	if nextFullCodePath != "/liubeiluo/demo" {
		t.Fatalf("next full code path = %q, want /liubeiluo/demo", nextFullCodePath)
	}
	if len(summaries) != 1 || summaries[0].Status != ToolCallStatusOK {
		t.Fatalf("unexpected summaries: %#v", summaries)
	}
	updated, err := sessionRepo.GetBySessionID("anchor-session")
	if err != nil {
		t.Fatalf("get updated session: %v", err)
	}
	if updated.RoleID != WorkspaceRoleQAEngineer {
		t.Fatalf("role not updated: %#v", updated)
	}
	if updated.FullCodePath != "/liubeiluo/demo" {
		t.Fatalf("full code path not updated: %#v", updated)
	}
	if updated.ModelContextAnchorMessageID != 0 {
		t.Fatalf("anchor id = %d, want 0 so history is preserved", updated.ModelContextAnchorMessageID)
	}
	if updated.ContextPolicy != ContextPolicyFull {
		t.Fatalf("context policy = %q, want %q", updated.ContextPolicy, ContextPolicyFull)
	}
	messages, err := messageRepo.ListBySessionID("anchor-session")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	var joined strings.Builder
	for _, msg := range messages {
		joined.WriteString(msg.Content)
		joined.WriteString("\n")
	}
	if !strings.Contains(joined.String(), "旧开发讨论") || !strings.Contains(joined.String(), "build 已通过") {
		t.Fatalf("role switch should preserve old and current messages: %#v", messages)
	}
	if len(messages) < 2 || messages[0].ID != oldMsg.ID || messages[1].ID != currentMsg.ID {
		t.Fatalf("messages should retain original order: %#v", messages)
	}
}

func createSQLiteAgentChatMessagesTable(db *gorm.DB) error {
	return db.Exec(`
CREATE TABLE agent_chat_messages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at datetime,
	updated_at datetime,
	deleted_at datetime,
	created_by text,
	updated_by text,
	deleted_by text,
	session_id text NOT NULL,
	role text NOT NULL,
	content text,
	display_content text,
	thinking_content text,
	files text,
	tool_calls text,
	tool_call_id text,
	tool_status text,
	result_data text,
	result_metadata text,
	llm_config_id integer,
	llm_config_name text,
	llm_provider text,
	llm_model text,
	llm_usage text,
	model_context_plan text,
	context_usage text DEFAULT 'include',
	artifact_kind text,
	user text NOT NULL
)`).Error
}
