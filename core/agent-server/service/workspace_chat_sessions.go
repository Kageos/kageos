package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kageos/kageos/core/agent-server/model"
	"github.com/kageos/kageos/core/agent-server/repository"
	"github.com/kageos/kageos/core/agent-server/streamloop"
	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/apicall"
	"github.com/kageos/kageos/pkg/contextx"
	"github.com/kageos/kageos/pkg/logger"
)

// CancelSession 手动取消正在执行的会话
func (s *WorkspaceChatService) CancelSession(ctx context.Context, sessionID string) error {
	session, err := s.sessionRepo.GetBySessionID(sessionID)
	if err != nil {
		return fmt.Errorf("会话不存在: %w", err)
	}
	if err := ensureWorkspaceSessionOwner(ctx, session); err != nil {
		return err
	}
	cancelFn, hasRunningCancel := s.runningCancels.Load(sessionID)
	if session.Status != model.ChatSessionStatusGenerating && !hasRunningCancel {
		if session.Status == model.ChatSessionStatusCancelled {
			return nil
		}
		return fmt.Errorf("会话未在执行中（当前状态: %s）", session.Status)
	}

	// 标记为已取消
	session.Status = model.ChatSessionStatusCancelled
	user := contextx.GetRequestUser(ctx)
	session.UpdatedBy = user
	if err := s.sessionRepo.Update(session); err != nil {
		return fmt.Errorf("更新会话状态失败: %w", err)
	}

	// 触发 cancelFunc，让 streamloop 尽快退出
	if hasRunningCancel {
		s.runningCancels.Delete(sessionID)
		cancelFn.(context.CancelFunc)()
		logger.Infof(ctx, "[WorkspaceChatStream] 会话已取消 - SessionID: %s", sessionID)
	}
	s.deleteWorkspaceRuntimeState(ctx, sessionID)
	return nil
}

// IsSSEConnected 检查该 session 是否仍有活跃的 SSE 连接（供前端存活检测，SSE 存活则不轮询大消息列表）
func (s *WorkspaceChatService) IsSSEConnected(sessionID string) bool {
	_, ok := s.sseConnections.Load(sessionID)
	return ok
}

// ListRunningSessions 查询当前用户所有正在执行的工作台会话
func (s *WorkspaceChatService) ListRunningSessions(ctx context.Context) ([]*dto.WorkspaceSessionItem, error) {
	user := contextx.GetRequestUser(ctx)
	sessions, err := s.sessionRepo.ListRunningByUser(user)
	if err != nil {
		return nil, fmt.Errorf("查询执行中会话失败: %w", err)
	}
	return s.buildWorkspaceSessionItems(ctx, sessions), nil
}

// ListFinishedSessions 查询当前用户最近已结束的工作台会话
func (s *WorkspaceChatService) ListFinishedSessions(ctx context.Context, limit int) ([]*dto.WorkspaceSessionItem, error) {
	if limit <= 0 {
		limit = 20
	}
	user := contextx.GetRequestUser(ctx)
	sessions, err := s.sessionRepo.ListFinishedByUser(user, limit)
	if err != nil {
		return nil, fmt.Errorf("查询已结束会话失败: %w", err)
	}
	return s.buildWorkspaceSessionItems(ctx, sessions), nil
}

func (s *WorkspaceChatService) buildWorkspaceSessionItems(ctx context.Context, sessions []*model.AgentChatSession) []*dto.WorkspaceSessionItem {
	directoryNames := s.resolveWorkspaceSessionDirectoryNames(ctx, sessions)
	items := make([]*dto.WorkspaceSessionItem, 0, len(sessions))
	for _, session := range sessions {
		fullCodePath := normalizeWorkspacePath(session.FullCodePath)
		resourceFullCodePath := normalizeWorkspacePath(session.ResourceFullCodePath)
		item := &dto.WorkspaceSessionItem{
			SessionID:                   session.SessionID,
			Title:                       session.Title,
			Source:                      session.Source,
			AutomationTaskID:            session.AutomationTaskID,
			AutomationTaskCode:          session.AutomationTaskCode,
			AutomationTaskTitle:         session.AutomationTaskTitle,
			User:                        session.User,
			ModeCode:                    normalizeWorkspaceModeCode(session.ModeCode),
			Status:                      session.Status,
			RoleID:                      workspaceSessionRoleID(session),
			RoleDisplayName:             workspaceSessionRoleDisplayName(session),
			FullCodePath:                fullCodePath,
			DirectoryName:               directoryNames[fullCodePath],
			ResourceTreeID:              session.ResourceTreeID,
			ResourceFullCodePath:        resourceFullCodePath,
			ResourceName:                directoryNames[resourceFullCodePath],
			ParentSessionID:             session.ParentSessionID,
			HandoffKind:                 session.HandoffKind,
			HandoffTargetRole:           session.HandoffTargetRole,
			ContextPolicy:               session.ContextPolicy,
			ModelContextAnchorMessageID: session.ModelContextAnchorMessageID,
			ArchivedForModel:            session.ArchivedForModel,
			ArchiveReason:               session.ArchiveReason,
			CreatedAt:                   session.CreatedAt,
			UpdatedAt:                   session.UpdatedAt,
		}
		// PRD 确认已降级为预览内的快捷动作，不再向前端暴露会话级待处理卡点。
		item.PendingInteraction = nil
		items = append(items, item)
	}
	return items
}

