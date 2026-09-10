package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kageos/kageos/core/agent-server/model"
	"github.com/kageos/kageos/core/agent-server/prompt"
	"github.com/kageos/kageos/core/agent-server/repository"
	"github.com/kageos/kageos/core/agent-server/streamloop"
	"github.com/kageos/kageos/dto"
	"github.com/kageos/kageos/pkg/apicall"
	"github.com/kageos/kageos/pkg/contextx"
	"github.com/kageos/kageos/pkg/logger"
	"github.com/kageos/kageos/pkg/secretvault"
)

const (
	SourceWorkspace       = model.ChatSessionSourceWorkspace
	SourceAutomationAgent = model.ChatSessionSourceAutomationAgent
)

// 工作台系统提示词与内置文档统一来自本地内嵌的 /system/prompt。

// 消息角色常量
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

const (
	ContextPolicyFull         = "full"
	ContextPolicyArtifactOnly = "artifact_only"
	ContextPolicyDisplayOnly  = "display_only"

	MessageContextInclude     = dto.WorkspaceMessageContextInclude
	MessageContextDisplayOnly = dto.WorkspaceMessageContextDisplayOnly
	MessageContextArtifact    = dto.WorkspaceMessageContextArtifact
	MessageContextCurrentTurn = dto.WorkspaceMessageContextCurrentTurn
)

// 工具调用状态常量
const (
	ToolCallStatusOK        = dto.WorkspaceToolCallStatusOK
	ToolCallStatusError     = dto.WorkspaceToolCallStatusError
	ToolCallStatusRunning   = dto.WorkspaceToolCallStatusRunning
	ToolCallStatusStreaming = dto.WorkspaceToolCallStatusStreaming
)

// 流式事件类型常量
const (
	EventSession              = dto.WorkspaceStreamEventSession
	EventModelContextPlan     = dto.WorkspaceStreamEventModelContextPlan
	EventToolCall             = dto.WorkspaceStreamEventToolCall
	EventToolCallsStreamDelta = dto.WorkspaceStreamEventToolCallsStreamDelta
	EventContent              = dto.WorkspaceStreamEventContent
	EventDone                 = dto.WorkspaceStreamEventDone
	EventError                = dto.WorkspaceStreamEventError
)

// WorkspaceChatService 工作台对话编排：会话、历史、LLM、Tool 循环；只认 LLM + 单模式（dev）
type WorkspaceChatService struct {
	toolReg        *ToolRegistry
	sessionRepo    *repository.ChatSessionRepository
	messageRepo    *repository.ChatMessageRepository
	llmRepo        *repository.LLMRepository
	runtimeState   RuntimeStateStore
	apiKeyVault    *secretvault.Vault
	apiKeyVaultErr error

	// runningCancels 维护「正在执行的 session → cancelFunc」映射，供手动取消使用
	runningCancels sync.Map // key: sessionID (string), value: context.CancelFunc
	// sseConnections 维护「有活跃 SSE 连接的 session」，供前端存活检测，避免无谓轮询大消息列表
	sseConnections sync.Map // key: sessionID (string), value: struct{}
}

// NewWorkspaceChatService 创建 WorkspaceChatService
func NewWorkspaceChatService(
	toolReg *ToolRegistry,
	sessionRepo *repository.ChatSessionRepository,
	messageRepo *repository.ChatMessageRepository,
	llmRepo *repository.LLMRepository,
	runtimeState ...RuntimeStateStore,
) *WorkspaceChatService {
	var stateStore RuntimeStateStore
	if len(runtimeState) > 0 {
		stateStore = runtimeState[0]
	}
	apiKeyVault, apiKeyVaultErr := newLLMAPIKeyVault(defaultLLMAPIKeySecret())
	return &WorkspaceChatService{
		toolReg:        toolReg,
		sessionRepo:    sessionRepo,
		messageRepo:    messageRepo,
		llmRepo:        llmRepo,
		runtimeState:   stateStore,
		apiKeyVault:    apiKeyVault,
		apiKeyVaultErr: apiKeyVaultErr,
	}
}

