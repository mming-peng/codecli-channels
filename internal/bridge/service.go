package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"qq-codex-go/internal/codex"
	cfgpkg "qq-codex-go/internal/config"
	"qq-codex-go/internal/qq"
	"qq-codex-go/internal/store"
)

type PendingTask struct {
	Mode         string
	Body         string
	Reason       string
	ProjectAlias string
	ProjectPath  string
	ExpiresAt    time.Time
}

type nativeApprovalState struct {
	Request   codex.ApprovalRequest
	Decision  chan codex.ApprovalDecision
	ExpiresAt time.Time
}

type Service struct {
	cfg      *cfgpkg.Config
	logger   *slog.Logger
	store    *store.Store
	api      *qq.APIClient
	runner   *codex.Runner
	audit    *AuditLogger
	gateways map[string]*qq.Gateway

	mu                   sync.Mutex
	busy                 map[string]bool
	pendingConfirmations map[string]PendingTask
	nativeApprovals      map[string]*nativeApprovalState
}

func NewService(cfg *cfgpkg.Config, logger *slog.Logger) (*Service, error) {
	stateStore, err := store.New(cfg.Bridge.StateFile)
	if err != nil {
		return nil, err
	}
	service := &Service{
		cfg:                  cfg,
		logger:               logger,
		store:                stateStore,
		api:                  qq.NewAPIClient(cfg),
		runner:               codex.NewRunner(),
		audit:                NewAuditLogger(cfg.Bridge.AuditFile),
		gateways:             map[string]*qq.Gateway{},
		busy:                 map[string]bool{},
		pendingConfirmations: map[string]PendingTask{},
		nativeApprovals:      map[string]*nativeApprovalState{},
	}
	for _, accountID := range cfg.Bridge.AccountIDs {
		account, err := cfg.ResolveAccount(accountID)
		if err != nil {
			return nil, err
		}
		if !account.Enabled {
			continue
		}
		service.gateways[accountID] = qq.NewGateway(cfg, service.api, accountID, logger.With("accountId", accountID), service.handleMessage)
	}
	if len(service.gateways) == 0 {
		return nil, fmt.Errorf("没有可启动的 QQ 账号")
	}
	return service, nil
}

func (s *Service) Start(ctx context.Context) error {
	for accountID, gateway := range s.gateways {
		s.logger.Info("启动 QQ 网关", "accountId", accountID)
		gateway.Start(ctx)
	}
	return nil
}

func (s *Service) handleMessage(ctx context.Context, msg qq.IncomingMessage) {
	if strings.TrimSpace(msg.Text) == "" {
		s.reply(ctx, msg, "我收到了空消息，换个说法试试？")
		return
	}
	if !s.isAllowedTarget(msg) {
		s.auditIfEnabled(AuditEvent{Status: "rejected", Reason: "target_not_allowed", AccountID: msg.AccountID, ChatType: msg.ChatType, TargetID: msg.TargetID, SenderID: msg.SenderID, Text: msg.Text})
		s.reply(ctx, msg, "当前目标未放行 Codex 执行，请先在配置里加入白名单。")
		return
	}

	text := strings.TrimSpace(msg.Text)
	s.logger.Info("收到 QQ 消息", "accountId", msg.AccountID, "chatType", msg.ChatType, "targetId", msg.TargetID, "senderId", msg.SenderID, "text", trimForLog(text, 200))
	switch {
	case text == "/ping":
		s.reply(ctx, msg, "pong，Go 版 QQ-Codex bridge 在线。")
		return
	case text == "/help":
		s.reply(ctx, msg, BuildHelpText(s.cfg.Bridge))
		return
	case strings.HasPrefix(text, "/project"):
		s.handleProjectCommand(ctx, msg)
		return
	case text == "/clear":
		s.handleClearCommand(ctx, msg)
		return
	case strings.HasPrefix(text, "/session"):
		s.handleSessionCommand(ctx, msg)
		return
	case strings.HasPrefix(text, "/mode"):
		s.handleModeCommand(ctx, msg)
		return
	case text == "/approve":
		s.handleApprove(ctx, msg)
		return
	case text == "/deny":
		s.handleDeny(ctx, msg)
		return
	}

	if s.handleNaturalApproval(ctx, msg, text) {
		return
	}

	parsed := ParseBridgeCommand(text, s.cfg.Bridge)
	if parsed.Type == "unmatched" {
		implicitMode := strings.ToLower(strings.TrimSpace(s.cfg.Bridge.ImplicitMessageMode))
		if implicitMode == "read" || implicitMode == "write" {
			go s.executeTask(context.Background(), msg, implicitMode, text)
			return
		}
		hint := "消息已收到，但没有匹配到可执行模式。"
		if s.cfg.Bridge.RequireCommandPrefix {
			hint = fmt.Sprintf("请使用 %s 或 %s 开头。\n\n%s", parsed.ReadOnlyPrefixes[0], parsed.WritePrefixes[0], BuildHelpText(s.cfg.Bridge))
		}
		s.reply(ctx, msg, hint)
		return
	}
	if parsed.Type == "confirm" {
		s.handleConfirm(ctx, msg)
		return
	}
	if parsed.Body == "" {
		s.reply(ctx, msg, "前缀后面还需要跟具体需求。")
		return
	}
	go s.executeTask(context.Background(), msg, parsed.Mode, parsed.Body)
}