func (s *WorkspaceChatService) resolveWorkspaceSessionDirectoryNames(ctx context.Context, sessions []*model.AgentChatSession) map[string]string {
	if s == nil || s.sessionRepo == nil || len(sessions) == 0 {
		return map[string]string{}
	}

	paths := make([]string, 0, len(sessions)*2)
	for _, session := range sessions {
		for _, rawPath := range []string{session.FullCodePath, session.ResourceFullCodePath} {
			path := normalizeWorkspacePath(rawPath)
			if path != "" {
				paths = append(paths, path)
			}
		}
	}

	directoryNames, err := s.sessionRepo.GetServiceTreeNamesByFullCodePaths(paths)
	if err != nil {
		logger.Warnf(ctx, "[WorkspaceChat] 查询会话目录名称失败: %v", err)
		return map[string]string{}
	}
	return directoryNames
}

func (s *WorkspaceChatService) ListSessionsFiltered(ctx context.Context, fullCodePath, sessionScope string, automationTaskID int64, page, pageSize int) ([]*dto.WorkspaceSessionItem, int64, []*dto.WorkspaceAutomationAgentItem, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	sessionScope = strings.TrimSpace(sessionScope)
	if sessionScope == "" {
		sessionScope = "human"
	}
	if sessionScope != "human" && sessionScope != "automation" && sessionScope != "all" {
		return nil, 0, nil, fmt.Errorf("无效的 session_scope: %s", sessionScope)
	}
	if sessionScope == "automation" && automationTaskID <= 0 {
		return nil, 0, nil, fmt.Errorf("查看自动化 Agent 会话时 automation_task_id 必填")
	}

	requestedPath := normalizeWorkspacePath(fullCodePath)
	if requestedPath == "" {
		return nil, 0, nil, fmt.Errorf("full_code_path 必填")
	}
	workspaceCtx, err := apicall.GetWorkspaceContext(ctx, requestedPath, "")
	if err != nil {
		return nil, 0, nil, fmt.Errorf("解析会话目录失败: %w", err)
	}
	if workspaceCtx == nil {
		return nil, 0, nil, fmt.Errorf("解析会话目录失败: 上游未返回工作台上下文")
	}
	directoryPath := normalizeWorkspacePath(workspaceCtx.Directory.FullCodePath)
	if directoryPath == "" {
		return nil, 0, nil, fmt.Errorf("解析会话目录失败: 上游未返回目录路径")
	}
	resourcePath := ""
	if requestedPath != directoryPath {
		resourcePath = requestedPath
	}
	user := contextx.GetRequestUser(ctx)
	sessions, total, err := s.sessionRepo.ListWorkspaceSessions(ctx, repository.WorkspaceSessionListOptions{
		FullCodePath:         directoryPath,
		ResourceFullCodePath: resourcePath,
		User:                 user,
		SessionScope:         sessionScope,
		AutomationTaskID:     automationTaskID,
		Offset:               (page - 1) * pageSize,
		Limit:                pageSize,
	})
	if err != nil {
		return nil, 0, nil, fmt.Errorf("获取会话列表失败: %w", err)
	}
	agents, err := s.sessionRepo.ListWorkspaceAutomationAgents(ctx, directoryPath, resourcePath, user)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("获取自动化 Agent 列表失败: %w", err)
	}
	agentItems := make([]*dto.WorkspaceAutomationAgentItem, 0, len(agents))
	for _, agent := range agents {
		title := strings.TrimSpace(agent.TaskTitle)
		if title == "" {
			title = fmt.Sprintf("自动化 Agent #%d", agent.TaskID)
		}
		agentItems = append(agentItems, &dto.WorkspaceAutomationAgentItem{TaskID: agent.TaskID, TaskCode: agent.TaskCode, TaskTitle: title})
	}
	return s.buildWorkspaceSessionItems(ctx, sessions), total, agentItems, nil
}