// StreamEvent 流式事件：用于 SSE 传输
type StreamEvent = dto.WorkspaceStreamEvent

// WorkspaceChatEventSink 接收工作台执行事件。SSE 与后台任务共用同一执行链路。
type WorkspaceChatEventSink interface {
	Send(event string, data interface{})
}

type workspaceChatEventSinkFunc func(event string, data interface{})

func (f workspaceChatEventSinkFunc) Send(event string, data interface{}) {
	f(event, data)
}

// StreamEventSession session 事件数据
type StreamEventSession = dto.WorkspaceStreamSession

// StreamEventToolCall tool_call 事件数据
type StreamEventToolCall = dto.WorkspaceStreamToolCall

// StreamEventContent content 事件数据
type StreamEventContent = dto.WorkspaceStreamContent

// StreamEventDone done 事件数据
type StreamEventDone = dto.WorkspaceStreamDone

type messageLLMMetadata struct {
	ConfigID   int64
	ConfigName string
	Provider   string
	Model      string
}

// StreamEventError error 事件数据
type StreamEventError = dto.WorkspaceStreamError

// WorkspaceChatStream 工作台对话流式入口：通过 eventChan 发送 SSE 事件（session、tool_call、content、done、error）
// eventChan 为只写 channel，由调用方在 goroutine 中读取并写 SSE，避免 Flush 阻塞 LLM stream 消费
func (s *WorkspaceChatService) WorkspaceChatStream(ctx context.Context, req *dto.WorkspaceChatReq, eventChan chan<- StreamEvent) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("WorkspaceChatStream panic: %v", e)
			select {
			case eventChan <- StreamEvent{Event: EventError, Data: StreamEventError{Message: fmt.Sprintf("%v", e)}}:
			default:
			}
		}
	}()
	var runtimeStateKey string
	var runtimeStateBase dto.RuntimeStateItem
	// 非阻塞发送事件：写不进 eventChan 时直接丢弃（刷新后无人读也不会卡死 goroutine）
	sendEvent := func(event string, data interface{}) {
		s.updateWorkspaceRuntimeStateFromEvent(ctx, runtimeStateKey, runtimeStateBase, event, data)
		select {
		case eventChan <- StreamEvent{Event: event, Data: data}:
		default:
			// channel 满或无人读，丢弃该事件（事件已存 DB，前端可通过轮询消息恢复）
		}
	}
	user := contextx.GetRequestUser(ctx)
	requestedPath := normalizeWorkspacePath(req.FullCodePath)
	explicitResourcePath := normalizeWorkspacePath(req.ResourceFullCodePath)
	fullCodePath := requestedPath
	resourceFullCodePath := explicitResourcePath
	var resourceTreeID int64
	if requestedPath == "" {
		return s.handleError(sendEvent, "full_code_path 必填", nil)
	}
	requestedModeCode := normalizeWorkspaceModeCode(req.ModeCode)
	requestedModeProvider := prompt.ResolveModeProvider(ctx, requestedModeCode)
	if req.SessionID == "" && requestedModeProvider == nil {
		return s.handleError(sendEvent, fmt.Sprintf("不支持的 mode_code: %s", requestedModeCode), nil)
	}

	// 1) 解析或创建 session。已有会话优先使用会话自身目录，避免阶段交接后被前端旧面板路径带回来源目录。
	var session *model.AgentChatSession
	if req.SessionID != "" {
		var e error
		session, e = s.sessionRepo.GetBySessionID(req.SessionID)
		if e != nil || session == nil {
			return s.handleError(sendEvent, fmt.Sprintf("会话不存在: %s", req.SessionID), e)
		}
		ctx, user = workspaceContextWithSessionRequestUser(ctx, session)
		if e := ensureWorkspaceSessionOwner(ctx, session); e != nil {
			return s.handleError(sendEvent, e.Error(), e)
		}
		if sessionPath := strings.TrimSpace(session.FullCodePath); sessionPath != "" {
			fullCodePath = normalizeWorkspacePath(sessionPath)
		}
		resourceFullCodePath = normalizeWorkspacePath(session.ResourceFullCodePath)
		resourceTreeID = session.ResourceTreeID
	}

	// 2) 获取工作台环境信息（包含目录详情、子节点等，一次性获取，避免重复调用）
	workspaceCtx, e := apicall.GetWorkspaceContext(ctx, fullCodePath, "")
	if e != nil || workspaceCtx == nil {
		// 保留上游故障原因，避免 MySQL/网络故障被误报成目录不存在。
		message := fmt.Sprintf("无法解析工作台上下文: %s", fullCodePath)
		if e != nil {
			message += "；原因: " + e.Error()
		}
		return s.handleError(sendEvent, message, e)
	}
	directoryPath := normalizeWorkspacePath(workspaceCtx.Directory.FullCodePath)
	if directoryPath == "" {
		return s.handleError(sendEvent, "工作台上下文未返回有效目录", nil)
	}
	if req.SessionID == "" {
		if resourceFullCodePath == "" && requestedPath != directoryPath {
			resourceFullCodePath = requestedPath
		}
		if resourceFullCodePath == directoryPath {
			resourceFullCodePath = ""
		}
		if resourceFullCodePath != "" {
			var belongs bool
			resourceTreeID, belongs = workspaceContextResourceTreeID(workspaceCtx.Children, resourceFullCodePath)
			if !belongs {
				return s.handleError(sendEvent, fmt.Sprintf("resource_full_code_path 不属于工作台目录: %s", resourceFullCodePath), nil)
			}
		}
	}
	// 只调整本次执行上下文，不回写已有会话或历史记录。
	fullCodePath = directoryPath
	directoryName := workspaceCtx.Directory.Name
	if directoryName == "" {
		directoryName = workspaceCtx.Directory.Code
	}

	// 3) 创建新 session
	if req.SessionID == "" {
		source := SourceWorkspace
		automation := scheduledAgentSessionIdentityFromContext(ctx)
		if automation.TaskID > 0 {
			source = SourceAutomationAgent
		}
		session = &model.AgentChatSession{
			TreeID:               workspaceCtx.Directory.ID,
			FullCodePath:         fullCodePath,
			ResourceTreeID:       resourceTreeID,
			ResourceFullCodePath: resourceFullCodePath,
			Source:               source,
			AutomationTaskID:     automation.TaskID,
			AutomationTaskCode:   automation.TaskCode,
			AutomationTaskTitle:  automation.TaskTitle,
			SessionID:            uuid.New().String(),
			Title:                "",
			ModeCode:             requestedModeCode,
			Status:               model.ChatSessionStatusActive,
			ContextPolicy:        ContextPolicyFull,
			User:                 user,
		}
		applyDefaultWorkspaceSessionRole(session)
		session.CreatedBy = user
		session.UpdatedBy = user
		if e := s.sessionRepo.Create(session); e != nil {
			return s.handleError(sendEvent, "创建会话失败", e)
		}
	}
	modeCode := requestedModeCode
	if req.SessionID != "" && strings.TrimSpace(req.ModeCode) == "" {
		modeCode = normalizeWorkspaceModeCode(session.ModeCode)
	}
	if session.ArchivedForModel || session.ContextPolicy != ContextPolicyFull || session.ModelContextAnchorMessageID != 0 {
		session.ArchivedForModel = false
		session.ArchiveReason = ""
		session.ContextPolicy = ContextPolicyFull
		session.ModelContextAnchorMessageID = 0
		if session.Status == model.ChatSessionStatusDone {
			session.Status = model.ChatSessionStatusActive
		}
		session.UpdatedBy = user
		if e := s.sessionRepo.Update(session); e != nil {
			return s.handleError(sendEvent, "恢复完整会话上下文失败", e)
		}
	}
	if session.Status == model.ChatSessionStatusGenerating {
		return s.handleError(sendEvent, "该会话正在执行中，请等待当前任务完成，或先取消后再继续。", nil)
	}
	modeProvider := requestedModeProvider
	if modeCode != requestedModeCode || modeProvider == nil {
		modeProvider = prompt.ResolveModeProvider(ctx, modeCode)
	}
	var toolNames []string
	var systemPromptFragment string
	if modeProvider == nil {
		return s.handleError(sendEvent, fmt.Sprintf("不支持的 mode_code: %s", modeCode), nil)
	}

	sessionID := session.SessionID
	resumeExistingMessage := req.Resume
	if resumeExistingMessage {
		if req.SessionID == "" {
			return s.handleError(sendEvent, "resume 必须指定 session_id", nil)
		}
		if e := s.ensureWorkspaceSessionHasRunnableMessage(sessionID); e != nil {
			return s.handleError(sendEvent, e.Error(), e)
		}
	}
	s.sseConnections.Store(sessionID, struct{}{})

	// ⭐ 标记会话为 generating（后台执行中）
	markedGenerating, e := s.sessionRepo.TryMarkGenerating(sessionID, user, modeCode)
	if e != nil {
		logger.Warnf(ctx, "[WorkspaceChatStream] 标记 generating 失败: %v", e)
		s.sseConnections.Delete(sessionID)
		return s.handleError(sendEvent, "标记会话执行中失败", e)
	} else if !markedGenerating {
		s.sseConnections.Delete(sessionID)
		return s.handleError(sendEvent, "该会话正在执行中，请等待当前任务完成，或先取消后再继续。", nil)
	}
	session.Status = model.ChatSessionStatusGenerating
	session.ModeCode = modeCode
	session.UpdatedBy = user

	// ⭐ 创建可取消的 context 并注册到 runningCancels，供 CancelSession 使用
	runCtx, runCancel := context.WithCancel(ctx)
	s.runningCancels.Store(sessionID, runCancel)
	// 无论正常结束还是异常，都要恢复状态并清理
	defer func() {
		runCancel()
		s.runningCancels.Delete(sessionID)
		s.sseConnections.Delete(sessionID)
		// ⭐ 恢复会话状态：已取消的保持 cancelled，否则标回 active
		latest, e := s.sessionRepo.GetBySessionID(sessionID)
		if e == nil && latest != nil && latest.Status == model.ChatSessionStatusGenerating {
			latest.Status = model.ChatSessionStatusActive
			latest.UpdatedBy = user
			if e := s.sessionRepo.Update(latest); e != nil {
				logger.Warnf(ctx, "[WorkspaceChatStream] 恢复 active 失败: %v", e)
			}
		}
		finalStatus := ""
		if e == nil && latest != nil && latest.Status == model.ChatSessionStatusCancelled {
			finalStatus = RuntimeStateStatusCancelled
		}
		finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer finishCancel()
		s.finishWorkspaceRuntimeState(finishCtx, runtimeStateKey, runtimeStateBase, err, finalStatus)
	}()

	llmConfigID := req.LLMConfigID

	// 3) 保存 user 消息。handoff 原子化场景下，首条 artifact 消息已在 handoff 事务内落库，此处只续跑。
	var currentMessageID int64
	if !resumeExistingMessage {
		storageContent, storageFiles := userContentForStorage(req.Message.Files, req.Message.Content)
		userMsg := &model.AgentChatMessage{
			SessionID: sessionID, Role: RoleUser,
			Content: storageContent, DisplayContent: strings.TrimSpace(req.Message.DisplayContent), Files: storageFiles,
			ContextUsage: normalizeMessageContextUsage(req.Message.ContextUsage),
			ArtifactKind: strings.TrimSpace(req.Message.ArtifactKind),
			User:         user,
		}
		userMsg.CreatedBy = user
		userMsg.UpdatedBy = user
		if e := s.messageRepo.Create(userMsg); e != nil {
			return s.handleError(sendEvent, "保存用户消息失败", e)
		}
		currentMessageID = userMsg.ID
	}

	// 3.1) 新会话自动生成标题
	if !resumeExistingMessage && session.Title == "" {
		title := strings.TrimSpace(req.Message.DisplayContent)
		if title == "" {
			title = strings.TrimSpace(req.Message.Content)
		}
		title = strings.ReplaceAll(title, "\n", " ")
		if len(title) > 50 {
			title = title[:50] + "..."
		}
		if title == "" {
			title = "新会话"
		}
		session.Title = title
		session.UpdatedBy = user
		if e := s.sessionRepo.Update(session); e != nil {
			logger.Warnf(ctx, "[WorkspaceChatStream] 更新会话标题失败: %v", e)
		} else {
			logger.Infof(ctx, "[WorkspaceChatStream] 会话标题已生成 - SessionID: %s, Title: %s", sessionID, title)
		}
	}

	runtimeStateKey, runtimeStateBase = s.startWorkspaceRuntimeState(ctx, session, fullCodePath, modeCode, user)
	sendEvent(EventSession, StreamEventSession{SessionID: sessionID})

	deps := &workspaceStreamLoopDeps{
		ctx:                  runCtx,
		sendEvent:            sendEvent,
		sessionID:            sessionID,
		fullCodePath:         fullCodePath,
		llmConfigID:          llmConfigID,
		user:                 user,
		modeProvider:         modeProvider,
		toolNames:            toolNames,
		systemPromptFragment: systemPromptFragment,
		files:                req.Message.Files,
		currentMessageID:     currentMessageID,
		service:              s,
	}
	return streamloop.RunStreamLoop(runCtx, deps)
}