func (s *Service) handleProjectCommand(ctx context.Context, msg qq.IncomingMessage) {
	parts := strings.Fields(strings.TrimSpace(msg.Text))
	subcommand := "list"
	if len(parts) > 1 {
		subcommand = strings.ToLower(parts[1])
	}
	conversationKey := msg.ConversationKey()
	current := s.resolveCurrentProject(conversationKey)
	switch subcommand {
	case "list":
		lines := []string{"可用项目："}
		for _, project := range s.cfg.ProjectList() {
			marker := "-"
			if current.Alias == project.Alias {
				marker = "*"
			}
			line := fmt.Sprintf("%s %s -> %s", marker, project.Alias, project.Path)
			if project.Description != "" {
				line += " (" + project.Description + ")"
			}
			lines = append(lines, line)
		}
		s.reply(ctx, msg, strings.Join(lines, "\n"))
	case "current":
		s.reply(ctx, msg, fmt.Sprintf("当前项目：%s\n目录：%s", current.Alias, current.Path))
	case "use", "switch":
		if len(parts) < 3 {
			s.reply(ctx, msg, "请提供项目别名，例如：/project use qq-codex-go")
			return
		}
		project, ok := s.cfg.Project(parts[2])
		if !ok {
			s.reply(ctx, msg, fmt.Sprintf("未找到项目别名：%s", parts[2]))
			return
		}
		if err := s.store.SetProjectAlias(conversationKey, project.Alias); err != nil {
			s.reply(ctx, msg, fmt.Sprintf("切换项目失败：%v", err))
			return
		}
		s.auditIfEnabled(AuditEvent{Status: "project_switched", AccountID: msg.AccountID, ChatType: msg.ChatType, TargetID: msg.TargetID, SenderID: msg.SenderID, ProjectAlias: project.Alias, ProjectPath: project.Path})
		s.reply(ctx, msg, fmt.Sprintf("已切换项目到 %s\n目录：%s", project.Alias, project.Path))
	default:
		s.reply(ctx, msg, "支持：/project list | /project current | /project use <别名>")
	}
}

func (s *Service) handleClearCommand(ctx context.Context, msg qq.IncomingMessage) {
	project := s.resolveCurrentProject(msg.ConversationKey())
	item, err := s.store.CreateSession(msg.ConversationKey(), project.Alias, project.Path, "session", s.cfg.Bridge.DefaultRunMode)
	if err != nil {
		s.reply(ctx, msg, fmt.Sprintf("开启新会话失败：%v", err))
		return
	}
	s.clearPendingConfirmation(msg.ConversationKey())
	s.clearNativeApproval(msg.ConversationKey())
	s.reply(ctx, msg, fmt.Sprintf("已开启新会话：%s (%s)\n项目：%s", item.Name, item.ID, project.Alias))
}

