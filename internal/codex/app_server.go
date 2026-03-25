package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const appServerSessionIdleTTL = 15 * time.Minute

type AppServerRunner struct {
	Binary   string
	mu       sync.Mutex
	closed   bool
	sessions map[string]*appServerSession
}

type appServerSession struct {
	key         string
	projectPath string
	codexHome   string
	binary      string

	turnMu  sync.Mutex
	writeMu sync.Mutex
	mu      sync.Mutex

	cmd        *exec.Cmd
	stdin      io.WriteCloser
	pending    map[string]chan rpcEnvelope
	current    *appServerTurn
	nextID     int64
	started    bool
	closed     bool
	closeErr   error
	lastUsedAt time.Time
	running    bool
}

type appServerTurn struct {
	opts TurnOptions
	done chan struct{}
	once sync.Once
	mu   sync.Mutex

	threadID         string
	turnID           string
	responseText     string
	lastProgressText string
	combined         []string
	exitCode         int
	err              error
}

type rpcEnvelope struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcSuccess struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result"`
}

type rpcFailure struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Error   rpcError        `json:"error"`
}

type initializeResponse struct {
	UserAgent string `json:"userAgent"`
}

type threadStartLikeResponse struct {
	Thread struct {
		ID string `json:"id"`
	} `json:"thread"`
}

