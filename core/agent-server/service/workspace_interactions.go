package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kageos/kageos/core/agent-server/model"
	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/contextx"
)

func workspaceSessionHasPendingInteractionStatus(status string) bool {
	return normalizeWorkspacePendingInteractionStatus(status) == model.ChatSessionStatusPendingConfirmation
}

func (s *WorkspaceChatService) pendingInteractionForSession(session *model.AgentChatSession) *dto.WorkspaceInteraction {
	if s == nil || s.messageRepo == nil || session == nil || !workspaceSessionHasPendingInteractionStatus(session.Status) {
		return nil
	}
	messages, err := s.messageRepo.ListBySessionID(session.SessionID)
	if err != nil {
		return nil
	}
	return workspacePendingInteractionFromMessages(session.Status, messages)
}

func workspacePendingInteractionFromMessages(sessionStatus string, messages []*model.AgentChatMessage) *dto.WorkspaceInteraction {
	expectedStatus := normalizeWorkspacePendingInteractionStatus(sessionStatus)
	if expectedStatus == "" {
		return nil
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg == nil || msg.ResultData == nil || strings.TrimSpace(*msg.ResultData) == "" {
			continue
		}
		interaction := workspaceInteractionFromResultData([]byte(*msg.ResultData))
		if interaction == nil || normalizeWorkspacePendingInteractionStatus(interaction.Status) != expectedStatus {
			continue
		}
		return interaction
	}
	return nil
}

func workspaceInteractionFromResultData(raw []byte) *dto.WorkspaceInteraction {
	var artifact map[string]interface{}
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return nil
	}
	interactionRaw, ok := artifact["interaction"].(map[string]interface{})
	if !ok || interactionRaw == nil {
		return nil
	}
	status := normalizeWorkspacePendingInteractionStatus(workspaceStringFromMap(interactionRaw, "status"))
	if status != model.ChatSessionStatusPendingConfirmation {
		return nil
	}
	artifactKind := firstNonEmptyString(
		workspaceStringFromMap(interactionRaw, "artifact_kind"),
		workspaceStringFromMap(artifact, "kind"),
	)
	cardType := firstNonEmptyString(
		workspaceStringFromMap(interactionRaw, "card_type"),
		workspaceInteractionCardType(artifactKind, status),
	)
	// PRD confirmation is the only interaction allowed to pause a workspace
	// session. Build failures and every other process state are model-owned and
	// must continue without asking the user to handle a card.
	if artifactKind != "agent_app_prd" || cardType != "prd_confirmation" {
		return nil
	}
	blocking, hasBlocking := interactionRaw["blocking"].(bool)
	if !hasBlocking {
		blocking = status != model.ChatSessionStatusPendingBuildRepair
	}
	out := &dto.WorkspaceInteraction{
		ID:                  workspaceInteractionID(status, raw),
		CardType:            cardType,
		ArtifactKind:        artifactKind,
		Status:              status,
		Blocking:            blocking,
		Title:               firstNonEmptyString(workspaceStringFromMap(interactionRaw, "title"), workspaceInteractionTitle(cardType, status)),
		Description:         workspaceStringFromMap(interactionRaw, "description"),
		HelpText:            workspaceStringFromMap(interactionRaw, "help_text"),
		ViewText:            firstNonEmptyString(workspaceStringFromMap(interactionRaw, "view_text"), workspaceInteractionViewText(cardType)),
		ConfirmText:         workspaceStringFromMap(interactionRaw, "confirm_text"),
		ReviseText:          workspaceStringFromMap(interactionRaw, "revise_text"),
		CancelText:          workspaceStringFromMap(interactionRaw, "cancel_text"),
		TargetRoleOnConfirm: workspaceStringFromMap(interactionRaw, "target_role_on_confirm"),
		AllowedActions:      workspaceStringListFromMap(interactionRaw, "allowed_actions"),
		Artifact:            artifact,
	}
	return out
}

func workspaceInteractionID(status string, raw []byte) string {
	sum := sha1.Sum(raw)
	return fmt.Sprintf("%s:%s", status, hex.EncodeToString(sum[:])[:12])
}

func workspaceInteractionCardType(artifactKind, status string) string {
	switch {
	case artifactKind == workspaceBuildFailureKind || status == model.ChatSessionStatusPendingBuildRepair:
		return "build_repair"
	case artifactKind == "agent_app_prd" || status == model.ChatSessionStatusPendingConfirmation:
		return "prd_confirmation"
	default:
		return "stage_confirmation"
	}
}

func workspaceInteractionTitle(cardType, status string) string {
	switch cardType {
	case "build_repair":
		return "构建等待修复"
	case "prd_confirmation":
		return "PRD 等待确认"
	default:
		if status == model.ChatSessionStatusPendingConfirmation {
			return "等待确认"
		}
		return "等待处理"
	}
}