func (s *Service) handleSessionCommand(ctx context.Context, msg qq.IncomingMessage) {
	parts := strings.Fields(strings.TrimSpace(msg.Text))
	subcommand := "list"
	if len(parts) > 1 {
		subcommand = strings.ToLower(parts[1])
	}
	project := s.resolveCurrentProject(msg.ConversationKey())
	conversationKey := msg.ConversationKey()
	switch subcommand {
	case "list":
		items := s.store.ListSessions(conversationKey, project.Alias)
		if len(items) == 0 {
			s.reply(ctx, msg, "当前项目下还没有会话，先发一条普通消息或 /run 即可自动创建。")
			return
		}
		current := s.store.CurrentSession(conversationKey, project.Alias)
		lines := []string{fmt.Sprintf("项目 %s 的会话：", project.Alias)}
		for _, item := range items {
			marker := "-"
			if current != nil && current.ID == item.ID {
				marker = "*"
			}
			thread := item.ThreadID
			if thread == "" {
				thread = "(尚未绑定 Codex thread)"
			} else if len(thread) > 18 {
				thread = thread[:18] + "..."
			}
			lines = append(lines, fmt.Sprintf("%s %s | %s | %s", marker, item.ID, item.Name, thread))
		}
		s.reply(ctx, msg, strings.Join(lines, "\n"))
	case "current":
		item, err := s.store.GetOrCreateActiveSession(conversationKey, project.Alias, project.Path, s.cfg.Bridge.DefaultRunMode)
		if err != nil {
			s.reply(ctx, msg, fmt.Sprintf("读取当前会话失败：%v", err))
			return
		}
		thread := item.ThreadID
		if thread == "" {
			thread = "(尚未开始)"
		}
		s.reply(ctx, msg, fmt.Sprintf("当前会话：%s\n名称：%s\n项目：%s\nThread：%s\n默认模式：%s", item.ID, item.Name, item.ProjectAlias, thread, item.DefaultRunMode))
	case "new":
		name := "session"
		if len(parts) > 2 {
			name = strings.Join(parts[2:], " ")
		}
		item, err := s.store.CreateSession(conversationKey, project.Alias, project.Path, name, s.cfg.Bridge.DefaultRunMode)
		if err != nil {
			s.reply(ctx, msg, fmt.Sprintf("创建会话失败：%v", err))
			return
		}
		s.reply(ctx, msg, fmt.Sprintf("已创建会话：%s (%s)", item.Name, item.ID))
	case "switch":
		if len(parts) < 3 {
			s.reply(ctx, msg, "请提供会话 ID，例如：/session switch s2")
			return
		}
		item, err := s.store.SwitchSession(conversationKey, project.Alias, parts[2])
		if err != nil {
			s.reply(ctx, msg, err.Error())
			return
		}
		s.reply(ctx, msg, fmt.Sprintf("已切换到会话：%s (%s)", item.Name, item.ID))
	default:
		s.reply(ctx, msg, "支持：/session list | /session current | /session new [名称] | /session switch <id>")
	}
}

func (s *Service) handleModeCommand(ctx context.Context, msg qq.IncomingMessage) {
	parts := strings.Fields(strings.TrimSpace(msg.Text))
	project := s.resolveCurrentProject(msg.ConversationKey())
	session, err := s.store.GetOrCreateActiveSession(msg.ConversationKey(), project.Alias, project.Path, s.cfg.Bridge.DefaultRunMode)
	if err != nil {
		s.reply(ctx, msg, fmt.Sprintf("读取会话失败：%v", err))
		return
	}
	if len(parts) == 1 {
		s.reply(ctx, msg, fmt.Sprintf("当前默认执行模式：%s\n普通消息模式：%s\nwrite = %s", session.DefaultRunMode, s.cfg.Bridge.ImplicitMessageMode, s.cfg.Bridge.WriteCodexSandbox))
		return
	}
	mode := strings.ToLower(strings.TrimSpace(parts[1]))
	if mode != "write" && mode != "read" {
		s.reply(ctx, msg, "只支持 /mode write 或 /mode read")
		return
	}
	session.DefaultRunMode = mode
	if err := s.store.UpdateSession(session); err != nil {
		s.reply(ctx, msg, fmt.Sprintf("更新模式失败：%v", err))
		return
	}
	s.reply(ctx, msg, fmt.Sprintf("已将当前会话默认执行模式切换为 %s", mode))
}