func (s *WorkspaceChatService) persistWorkspaceSessionInteractionStatus(ctx context.Context, sessionID string, summaries []streamloop.ToolCallSummary, user string) {
	nextStatus := workspaceSessionStatusFromToolSummaries(summaries)
	if nextStatus == "" || s == nil || s.sessionRepo == nil {
		return
	}

	session, err := s.sessionRepo.GetBySessionID(sessionID)
	if err != nil || session == nil {
		logger.Warnf(ctx, "[WorkspaceChat] 读取待交互会话失败 session_id=%s err=%v", sessionID, err)
		return
	}
	if session.Status == model.ChatSessionStatusCancelled || session.Status == model.ChatSessionStatusDone {
		return
	}
	session.Status = nextStatus
	session.UpdatedBy = user
	if err := s.sessionRepo.Update(session); err != nil {
		logger.Warnf(ctx, "[WorkspaceChat] 持久化待交互状态失败 session_id=%s status=%s err=%v", sessionID, nextStatus, err)
	}
}

func workspaceSessionStatusFromToolSummaries(summaries []streamloop.ToolCallSummary) string {
	if workspaceToolSummariesHaveGeneratedOutput(summaries) {
		return model.ChatSessionStatusOutput
	}
	return ""
}

func workspaceToolSummariesHaveGeneratedOutput(summaries []streamloop.ToolCallSummary) bool {
	for i := len(summaries) - 1; i >= 0; i-- {
		summary := summaries[i]
		if summary.Status != ToolCallStatusOK {
			continue
		}
		if workspaceToolCallHasGeneratedOutput(summary) {
			return true
		}
	}
	return false
}

func workspaceToolCallHasGeneratedOutput(summary streamloop.ToolCallSummary) bool {
	switch strings.TrimSpace(summary.Name) {
	case "write_prd",
		"build_workspace",
		"write_file",
		"edit_file",
		"create_directory":
		return true
	}
	if summary.Metadata != nil && len(summary.Metadata.DisplayFileFields) > 0 {
		return true
	}
	return workspaceResultDataLooksLikeArtifact(summary.ResultData)
}

func workspaceResultDataLooksLikeArtifact(resultData interface{}) bool {
	if resultData == nil {
		return false
	}
	raw, err := json.Marshal(resultData)
	if err != nil {
		return false
	}
	var payload struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	kind := strings.TrimSpace(payload.Kind)
	return strings.HasPrefix(kind, "agent_app_") || strings.HasPrefix(kind, "workspace_")
}

func normalizeWorkspacePendingInteractionStatus(status string) string {
	switch strings.TrimSpace(status) {
	case model.ChatSessionStatusPendingConfirmation:
		return model.ChatSessionStatusPendingConfirmation
	default:
		return ""
	}
}

func (s *WorkspaceChatService) ResolveWorkspacePendingInteraction(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session_id 必填")
	}
	session, err := s.sessionRepo.GetBySessionID(sessionID)
	if err != nil || session == nil {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}
	user := contextx.GetRequestUser(ctx)
	if user != "" && session.User != "" && session.User != user {
		return fmt.Errorf("不能操作其他用户的会话")
	}
	switch session.Status {
	case model.ChatSessionStatusPendingConfirmation, model.ChatSessionStatusPendingTest, model.ChatSessionStatusPendingBuildRepair:
		session.Status = model.ChatSessionStatusActive
		session.UpdatedBy = user
		if err := s.sessionRepo.Update(session); err != nil {
			return fmt.Errorf("更新会话状态失败: %w", err)
		}
	}
	return nil
}

// ListSessions 根据 full_code_path 获取会话列表
func (s *WorkspaceChatService) ListSessions(ctx context.Context, fullCodePath string, page, pageSize int) ([]*dto.WorkspaceSessionItem, int64, error) {
	items, total, _, err := s.ListSessionsFiltered(ctx, fullCodePath, "human", 0, page, pageSize)
	return items, total, err
}

// ListMessages 根据 sessionID 获取消息列表
func (s *WorkspaceChatService) ListMessages(ctx context.Context, sessionID string) ([]*model.AgentChatMessage, error) {
	session, err := s.sessionRepo.GetBySessionID(sessionID)
	if err != nil || session == nil {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}
	if err := ensureWorkspaceSessionOwner(ctx, session); err != nil {
		return nil, err
	}
	return s.messageRepo.ListBySessionID(sessionID)
}

func ensureWorkspaceSessionOwner(ctx context.Context, session *model.AgentChatSession) error {
	if session == nil {
		return fmt.Errorf("会话不存在")
	}
	user := contextx.GetRequestUser(ctx)
	if user != "" && session.User != "" && session.User != user {
		return fmt.Errorf("不能操作其他用户的会话")
	}
	return nil
}

func (s *WorkspaceChatService) ensureWorkspaceSessionHasRunnableMessage(sessionID string) error {
	if s == nil || s.messageRepo == nil {
		return fmt.Errorf("消息仓储未初始化，无法续跑会话")
	}
	messages, err := s.messageRepo.ListBySessionID(sessionID)
	if err != nil {
		return fmt.Errorf("读取会话消息失败: %w", err)
	}
	for _, msg := range messages {
		if msg == nil || msg.Role != RoleUser {
			continue
		}
		return nil
	}
	return fmt.Errorf("会话没有可进入模型上下文的用户消息，无法续跑")
}