type turnStartResponse struct {
	Turn struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"turn"`
}

type turnStartedNotification struct {
	ThreadID string `json:"threadId"`
	Turn     struct {
		ID string `json:"id"`
	} `json:"turn"`
}

type turnCompletedNotification struct {
	ThreadID string `json:"threadId"`
	Turn     struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Error  *struct {
			Message           string  `json:"message"`
			AdditionalDetails *string `json:"additionalDetails"`
		} `json:"error"`
	} `json:"turn"`
}

type itemNotification struct {
	ThreadID string          `json:"threadId"`
	TurnID   string          `json:"turnId"`
	Item     json.RawMessage `json:"item"`
}

type errorNotification struct {
	Error struct {
		Message           string  `json:"message"`
		AdditionalDetails *string `json:"additionalDetails"`
	} `json:"error"`
	ThreadID  string `json:"threadId"`
	TurnID    string `json:"turnId"`
	WillRetry bool   `json:"willRetry"`
}

type codexEventNotification struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	Msg            struct {
		Type              string  `json:"type"`
		Message           string  `json:"message"`
		AdditionalDetails string  `json:"additional_details"`
		LastAgentMessage  *string `json:"last_agent_message"`
	} `json:"msg"`
}

type agentMessageItem struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Text  string `json:"text"`
	Phase string `json:"phase"`
}

type commandExecutionItem struct {
	ID               string  `json:"id"`
	Type             string  `json:"type"`
	Command          string  `json:"command"`
	Status           string  `json:"status"`
	AggregatedOutput *string `json:"aggregatedOutput"`
	ExitCode         *int    `json:"exitCode"`
}

type fileChangeItem struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

type commandApprovalParams struct {
	ApprovalID string `json:"approvalId"`
	ThreadID   string `json:"threadId"`
	TurnID     string `json:"turnId"`
	Command    string `json:"command"`
	Cwd        string `json:"cwd"`
	Reason     string `json:"reason"`
	ItemID     string `json:"itemId"`
}

type fileChangeApprovalParams struct {
	ThreadID  string `json:"threadId"`
	TurnID    string `json:"turnId"`
	Reason    string `json:"reason"`
	GrantRoot string `json:"grantRoot"`
	ItemID    string `json:"itemId"`
}

type legacyExecApprovalParams struct {
	ApprovalID     string   `json:"approvalId"`
	ConversationID string   `json:"conversationId"`
	Command        []string `json:"command"`
	Cwd            string   `json:"cwd"`
	Reason         string   `json:"reason"`
}

func NewAppServerRunner() *AppServerRunner {
	return &AppServerRunner{
		Binary:   "codex",
		sessions: map[string]*appServerSession{},
	}
}

func (r *AppServerRunner) RunTurn(ctx context.Context, opts TurnOptions) (TurnResult, error) {
	key := scopedRunnerSessionKey(opts.ProjectPath, opts.SessionID)
	now := time.Now()
	staleSessions := make([]*appServerSession, 0)
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return TurnResult{}, fmt.Errorf("app-server runner 已关闭")
	}
	staleSessions = r.collectReapableSessionsLocked(now.Add(-appServerSessionIdleTTL))
	session := r.sessions[key]
	if session != nil && session.isClosed() {
		delete(r.sessions, key)
		staleSessions = append(staleSessions, session)
		session = nil
	}
	if session == nil {
		codexHome, err := prepareScopedCodexRuntimeHome(opts.ProjectPath, key)
		if err != nil {
			r.mu.Unlock()
			return TurnResult{}, err
		}
		session = newAppServerSession(key, opts.ProjectPath, codexHome, r.Binary)
		if err := session.Start(); err != nil {
			r.mu.Unlock()
			return TurnResult{}, err
		}
		r.sessions[key] = session
	}
	if err := session.beginUse(now); err != nil {
		r.mu.Unlock()
		for _, stale := range staleSessions {
			_ = stale.Close()
		}
		return TurnResult{}, err
	}
	r.mu.Unlock()
	for _, stale := range staleSessions {
		_ = stale.Close()
	}

	defer func() {
		session.endUse(time.Now())
	}()
	result, err := session.RunTurn(ctx, opts)
	if err != nil && session.isClosed() {
		r.mu.Lock()
		if current := r.sessions[key]; current == session {
			delete(r.sessions, key)
		}
		r.mu.Unlock()
	}
	return result, err
}

func (r *AppServerRunner) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	sessions := make([]*appServerSession, 0, len(r.sessions))
	for key, session := range r.sessions {
		sessions = append(sessions, session)
		delete(r.sessions, key)
	}
	r.mu.Unlock()

	errs := make([]error, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if err := session.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *AppServerRunner) collectReapableSessionsLocked(cutoff time.Time) []*appServerSession {
	staleSessions := make([]*appServerSession, 0)
	for key, session := range r.sessions {
		if session == nil {
			delete(r.sessions, key)
			continue
		}
		if session.isClosed() || session.shouldRecycle(cutoff) {
			staleSessions = append(staleSessions, session)
			delete(r.sessions, key)
		}
	}
	return staleSessions
}

func scopedRunnerSessionKey(projectPath, sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return projectPath
	}
	return projectPath + "|" + sessionID
}

func newAppServerSession(key, projectPath, codexHome, binary string) *appServerSession {
	return &appServerSession{
		key:         key,
		projectPath: projectPath,
		codexHome:   codexHome,
		binary:      binary,
		pending:     map[string]chan rpcEnvelope{},
	}
}

func (s *appServerSession) Start() error {
	s.mu.Lock()
	if s.started && !s.closed {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	cmd := exec.Command(s.binary, "app-server", "--listen", "stdio://")
	cmd.Dir = s.projectPath
	cmd.Env = buildCleanCodexEnv(s.codexHome)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	s.mu.Lock()
	s.cmd = cmd
	s.stdin = stdin
	s.started = true
	s.closed = false
	s.closeErr = nil
	s.nextID = 0
	s.running = false
	s.lastUsedAt = time.Now()
	s.mu.Unlock()

	go s.readLoop(stdout)
	go s.stderrLoop(stderr)
	go s.waitLoop()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var resp initializeResponse
	if err := s.request(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "codecli-channels",
			"version": "0.1.0",
		},
	}, &resp); err != nil {
		s.shutdown(err)
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		return err
	}
	return nil
}

func (s *appServerSession) RunTurn(ctx context.Context, opts TurnOptions) (TurnResult, error) {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()

	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	turn := newAppServerTurn(opts)
	s.setCurrentTurn(turn)
	defer s.clearCurrentTurn(turn)

	threadID, err := s.ensureThread(ctx, opts)
	if err != nil {
		turn.appendCombined(err.Error())
		turn.finish(err)
		return turn.snapshot()
	}
	turn.setThreadID(threadID)

	if err := s.startTurn(ctx, turn, opts, threadID); err != nil {
		turn.appendCombined(err.Error())
		turn.finish(err)
		return turn.snapshot()
	}

	select {
	case <-turn.done:
		return turn.snapshot()
	case <-ctx.Done():
		s.interruptTurn(turn)
		turn.finish(fmt.Errorf("Codex 执行超时（>%s）", opts.Timeout))
		return turn.snapshot()
	}
}

func (s *appServerSession) Close() error {
	s.shutdown(io.EOF)

	s.mu.Lock()
	stdin := s.stdin
	cmd := s.cmd
	s.stdin = nil
	s.cmd = nil
	s.mu.Unlock()

	if stdin != nil {
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return nil
}

func (s *appServerSession) ensureThread(ctx context.Context, opts TurnOptions) (string, error) {
	params := map[string]any{
		"approvalPolicy": "on-request",
	}
	if strings.TrimSpace(opts.ProjectPath) != "" {
		params["cwd"] = opts.ProjectPath
	}
	if strings.TrimSpace(opts.SandboxMode) != "" {
		params["sandbox"] = opts.SandboxMode
	}
	if strings.TrimSpace(opts.Model) != "" {
		params["model"] = opts.Model
	}
	if strings.TrimSpace(opts.ThreadID) != "" {
		params["threadId"] = opts.ThreadID
		var resp threadStartLikeResponse
		if err := s.request(ctx, "thread/resume", params, &resp); err == nil && strings.TrimSpace(resp.Thread.ID) != "" {
			return strings.TrimSpace(resp.Thread.ID), nil
		}
		delete(params, "threadId")
	}
	var resp threadStartLikeResponse
	if err := s.request(ctx, "thread/start", params, &resp); err != nil {
		return "", err
	}
	threadID := strings.TrimSpace(resp.Thread.ID)
	if threadID == "" {
		return "", fmt.Errorf("app-server 未返回 threadId")
	}
	return threadID, nil
}

func (s *appServerSession) startTurn(ctx context.Context, turn *appServerTurn, opts TurnOptions, threadID string) error {
	params := map[string]any{
		"threadId": threadID,
		"input": []map[string]any{{
			"type": "text",
			"text": buildPrompt(opts),
		}},
	}
	var resp turnStartResponse
	if err := s.request(ctx, "turn/start", params, &resp); err != nil {
		return err
	}
	turn.setTurnID(strings.TrimSpace(resp.Turn.ID))
	return nil
}

func (s *appServerSession) interruptTurn(turn *appServerTurn) {
	threadID, turnID := turn.ids()
	if threadID == "" || turnID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.request(ctx, "turn/interrupt", map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
	}, nil)
}

func (s *appServerSession) request(ctx context.Context, method string, params any, out any) error {
	id := s.nextRequestID()
	key := fmt.Sprintf("%d", id)
	respCh := make(chan rpcEnvelope, 1)

	s.mu.Lock()
	if s.closed {
		err := s.closeErr
		s.mu.Unlock()
		if err == nil {
			err = fmt.Errorf("app-server 会话已关闭")
		}
		return err
	}
	s.pending[key] = respCh
	s.mu.Unlock()

	if err := s.writeJSON(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		s.mu.Lock()
		delete(s.pending, key)
		s.mu.Unlock()
		return err
	}

	select {
	case env := <-respCh:
		if env.Error != nil {
			return s.rpcErrorf(method, env.Error)
		}
		if out != nil && len(env.Result) > 0 {
			if err := json.Unmarshal(env.Result, out); err != nil {
				return err
			}
		}
		return nil
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.pending, key)
		s.mu.Unlock()
		return ctx.Err()
	}
}

func (s *appServerSession) rpcErrorf(method string, rpcErr *rpcError) error {
	if rpcErr == nil {
		return fmt.Errorf("%s 失败", method)
	}
	message := strings.TrimSpace(rpcErr.Message)
	if strings.TrimSpace(string(rpcErr.Data)) != "" && string(rpcErr.Data) != "null" {
		message = strings.TrimSpace(message + " | " + strings.TrimSpace(string(rpcErr.Data)))
	}
	if message == "" {
		message = method + " 失败"
	}
	return fmt.Errorf("%s", message)
}

func (s *appServerSession) writeJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	stdin := s.stdin
	closed := s.closed
	err = s.closeErr
	s.mu.Unlock()
	if closed || stdin == nil {
		if err == nil {
			err = fmt.Errorf("app-server 会话已关闭")
		}
		return err
	}
	_, err = stdin.Write(append(data, '\n'))
	return err
}

func (s *appServerSession) nextRequestID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	return s.nextID
}

func (s *appServerSession) readLoop(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var env rpcEnvelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			s.appendCurrentCombined(line)
			continue
		}
		s.routeEnvelope(env)
	}
	if err := scanner.Err(); err != nil {
		s.shutdown(fmt.Errorf("app-server stdout 读取失败：%w", err))
		return
	}
	s.shutdown(io.EOF)
}

func (s *appServerSession) stderrLoop(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 256*1024), 2*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		s.appendCurrentCombined(line)
	}
}

func (s *appServerSession) waitLoop() {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()
	if cmd == nil {
		return
	}
	err := cmd.Wait()
	s.shutdown(err)
}

func (s *appServerSession) routeEnvelope(env rpcEnvelope) {
	switch {
	case env.Method != "" && len(env.ID) > 0:
		s.handleServerRequest(env)
	case env.Method != "":
		s.handleNotification(env)
	case len(env.ID) > 0:
		s.deliverResponse(env)
	}
}

func (s *appServerSession) deliverResponse(env rpcEnvelope) {
	key := normalizeRPCID(env.ID)
	if key == "" {
		return
	}
	s.mu.Lock()
	ch := s.pending[key]
	delete(s.pending, key)
	s.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- env:
	default:
	}
}

func (s *appServerSession) handleServerRequest(env rpcEnvelope) {
	switch env.Method {
	case "item/commandExecution/requestApproval":
		var params commandApprovalParams
		_ = json.Unmarshal(env.Params, &params)
		decision := s.resolveApproval(ApprovalRequest{
			Title:   "Codex 原生命令审批请求",
			Reason:  firstNonEmpty(strings.TrimSpace(params.Reason), "Codex 请求执行受限命令，需要人工确认。"),
			Command: strings.TrimSpace(params.Command),
			RawText: strings.TrimSpace(string(env.Params)),
		})
		_ = s.replyRequest(env.ID, map[string]any{"decision": commandApprovalDecisionValue(decision)})
	case "item/fileChange/requestApproval":
		var params fileChangeApprovalParams
		_ = json.Unmarshal(env.Params, &params)
		command := strings.TrimSpace(params.GrantRoot)
		if command != "" {
			command = "grantRoot=" + command
		}
		decision := s.resolveApproval(ApprovalRequest{
			Title:   "Codex 原生文件变更审批请求",
			Reason:  firstNonEmpty(strings.TrimSpace(params.Reason), "Codex 请求应用文件改动，需要人工确认。"),
			Command: command,
			RawText: strings.TrimSpace(string(env.Params)),
		})
		_ = s.replyRequest(env.ID, map[string]any{"decision": fileChangeApprovalDecisionValue(decision)})
	case "execCommandApproval":
		var params legacyExecApprovalParams
		_ = json.Unmarshal(env.Params, &params)
		decision := s.resolveApproval(ApprovalRequest{
			Title:   "Codex 原生命令审批请求",
			Reason:  firstNonEmpty(strings.TrimSpace(params.Reason), "Codex 请求执行命令，需要人工确认。"),
			Command: strings.TrimSpace(strings.Join(params.Command, " ")),
			RawText: strings.TrimSpace(string(env.Params)),
		})
		_ = s.replyRequest(env.ID, map[string]any{"decision": legacyApprovalDecisionValue(decision)})
	case "applyPatchApproval":
		decision := s.resolveApproval(ApprovalRequest{
			Title:   "Codex 原生文件变更审批请求",
			Reason:  "Codex 请求应用补丁，需要人工确认。",
			RawText: strings.TrimSpace(string(env.Params)),
		})
		_ = s.replyRequest(env.ID, map[string]any{"decision": legacyApprovalDecisionValue(decision)})
	case "item/tool/requestUserInput":
		s.appendCurrentCombined("收到 request_user_input，但当前 QQ 桥接暂不支持交互式追问，已返回空答案。")
		_ = s.replyRequest(env.ID, map[string]any{"answers": map[string]any{}})
	case "mcpServer/elicitation/request":
		s.appendCurrentCombined("收到 MCP elicitation，但当前 QQ 桥接暂不支持该交互，已拒绝。")
		_ = s.replyRequest(env.ID, map[string]any{"action": "decline"})
	default:
		s.appendCurrentCombined("收到未支持的 app-server 请求：" + env.Method)
		_ = s.replyRequestError(env.ID, -32000, "unsupported request")
	}
}

func (s *appServerSession) resolveApproval(req ApprovalRequest) ApprovalDecision {
	turn := s.currentTurn()
	if turn == nil || turn.opts.ApprovalHandler == nil {
		return ApprovalDeny
	}
	decision, err := turn.opts.ApprovalHandler(req)
	if err != nil {
		turn.appendCombined(err.Error())
		return ApprovalDeny
	}
	return decision
}

func (s *appServerSession) replyRequest(id json.RawMessage, result any) error {
	return s.writeJSON(rpcSuccess{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *appServerSession) replyRequestError(id json.RawMessage, code int, message string) error {
	return s.writeJSON(rpcFailure{JSONRPC: "2.0", ID: id, Error: rpcError{Code: code, Message: message}})
}

func (s *appServerSession) handleNotification(env rpcEnvelope) {
	turn := s.currentTurn()
	if turn == nil {
		return
	}
	switch env.Method {
	case "turn/started":
		var params turnStartedNotification
		if json.Unmarshal(env.Params, &params) == nil {
			if strings.TrimSpace(params.ThreadID) != "" {
				turn.setThreadID(strings.TrimSpace(params.ThreadID))
			}
			if strings.TrimSpace(params.Turn.ID) != "" {
				turn.setTurnID(strings.TrimSpace(params.Turn.ID))
			}
		}
	case "item/completed":
		var params itemNotification
		if json.Unmarshal(env.Params, &params) == nil && s.matchesTurn(turn, params.ThreadID, params.TurnID) {
			handleCompletedItem(turn, params.Item)
		}
	case "turn/completed":
		var params turnCompletedNotification
		if json.Unmarshal(env.Params, &params) == nil && s.matchesTurn(turn, params.ThreadID, params.Turn.ID) {
			if strings.TrimSpace(params.ThreadID) != "" {
				turn.setThreadID(strings.TrimSpace(params.ThreadID))
			}
			if strings.TrimSpace(params.Turn.ID) != "" {
				turn.setTurnID(strings.TrimSpace(params.Turn.ID))
			}
			if params.Turn.Error != nil {
				turn.appendCombined(joinNonEmpty(params.Turn.Error.Message, stringValue(params.Turn.Error.AdditionalDetails), " | "))
			}
			switch params.Turn.Status {
			case "completed":
				turn.finish(nil)
			case "failed":
				turn.finish(fmt.Errorf("Codex 执行失败：%s", firstNonEmpty(errorMessage(params.Turn.Error), "未知错误")))
			case "interrupted":
				turn.finish(fmt.Errorf("Codex 执行已中断"))
			default:
				turn.finish(nil)
			}
		}
	case "error":
		var params errorNotification
		if json.Unmarshal(env.Params, &params) == nil && s.matchesTurn(turn, params.ThreadID, params.TurnID) {
			turn.appendCombined(joinNonEmpty(params.Error.Message, stringValue(params.Error.AdditionalDetails), " | "))
		}
	default:
		if strings.HasPrefix(env.Method, "codex/event/") {
			handleCodexEvent(turn, env.Params)
		}
	}
}

func handleCompletedItem(turn *appServerTurn, raw json.RawMessage) {
	var head struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &head) != nil {
		return
	}
	switch head.Type {
	case "agentMessage":
		var item agentMessageItem
		if json.Unmarshal(raw, &item) == nil {
			turn.setResponseText(item.Text, item.Phase)
		}
	case "commandExecution":
		var item commandExecutionItem
		if json.Unmarshal(raw, &item) == nil {
			if item.ExitCode != nil {
				turn.setExitCode(*item.ExitCode)
			}
			if (item.Status == "failed" || item.Status == "declined") && item.AggregatedOutput != nil {
				turn.appendCombined(strings.TrimSpace(*item.AggregatedOutput))
			}
		}
	case "fileChange":
		var item fileChangeItem
		if json.Unmarshal(raw, &item) == nil && item.Status == "failed" {
			turn.appendCombined("文件变更应用失败")
		}
	}
}

func handleCodexEvent(turn *appServerTurn, raw json.RawMessage) {
	var params codexEventNotification
	if json.Unmarshal(raw, &params) != nil {
		return
	}
	if params.Msg.Type == "task_complete" && params.Msg.LastAgentMessage != nil {
		turn.setResponseText(*params.Msg.LastAgentMessage, "final_answer")
	}
	message := strings.TrimSpace(params.Msg.Message)
	if params.Msg.Type == "stream_error" || params.Msg.Type == "error" {
		turn.appendCombined(joinNonEmpty(message, strings.TrimSpace(params.Msg.AdditionalDetails), " | "))
	}
}

func (s *appServerSession) matchesTurn(turn *appServerTurn, threadID, turnID string) bool {
	currentThreadID, currentTurnID := turn.ids()
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	if currentThreadID != "" && threadID != "" && currentThreadID != threadID {
		return false
	}
	if currentTurnID != "" && turnID != "" && currentTurnID != turnID {
		return false
	}
	if currentThreadID == "" && threadID != "" {
		turn.setThreadID(threadID)
	}
	if currentTurnID == "" && turnID != "" {
		turn.setTurnID(turnID)
	}
	return true
}

func (s *appServerSession) setCurrentTurn(turn *appServerTurn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = turn
}

func (s *appServerSession) clearCurrentTurn(turn *appServerTurn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == turn {
		s.current = nil
	}
}

func (s *appServerSession) currentTurn() *appServerTurn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *appServerSession) appendCurrentCombined(text string) {
	turn := s.currentTurn()
	if turn == nil {
		return
	}
	turn.appendCombined(text)
}

func (s *appServerSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *appServerSession) beginUse(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		if s.closeErr != nil {
			return s.closeErr
		}
		return fmt.Errorf("app-server 会话已关闭")
	}
	s.running = true
	s.lastUsedAt = now
	return nil
}

func (s *appServerSession) endUse(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.lastUsedAt = now
}

func (s *appServerSession) shouldRecycle(cutoff time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.running || s.lastUsedAt.IsZero() {
		return false
	}
	return s.lastUsedAt.Before(cutoff)
}

func (s *appServerSession) shutdown(err error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.closeErr = err
	s.running = false
	pending := s.pending
	s.pending = map[string]chan rpcEnvelope{}
	current := s.current
	s.mu.Unlock()

	for _, ch := range pending {
		select {
		case ch <- rpcEnvelope{Error: &rpcError{Code: -32000, Message: firstNonEmpty(errorString(err), "app-server 会话已关闭")}}:
		default:
		}
	}
	if current != nil {
		current.finish(firstNonEmptyError(current.currentError(), err))
	}
}

func normalizeRPCID(raw json.RawMessage) string {
	value := strings.TrimSpace(string(raw))
	value = strings.Trim(value, "\"")
	return value
}

func newAppServerTurn(opts TurnOptions) *appServerTurn {
	return &appServerTurn{opts: opts, done: make(chan struct{})}
}

func (t *appServerTurn) ids() (string, string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.threadID, t.turnID
}

func (t *appServerTurn) setThreadID(value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.threadID = strings.TrimSpace(value)
}

func (t *appServerTurn) setTurnID(value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.turnID = strings.TrimSpace(value)
}

func (t *appServerTurn) setResponseText(text, phase string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	var handler func(ProgressEvent)
	shouldEmit := false
	t.mu.Lock()
	if phase == "final_answer" || t.responseText == "" {
		t.responseText = text
	}
	if phase == "final_answer" && t.opts.ProgressHandler != nil && t.lastProgressText != text {
		t.lastProgressText = text
		handler = t.opts.ProgressHandler
		shouldEmit = true
	}
	t.mu.Unlock()
	if shouldEmit && handler != nil {
		handler(ProgressEvent{Text: text})
	}
}

func (t *appServerTurn) setExitCode(code int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.exitCode = code
}

func (t *appServerTurn) appendCombined(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	parts := strings.Split(strings.ReplaceAll(text, "\r", "\n"), "\n")
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		t.combined = append(t.combined, part)
		if len(t.combined) > 200 {
			t.combined = append([]string(nil), t.combined[len(t.combined)-200:]...)
		}
	}
}

func (t *appServerTurn) finish(err error) {
	t.mu.Lock()
	if err != nil && t.err == nil {
		t.err = err
	}
	t.mu.Unlock()
	t.once.Do(func() { close(t.done) })
}

func (t *appServerTurn) currentError() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}

func (t *appServerTurn) snapshot() (TurnResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return TurnResult{
		ThreadID:     strings.TrimSpace(t.threadID),
		ResponseText: strings.TrimSpace(t.responseText),
		CombinedText: strings.TrimSpace(strings.Join(t.combined, "\n")),
		ExitCode:     t.exitCode,
	}, t.err
}

func commandApprovalDecisionValue(decision ApprovalDecision) any {
	switch decision {
	case ApprovalAllowForSession:
		return "acceptForSession"
	case ApprovalCancel:
		return "cancel"
	case ApprovalDeny:
		return "decline"
	default:
		return "accept"
	}
}

func fileChangeApprovalDecisionValue(decision ApprovalDecision) any {
	switch decision {
	case ApprovalAllowForSession:
		return "acceptForSession"
	case ApprovalCancel:
		return "cancel"
	case ApprovalDeny:
		return "decline"
	default:
		return "accept"
	}
}

func legacyApprovalDecisionValue(decision ApprovalDecision) any {
	switch decision {
	case ApprovalAllowForSession:
		return "approved_for_session"
	case ApprovalCancel:
		return "abort"
	case ApprovalDeny:
		return "denied"
	default:
		return "approved"
	}
}

func joinNonEmpty(left, right, sep string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	switch {
	case left == "":
		return right
	case right == "":
		return left
	default:
		return left + sep + right
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func errorMessage(err *struct {
	Message           string  `json:"message"`
	AdditionalDetails *string `json:"additionalDetails"`
}) string {
	if err == nil {
		return ""
	}
	return joinNonEmpty(err.Message, stringValue(err.AdditionalDetails), " | ")
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func firstNonEmptyError(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