func (s *Service) handleConfirm(ctx context.Context, msg qq.IncomingMessage) {
	pending, ok := s.getPendingConfirmation(msg.ConversationKey())
	if !ok {
		s.reply(ctx, msg, "当前没有待确认的高风险任务。")
		return
	}
	if pending.ProjectAlias != "" {
		_ = s.store.SetProjectAlias(msg.ConversationKey(), pending.ProjectAlias)
	}
	s.clearPendingConfirmation(msg.ConversationKey())
	go s.executeTask(context.Background(), msg, pending.Mode, pending.Body)
}

func (s *Service) handleApprove(ctx context.Context, msg qq.IncomingMessage) {
	if s.resolveNativeApproval(msg.ConversationKey(), codex.ApprovalAllow) {
		return
	}
	s.reply(ctx, msg, "当前没有待处理的审批请求。")
}

func (s *Service) handleDeny(ctx context.Context, msg qq.IncomingMessage) {
	if s.resolveNativeApproval(msg.ConversationKey(), codex.ApprovalDeny) {
		return
	}
	s.reply(ctx, msg, "当前没有待处理的审批请求。")
}

func (s *Service) executeTask(ctx context.Context, msg qq.IncomingMessage, mode, body string) {
	conversationKey := msg.ConversationKey()
	if !s.acquireBusy(conversationKey) {
		s.reply(ctx, msg, "我这边正在处理上一条任务，稍等一下再发我。⏳")
		return
	}
	defer s.releaseBusy(conversationKey)

	project := s.resolveCurrentProject(conversationKey)
	session, err := s.store.GetOrCreateActiveSession(conversationKey, project.Alias, project.Path, s.cfg.Bridge.DefaultRunMode)
	if err != nil {
		s.reply(ctx, msg, fmt.Sprintf("加载会话失败：%v", err))
		return
	}

	sandbox := ""
	if mode == "read" {
		sandbox = s.cfg.Bridge.ReadOnlyCodexSandbox
	}
	result, err := s.runner.RunTurn(ctx, codex.TurnOptions{
		Prompt:         body,
		ProjectAlias:   project.Alias,
		ProjectPath:    project.Path,
		TargetType:     msg.ChatType,
		SenderID:       msg.SenderID,
		TargetID:       msg.TargetID,
		ThreadID:       session.ThreadID,
		SandboxMode:    sandbox,
		Model:          s.cfg.Bridge.CodexModel,
		MaxPromptChars: s.cfg.Bridge.MaxPromptChars,
		Timeout:        time.Duration(s.cfg.Bridge.CodexTimeoutMs) * time.Millisecond,
		ApprovalHandler: func(req codex.ApprovalRequest) (codex.ApprovalDecision, error) {
			decisionCh := make(chan codex.ApprovalDecision, 1)
			state := &nativeApprovalState{Request: req, Decision: decisionCh, ExpiresAt: time.Now().Add(time.Duration(s.cfg.Bridge.ConfirmationTTLMS) * time.Millisecond)}
			s.setNativeApproval(conversationKey, state)
			prompt := []string{
				"Codex 原生审批请求：",
				"原因：" + req.Reason,
			}
			if strings.TrimSpace(req.Command) != "" {
				prompt = append(prompt, "命令：`"+req.Command+"`")
			}
			prompt = append(prompt, "回复“同意/拒绝”或 /approve /deny。")
			s.proactive(context.Background(), msg, strings.Join(prompt, "\n"))
			select {
			case decision := <-decisionCh:
				return decision, nil
			case <-time.After(time.Duration(s.cfg.Bridge.ConfirmationTTLMS) * time.Millisecond):
				s.clearNativeApproval(conversationKey)
				return codex.ApprovalDeny, fmt.Errorf("等待 QQ 审批超时")
			case <-ctx.Done():
				s.clearNativeApproval(conversationKey)
				return codex.ApprovalDeny, ctx.Err()
			}
		},
	})
	if result.ThreadID != "" && result.ThreadID != session.ThreadID {
		session.ThreadID = result.ThreadID
		_ = s.store.UpdateSession(session)
	}
	s.clearNativeApproval(conversationKey)
	if mode == "write" && looksLikeNeverPolicyResponse(result.ResponseText, result.CombinedText) && session.ThreadID != "" {
		oldThread := session.ThreadID
		session.ThreadID = ""
		_ = s.store.UpdateSession(session)
		s.auditIfEnabled(AuditEvent{Status: "thread_reset", Reason: "legacy_never_policy", AccountID: msg.AccountID, ChatType: msg.ChatType, TargetID: msg.TargetID, SenderID: msg.SenderID, ProjectAlias: project.Alias, ProjectPath: project.Path, SessionID: session.ID, Mode: mode, Text: oldThread})
		s.proactive(context.Background(), msg, fmt.Sprintf("%s\n\n%s",
			"我已经定位到原因：当前 QQ 会话绑定的是一条旧的 Codex 线程，它继承了历史上的 `never` 审批策略。",
			"我已自动切断这条旧线程绑定。请你把刚才那条需求再发一次；下一次会走新的、支持 Codex 原生审批的线程。",
		))
		return
	}

	if err != nil {
		s.auditIfEnabled(AuditEvent{Status: "failed", Reason: err.Error(), AccountID: msg.AccountID, ChatType: msg.ChatType, TargetID: msg.TargetID, SenderID: msg.SenderID, ProjectAlias: project.Alias, ProjectPath: project.Path, SessionID: session.ID, Mode: mode, Text: body})
		s.proactive(context.Background(), msg, buildFailureReply(err, result, body))
		return
	}
	content := strings.TrimSpace(result.ResponseText)
	if content == "" {
		content = "我这边执行完成了，但没有拿到可返回的文本结果。"
	}
	s.auditIfEnabled(AuditEvent{Status: "success", AccountID: msg.AccountID, ChatType: msg.ChatType, TargetID: msg.TargetID, SenderID: msg.SenderID, ProjectAlias: project.Alias, ProjectPath: project.Path, SessionID: session.ID, Mode: mode, Text: body})
	for _, chunk := range splitForQQ(trimForQQ(content, s.cfg.Bridge.QQMaxReplyChars), s.cfg.Bridge.QQMaxReplyChars) {
		s.proactive(context.Background(), msg, chunk)
	}
}