func workspaceInteractionViewText(cardType string) string {
	switch cardType {
	case "build_repair":
		return "查看诊断"
	case "prd_confirmation":
		return "查看 PRD"
	default:
		return "查看详情"
	}
}

func workspaceStringFromMap(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if value, ok := m[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func workspaceStringListFromMap(m map[string]interface{}, key string) []string {
	if m == nil {
		return nil
	}
	raw, ok := m[key].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text := strings.TrimSpace(fmt.Sprint(item))
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func workspaceInteractionAllowsAction(interaction *dto.WorkspaceInteraction, action string) bool {
	action = strings.TrimSpace(action)
	if interaction == nil || action == "" {
		return false
	}
	for _, allowed := range interaction.AllowedActions {
		if strings.TrimSpace(allowed) == action {
			return true
		}
	}
	return false
}

func workspaceInteractionActionCanRunModel(action string) bool {
	switch strings.TrimSpace(action) {
	case "revise_prd", "continue_development":
		return true
	default:
		return false
	}
}

func workspaceFallbackPendingInteraction(status string) *dto.WorkspaceInteraction {
	status = normalizeWorkspacePendingInteractionStatus(status)
	if status != model.ChatSessionStatusPendingConfirmation {
		return nil
	}
	cardType := workspaceInteractionCardType("", status)
	return &dto.WorkspaceInteraction{
		ID:             status + ":fallback",
		CardType:       cardType,
		ArtifactKind:   "agent_app_prd",
		Status:         status,
		Blocking:       true,
		Title:          workspaceInteractionTitle(cardType, status),
		ViewText:       workspaceInteractionViewText(cardType),
		AllowedActions: []string{"confirm_prd", "revise_prd", "cancel_prd", "view_prd"},
	}
}

func (s *WorkspaceChatService) RecordWorkspaceInteractionEvent(ctx context.Context, req *dto.RecordWorkspaceInteractionEventReq) error {
	if req == nil {
		return fmt.Errorf("交互事件不能为空")
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return fmt.Errorf("session_id 必填")
	}
	action := strings.TrimSpace(req.Action)
	if action == "" {
		return fmt.Errorf("action 必填")
	}
	session, err := s.sessionRepo.GetBySessionID(sessionID)
	if err != nil || session == nil {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}
	if err := ensureWorkspaceSessionOwner(ctx, session); err != nil {
		return err
	}
	user := contextx.GetRequestUser(ctx)
	if user == "" {
		user = session.User
	}
	displayContent := strings.TrimSpace(req.DisplayContent)
	if displayContent == "" {
		displayContent = workspaceInteractionEventDisplayContent(req)
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		content = displayContent
	}
	msg := &model.AgentChatMessage{
		SessionID:      sessionID,
		Role:           RoleUser,
		Content:        content,
		DisplayContent: displayContent,
		ContextUsage:   MessageContextDisplayOnly,
		ArtifactKind:   "workspace_interaction_event",
		User:           user,
	}
	msg.CreatedBy = user
	msg.UpdatedBy = user
	if err := s.messageRepo.Create(msg); err != nil {
		return err
	}
	if workspaceInteractionActionResolvesPending(action) && workspaceSessionHasPendingInteractionStatus(session.Status) {
		session.Status = model.ChatSessionStatusActive
	}
	session.UpdatedBy = user
	if err := s.sessionRepo.Update(session); err != nil {
		return fmt.Errorf("更新会话交互时间失败: %w", err)
	}
	return nil
}

func workspaceInteractionActionResolvesPending(action string) bool {
	switch strings.TrimSpace(action) {
	case "confirm_prd", "cancel_prd":
		return true
	default:
		return false
	}
}

func workspaceInteractionEventDisplayContent(req *dto.RecordWorkspaceInteractionEventReq) string {
	if req == nil {
		return "处理了工作台交互卡片"
	}
	label := workspaceInteractionActionLabel(req.Action)
	card := strings.TrimSpace(req.CardType)
	if card == "" {
		card = strings.TrimSpace(req.Status)
	}
	if card == "" {
		return "处理了工作台交互卡片：" + label
	}
	return fmt.Sprintf("处理了工作台交互卡片：%s（%s）", label, card)
}

func workspaceInteractionActionLabel(action string) string {
	switch strings.TrimSpace(action) {
	case "view_prd":
		return "查看方案"
	case "confirm_prd":
		return "按此方案开始"
	case "revise_prd":
		return "调整方案"
	case "cancel_prd":
		return "暂不使用"
	case "view_build_diagnostics":
		return "查看构建诊断"
	case "start_build_repair":
		return "交接构建修复"
	case "continue_development":
		return "继续修改"
	case "skip_build_repair":
		return "暂不修复"
	default:
		if strings.TrimSpace(action) == "" {
			return "未知动作"
		}
		return strings.TrimSpace(action)
	}
}