func workspaceContextResourceTreeID(children []dto.WorkspaceContextNode, resourceFullCodePath string) (int64, bool) {
	resourceFullCodePath = normalizeWorkspacePath(resourceFullCodePath)
	for _, child := range children {
		if normalizeWorkspacePath(child.FullCodePath) == resourceFullCodePath {
			return child.ID, true
		}
	}
	return 0, false
}

func workspaceContextWithSessionRequestUser(ctx context.Context, session *model.AgentChatSession) (context.Context, string) {
	requestUser := strings.TrimSpace(contextx.GetRequestUser(ctx))
	if hasWorkspaceRequestUser(requestUser) {
		return ctx, requestUser
	}
	owner := workspaceSessionOwner(session)
	if hasWorkspaceRequestUser(owner) {
		return contextx.WithRequestUser(ctx, owner), owner
	}
	return ctx, requestUser
}

func workspaceSessionOwner(session *model.AgentChatSession) string {
	if session == nil {
		return ""
	}
	return firstNonEmptyString(session.User, session.CreatedBy, session.UpdatedBy)
}

func hasWorkspaceRequestUser(user string) bool {
	user = strings.TrimSpace(user)
	return user != ""
}

// RunWorkspaceChat 后台执行工作台对话；用于定时 Agent 会话等无浏览器连接场景。
func (s *WorkspaceChatService) RunWorkspaceChat(ctx context.Context, req *dto.WorkspaceChatReq, sink WorkspaceChatEventSink) error {
	if sink == nil {
		sink = workspaceChatEventSinkFunc(func(string, interface{}) {})
	}
	eventChan := make(chan StreamEvent, 100)
	done := make(chan error, 1)
	go func() {
		done <- s.WorkspaceChatStream(ctx, req, eventChan)
		close(eventChan)
	}()
	for ev := range eventChan {
		sink.Send(ev.Event, ev.Data)
	}
	return <-done
}

// handleError 统一错误处理：发送错误事件并返回错误
func (s *WorkspaceChatService) handleError(sendEvent func(string, interface{}), message string, err error) error {
	sendEvent(EventError, StreamEventError{Message: message})
	if err != nil {
		return fmt.Errorf("%s: %w", message, err)
	}
	return fmt.Errorf("%s", message)
}