func buildFailureReply(execErr error, result codex.TurnResult, userBody string) string {
	details := strings.TrimSpace(result.ResponseText)
	if details == "" {
		details = extractFailureSummary(result.CombinedText, userBody)
	}
	if details == "" {
		return fmt.Sprintf("执行失败：%v", execErr)
	}
	return fmt.Sprintf("执行失败：%v\n\n%s", execErr, tailText(details, 1200))
}

func extractFailureSummary(text, userBody string) string {
	filtered := filterProgressForQQ(text, userBody)
	if filtered == "" {
		return ""
	}
	keywords := []string{"失败", "错误", "超时", "拒绝", "权限", "审批", "沙箱", "error", "failed", "timeout", "denied", "permission", "approval", "sandbox", "not permitted", "operation not permitted", "network"}
	lines := strings.Split(filtered, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		if lower == "" {
			continue
		}
		for _, keyword := range keywords {
			if strings.Contains(lower, strings.ToLower(keyword)) {
				kept = append(kept, line)
				break
			}
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func (s *Service) resolveCurrentProject(conversationKey string) cfgpkg.ProjectConfig {
	alias := s.store.GetProjectAlias(conversationKey, s.cfg.Bridge.DefaultProject)
	project, ok := s.cfg.Project(alias)
	if ok {
		return project
	}
	project, _ = s.cfg.Project(s.cfg.Bridge.DefaultProject)
	return project
}

func (s *Service) isAllowedTarget(msg qq.IncomingMessage) bool {
	if s.cfg.Bridge.AllowAllTargets {
		return true
	}
	scoped := msg.AccountID + ":" + msg.ChatType + ":" + msg.TargetID
	unscoped := msg.ChatType + ":" + msg.TargetID
	for _, item := range s.cfg.Bridge.AllowedTargets {
		if item == scoped || item == unscoped {
			return true
		}
	}
	return false
}

func (s *Service) reply(ctx context.Context, msg qq.IncomingMessage, content string) {
	content = trimForQQ(content, s.cfg.Bridge.QQMaxReplyChars)
	s.logger.Info("发送 QQ 回复", "accountId", msg.AccountID, "chatType", msg.ChatType, "targetId", msg.TargetID, "replyTo", msg.MessageID, "content", trimForLog(content, 200))
	if err := s.api.ReplyMessage(ctx, msg.AccountID, msg.ChatType, msg.TargetID, msg.MessageID, content); err != nil {
		s.logger.Error("回复消息失败", "accountId", msg.AccountID, "chatType", msg.ChatType, "targetId", msg.TargetID, "replyTo", msg.MessageID, "error", err)
	}
}

func (s *Service) proactive(ctx context.Context, msg qq.IncomingMessage, content string) {
	content = trimForQQ(content, s.cfg.Bridge.QQMaxReplyChars)
	s.logger.Info("发送 QQ 主动消息", "accountId", msg.AccountID, "chatType", msg.ChatType, "targetId", msg.TargetID, "content", trimForLog(content, 200))
	if err := s.api.ProactiveMessage(ctx, msg.AccountID, msg.ChatType, msg.TargetID, content); err != nil {
		s.logger.Error("主动消息发送失败", "accountId", msg.AccountID, "chatType", msg.ChatType, "targetId", msg.TargetID, "error", err)
	}
}

func (s *Service) auditIfEnabled(event AuditEvent) {
	if !s.cfg.Bridge.AuditEnabled {
		return
	}
	if err := s.audit.Write(event); err != nil {
		s.logger.Error("写审计日志失败", "error", err)
	}
}

func (s *Service) acquireBusy(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy[key] {
		return false
	}
	s.busy[key] = true
	return true
}

func (s *Service) releaseBusy(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.busy, key)
}

func (s *Service) setPendingConfirmation(key string, task PendingTask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingConfirmations[key] = task
}

func (s *Service) getPendingConfirmation(key string) (PendingTask, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.pendingConfirmations[key]
	if !ok {
		return PendingTask{}, false
	}
	if time.Now().After(task.ExpiresAt) {
		delete(s.pendingConfirmations, key)
		return PendingTask{}, false
	}
	return task, true
}

func (s *Service) clearPendingConfirmation(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pendingConfirmations, key)
}

func (s *Service) setNativeApproval(key string, state *nativeApprovalState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nativeApprovals[key] = state
}

func (s *Service) clearNativeApproval(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nativeApprovals, key)
}

func (s *Service) resolveNativeApproval(key string, decision codex.ApprovalDecision) bool {
	s.mu.Lock()
	state, ok := s.nativeApprovals[key]
	if ok {
		delete(s.nativeApprovals, key)
	}
	s.mu.Unlock()
	if !ok || state == nil || time.Now().After(state.ExpiresAt) {
		return false
	}
	select {
	case state.Decision <- decision:
	default:
	}
	return true
}

func (s *Service) handleNaturalApproval(_ context.Context, msg qq.IncomingMessage, text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if isAllowResponse(lower) {
		return s.resolveNativeApproval(msg.ConversationKey(), codex.ApprovalAllow)
	}
	if isDenyResponse(lower) {
		return s.resolveNativeApproval(msg.ConversationKey(), codex.ApprovalDeny)
	}
	return false
}

func isAllowResponse(text string) bool {
	words := []string{"同意", "通过", "继续", "可以", "好", "好的", "允许", "yes", "y", "ok", "approve"}
	for _, word := range words {
		if text == strings.ToLower(word) {
			return true
		}
	}
	return false
}

func isDenyResponse(text string) bool {
	words := []string{"拒绝", "驳回", "取消", "不同意", "不可以", "不要", "不", "deny", "no", "n", "cancel"}
	for _, word := range words {
		if text == strings.ToLower(word) {
			return true
		}
	}
	return false
}

func filterProgressForQQ(text, userBody string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isQQRelayNoiseLine(line, userBody) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func isQQRelayNoiseLine(line, userBody string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if lower == "" {
		return true
	}
	if lower == "thinking" || strings.HasPrefix(lower, "**considering") {
		return true
	}
	if strings.TrimSpace(line) == strings.TrimSpace(userBody) {
		return true
	}
	contains := []string{
		"请使用中文回答。",
		"succeeded in ",
		"failed in ",
		"用户消息如下：",
		"发送者 id：",
		"目标 id：",
		"你正在通过官方 qq 机器人为本机用户提供 codex 远程代理服务",
		"必须保留 codex 原生审批",
		"优先直接给出结论和最小必要说明",
		"当前项目别名：",
		"当前项目目录：",
		"当前消息来源：",
		"发送者 id：",
		"目标 id：",
		"用户消息如下：",
		"workdir:",
		"provider:",
		"approval:",
		"sandbox:",
		"reasoning effort:",
		"reasoning summaries:",
		"session id:",
		"tokens used",
		"mcp:",
		"mcp startup:",
		"chrome-devtools",
		"context7",
		"sequential-thinking",
		"shell_snapshot",
		"model personality requested",
		"warn codex_",
		"/users/ming/.codex/",
		"under-development features enabled",
		"startup_timeout_sec",
	}
	for _, item := range contains {
		if strings.Contains(lower, item) {
			return true
		}
	}
	prefixes := []string{
		"hi",
		"none",
		"ready",
		"exec",
		"--------",
		"user",
		"codex",
		"session id:",
		"workdir:",
		"provider:",
		"approval:",
		"sandbox:",
	}
	for _, prefix := range prefixes {
		if lower == prefix || strings.HasPrefix(lower, prefix+" ") {
			return true
		}
	}
	if matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}t.*(warn|info)`, lower); matched {
		return true
	}
	if matched, _ := regexp.MatchString(`^(succeeded|failed) in \d+(ms|s|m):?$`, lower); matched {
		return true
	}
	if (strings.HasPrefix(lower, "/bin/") || strings.HasPrefix(lower, "bash ") || strings.HasPrefix(lower, "zsh ") || strings.HasPrefix(lower, "sh ")) && strings.Contains(lower, " in /users/") {
		return true
	}
	if matched, _ := regexp.MatchString(`^[0-9,]+$`, lower); matched {
		return true
	}
	return false
}

func normalizeRelayText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.Join(strings.Fields(text), " ")
	return text
}

func shouldSkipFinalReply(streamed, final string) bool {
	left := normalizeRelayText(streamed)
	right := normalizeRelayText(final)
	if left == "" || right == "" {
		return false
	}
	return strings.Contains(left, right)
}

func trimForQQ(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "我这边执行完成了，但没有拿到可返回的文本结果。"
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	if maxChars < 32 {
		return string(runes[:maxChars])
	}
	return string(runes[:maxChars-10]) + "\n\n……已截断"
}

func tailText(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return "……\n" + string(runes[len(runes)-maxChars:])
}

func trimForLog(text string, maxChars int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxChars {
		return string(runes)
	}
	return string(runes[:maxChars]) + "..."
}

func splitForQQ(text string, maxChars int) []string {
	runes := []rune(text)
	if len(runes) <= maxChars {
		return []string{text}
	}
	parts := make([]string, 0)
	for len(runes) > 0 {
		take := maxChars
		if len(runes) < take {
			take = len(runes)
		}
		parts = append(parts, string(runes[:take]))
		runes = runes[take:]
	}
	return parts
}

func looksLikeNeverPolicyResponse(values ...string) bool {
	patterns := []string{
		"审批策略是 `never`",
		"审批策略是 never",
		"approval policy is `never`",
		"approval policy is never",
		"approval policy = `never`",
		"approval policy = never",
		"approval_policy = `never`",
		"approval_policy = never",
		"approval_policy=never",
		"当前运行策略是 `approval_policy = never`",
		"不能发起原生审批请求",
	}
	for _, value := range values {
		lower := strings.ToLower(strings.TrimSpace(value))
		for _, pattern := range patterns {
			if strings.Contains(lower, strings.ToLower(pattern)) {
				return true
			}
		}
	}
	return false
}

func SortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
