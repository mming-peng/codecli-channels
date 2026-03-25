package bridge

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"codecli-channels/internal/channel"
	"codecli-channels/internal/codex"
	cfgpkg "codecli-channels/internal/config"
	"codecli-channels/internal/store"
)

type fakeChannelDriver struct {
	id       string
	platform string
	replies  []string
	sends    []string
}

type fakeTurnRunner struct {
	mu       sync.Mutex
	calls    int
	lastOpts codex.TurnOptions
	result   codex.TurnResult
	err      error
	calledCh chan codex.TurnOptions
}

func (d *fakeChannelDriver) ID() string { return d.id }

func (d *fakeChannelDriver) Platform() string { return d.platform }

func (d *fakeChannelDriver) Start(context.Context, channel.MessageSink) error { return nil }

func (d *fakeChannelDriver) Reply(_ context.Context, _ any, content string) error {
	d.replies = append(d.replies, content)
	return nil
}

func (d *fakeChannelDriver) Send(_ context.Context, _ any, content string) error {
	d.sends = append(d.sends, content)
	return nil
}

func (d *fakeChannelDriver) Stop(context.Context) error { return nil }

func (r *fakeTurnRunner) RunTurn(_ context.Context, opts codex.TurnOptions) (codex.TurnResult, error) {
	r.mu.Lock()
	r.calls++
	r.lastOpts = opts
	r.mu.Unlock()
	if r.calledCh != nil {
		select {
		case r.calledCh <- opts:
		default:
		}
	}
	return r.result, r.err
}

func (r *fakeTurnRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestExtractFailureSummary(t *testing.T) {
	input := strings.Join([]string{
		"thinking",
		"**Considering a Chinese greeting**",
		"operation not permitted",
		"需要审批后重试",
		"普通说明",
	}, "\n")
	got := extractFailureSummary(input, "hi")
	if strings.Contains(strings.ToLower(got), "considering") || strings.Contains(strings.ToLower(got), "thinking") {
		t.Fatalf("unexpected thinking leak: %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "operation not permitted") {
		t.Fatalf("expected permission detail, got %q", got)
	}
}

func TestLooksLikeNeverPolicyResponse(t *testing.T) {
	cases := []string{
		"approval policy is never",
		"approval_policy = never",
		"当前运行策略是 `approval_policy = never`",
	}
	for _, item := range cases {
		if !looksLikeNeverPolicyResponse(item) {
			t.Fatalf("expected match for %q", item)
		}
	}
}

func TestParseApprovalCommandDecision(t *testing.T) {
	decision, ok := parseApprovalCommandDecision("/approve")
	if !ok || decision != codex.ApprovalAllow {
		t.Fatalf("unexpected decision: ok=%v decision=%v", ok, decision)
	}
	decision, ok = parseApprovalCommandDecision("/approve session")
	if !ok || decision != codex.ApprovalAllowForSession {
		t.Fatalf("unexpected session decision: ok=%v decision=%v", ok, decision)
	}
}

func TestParseNaturalApprovalDecision(t *testing.T) {
	decision, ok := parseNaturalApprovalDecision("本会话允许")
	if !ok || decision != codex.ApprovalAllowForSession {
		t.Fatalf("unexpected decision: ok=%v decision=%v", ok, decision)
	}
	decision, ok = parseNaturalApprovalDecision("拒绝")
	if !ok || decision != codex.ApprovalDeny {
		t.Fatalf("unexpected deny decision: ok=%v decision=%v", ok, decision)
	}
}

func TestShouldRequireConfirmation(t *testing.T) {
	match := shouldRequireConfirmation("write", "请执行 rm -rf build")
	if !match.Matched {
		t.Fatal("expected dangerous write task to require confirmation")
	}
	if shouldRequireConfirmation("read", "请执行 rm -rf build").Matched {
		t.Fatal("read mode should not require confirmation")
	}
}

func TestResolveDefaultRunMode(t *testing.T) {
	if got := resolveDefaultRunMode("read", "write"); got != "read" {
		t.Fatalf("expected session mode to win, got %q", got)
	}
	if got := resolveDefaultRunMode("", "read", "write"); got != "read" {
		t.Fatalf("expected implicit mode fallback, got %q", got)
	}
	if got := resolveDefaultRunMode("", "", ""); got != "write" {
		t.Fatalf("expected write default, got %q", got)
	}
}

func TestSplitReplyTextDoesNotTruncate(t *testing.T) {
	got := splitReplyText("abcdef", 4)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}
	if strings.Join(got, "") != "abcdef" {
		t.Fatalf("unexpected chunks: %#v", got)
	}
}

func TestBuildStatusTextIncludesRuntimeAndPendingState(t *testing.T) {
	text := buildStatusText(statusView{
		ProjectAlias:         "codecli-channels",
		ProjectDescription:   "Go 版 QQ-Codex 持久会话桥接",
		Backend:              "Codex",
		SessionID:            "s2",
		SessionName:          "修复体验",
		Mode:                 "write",
		Busy:                 true,
		PendingConfirmReason: "检测到 rm -rf 风险指令",
		PendingApprovalTitle: "命令执行审批",
		LastTaskStatus:       "success",
		LastTaskSummary:      "新增 /status 命令",
	})
	for _, want := range []string{
		"当前状态",
		"项目：codecli-channels",
		"后端：Codex",
		"运行中：是",
		"高风险审批：检测到 rm -rf 风险指令",
		"原生审批：命令执行审批",
		"最近任务：success | 新增 /status 命令",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in %q", want, text)
		}
	}
}

func TestBuildProjectListTextOmitsPath(t *testing.T) {
	text := buildProjectListText([]cfgpkg.ProjectConfig{
		{Alias: "codecli-channels", Path: "/tmp/project", Description: "当前仓库"},
	}, "codecli-channels")
	if strings.Contains(text, "/tmp/project") {
		t.Fatalf("expected project list to omit path, got %q", text)
	}
	if !strings.Contains(text, "当前仓库") {
		t.Fatalf("expected description in project list, got %q", text)
	}
}

func TestBuildSessionListTextIncludesSummary(t *testing.T) {
	now := time.Date(2026, 3, 22, 11, 0, 0, 0, time.UTC)
	text := buildSessionListText("codecli-channels", []*store.SessionRecord{
		{
			ID:              "s2",
			Name:            "修复体验",
			DefaultRunMode:  "write",
			LastTaskAt:      now,
			LastTaskStatus:  "success",
			LastTaskSummary: "新增 /history 命令",
		},
	}, "s2")
	for _, want := range []string{"success", "新增 /history 命令", "write"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in %q", want, text)
		}
	}
}

func TestBuildHistoryTextShowsRecentEvents(t *testing.T) {
	text := buildHistoryText("codecli-channels", []AuditEvent{
		{
			Time:         time.Date(2026, 3, 22, 11, 5, 0, 0, time.UTC),
			Status:       "success",
			SessionID:    "s2",
			Mode:         "write",
			ProjectAlias: "codecli-channels",
			Text:         "新增 /status 命令",
		},
		{
			Time:         time.Date(2026, 3, 22, 10, 55, 0, 0, time.UTC),
			Status:       "failed",
			SessionID:    "s1",
			Mode:         "read",
			ProjectAlias: "codecli-channels",
			Text:         "分析最近日志",
		},
	})
	for _, want := range []string{"最近任务", "success", "failed", "新增 /status 命令"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in %q", want, text)
		}
	}
}

func TestStopRunningTaskCancelsCurrentTask(t *testing.T) {
	canceled := false
	service := &Service{
		runningTasks: map[string]*runningTaskState{
			"conv-1": {
				Cancel: func() { canceled = true },
			},
		},
	}
	state, ok := service.stopRunningTask("conv-1")
	if !ok || state == nil {
		t.Fatal("expected running task to be stopped")
	}
	if !canceled {
		t.Fatal("expected cancel callback to run")
	}
	if !state.StopRequested {
		t.Fatal("expected stop to mark running task")
	}
}

func TestBuildConversationStatusUsesSessionSummary(t *testing.T) {
	path := t.TempDir() + "/state.json"
	stateStore, err := store.New(path)
	if err != nil {
		t.Fatalf("store.New error: %v", err)
	}
	cfg := &cfgpkg.Config{
		Bridge: cfgpkg.BridgeConfig{
			Backend:        "codex",
			DefaultProject: "codecli-channels",
			Projects: map[string]cfgpkg.ProjectConfig{
				"codecli-channels": {Alias: "codecli-channels", Description: "当前仓库", Path: "/tmp/project"},
			},
		},
	}
	service := &Service{
		cfg:                  cfg,
		store:                stateStore,
		pendingConfirmations: map[string]PendingTask{},
		nativeApprovals:      map[string]*nativeApprovalState{},
		runningTasks:         map[string]*runningTaskState{},
	}
	key := store.ConversationKey("default", "user", "u1")
	session, err := stateStore.GetOrCreateActiveSession(key, "codecli-channels", "/tmp/project", "write")
	if err != nil {
		t.Fatalf("GetOrCreateActiveSession error: %v", err)
	}
	err = stateStore.UpdateSessionTaskSummary(session.ID, store.SessionTaskSummary{
		At:      time.Now(),
		Backend: "codex",
		Mode:    "write",
		Status:  "success",
		Summary: "完成 /status",
	})
	if err != nil {
		t.Fatalf("UpdateSessionTaskSummary error: %v", err)
	}
	text := service.buildConversationStatus(context.Background(), key)
	if !strings.Contains(text, "完成 /status") {
		t.Fatalf("expected latest task summary in %q", text)
	}
}

func TestHandleMessageUsesGenericChannelDriver(t *testing.T) {
	path := t.TempDir() + "/state.json"
	stateStore, err := store.New(path)
	if err != nil {
		t.Fatalf("store.New error: %v", err)
	}
	cfg := &cfgpkg.Config{
		Channels: map[string]cfgpkg.ChannelConfig{
			"test-main": {Alias: "test-main", Type: "fake", Enabled: true},
		},
		Bridge: cfgpkg.BridgeConfig{
			Backend:         "codex",
			ChannelIDs:      []string{"test-main"},
			AllowAllTargets: true,
			MaxReplyChars:   200,
			DefaultProject:  "codecli-channels",
			Projects: map[string]cfgpkg.ProjectConfig{
				"codecli-channels": {Alias: "codecli-channels", Description: "当前仓库", Path: "/tmp/project"},
			},
		},
	}
	driver := &fakeChannelDriver{id: "test-main", platform: "fake"}
	service := &Service{
		cfg:                  cfg,
		logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		store:                stateStore,
		drivers:              map[string]channel.Driver{"test-main": driver},
		pendingConfirmations: map[string]PendingTask{},
		nativeApprovals:      map[string]*nativeApprovalState{},
		runningTasks:         map[string]*runningTaskState{},
		busy:                 map[string]bool{},
	}
	service.handleMessage(context.Background(), channel.Message{
		ChannelID: "test-main",
		Platform:  "fake",
		Scope:     channel.ConversationScope{Key: "test-main:dm:u1", Kind: "dm"},
		Sender:    channel.Sender{ID: "u1"},
		MessageID: "m1",
		Text:      "/help",
		ReplyRef:  "reply-1",
	})
	if len(driver.replies) != 1 {
		t.Fatalf("expected one reply, got %d", len(driver.replies))
	}
	if !strings.Contains(driver.replies[0], "/history") {
		t.Fatalf("expected help reply, got %q", driver.replies[0])
	}
}

func TestHandleMessageRemovedCommandReturnsMigrationHint(t *testing.T) {
	path := t.TempDir() + "/state.json"
	stateStore, err := store.New(path)
	if err != nil {
		t.Fatalf("store.New error: %v", err)
	}
	runner := &fakeTurnRunner{}
	cfg := &cfgpkg.Config{
		Channels: map[string]cfgpkg.ChannelConfig{
			"test-main": {Alias: "test-main", Type: "fake", Enabled: true},
		},
		Bridge: cfgpkg.BridgeConfig{
			Backend:         "codex",
			ChannelIDs:      []string{"test-main"},
			AllowAllTargets: true,
			MaxReplyChars:   200,
			DefaultProject:  "codecli-channels",
			Projects: map[string]cfgpkg.ProjectConfig{
				"codecli-channels": {Alias: "codecli-channels", Description: "当前仓库", Path: "/tmp/project"},
			},
		},
	}
	driver := &fakeChannelDriver{id: "test-main", platform: "fake"}
	service := &Service{
		cfg:                  cfg,
		logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		store:                stateStore,
		drivers:              map[string]channel.Driver{"test-main": driver},
		codexRunner:          runner,
		claudeRunner:         runner,
		pendingConfirmations: map[string]PendingTask{},
		nativeApprovals:      map[string]*nativeApprovalState{},
		runningTasks:         map[string]*runningTaskState{},
		busy:                 map[string]bool{},
	}
	service.handleMessage(context.Background(), channel.Message{
		ChannelID: "test-main",
		Platform:  "fake",
		Scope:     channel.ConversationScope{Key: "test-main:dm:u1", Kind: "dm"},
		Sender:    channel.Sender{ID: "u1"},
		MessageID: "m1",
		Text:      "/ask 看下项目结构",
		ReplyRef:  "reply-1",
	})
	if len(driver.replies) != 1 {
		t.Fatalf("expected one reply, got %d", len(driver.replies))
	}
	if !strings.Contains(driver.replies[0], "直接发普通消息") {
		t.Fatalf("expected migration hint, got %q", driver.replies[0])
	}
	time.Sleep(20 * time.Millisecond)
	if runner.CallCount() != 0 {
		t.Fatalf("expected removed command not to dispatch task, got %d calls", runner.CallCount())
	}
}

func TestApproveConfirmsPendingDangerousTask(t *testing.T) {
	path := t.TempDir() + "/state.json"
	stateStore, err := store.New(path)
	if err != nil {
		t.Fatalf("store.New error: %v", err)
	}
	calledCh := make(chan codex.TurnOptions, 1)
	runner := &fakeTurnRunner{
		result:   codex.TurnResult{ResponseText: "done"},
		calledCh: calledCh,
	}
	cfg := &cfgpkg.Config{
		Channels: map[string]cfgpkg.ChannelConfig{
			"test-main": {Alias: "test-main", Type: "fake", Enabled: true},
		},
		Bridge: cfgpkg.BridgeConfig{
			Backend:           "codex",
			ChannelIDs:        []string{"test-main"},
			AllowAllTargets:   true,
			MaxReplyChars:     200,
			DefaultProject:    "codecli-channels",
			ConfirmationTTLMS: 60000,
			Projects: map[string]cfgpkg.ProjectConfig{
				"codecli-channels": {Alias: "codecli-channels", Description: "当前仓库", Path: "/tmp/project"},
			},
		},
	}
	driver := &fakeChannelDriver{id: "test-main", platform: "fake"}
	service := &Service{
		cfg:                  cfg,
		logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		store:                stateStore,
		drivers:              map[string]channel.Driver{"test-main": driver},
		codexRunner:          runner,
		claudeRunner:         runner,
		pendingConfirmations: map[string]PendingTask{},
		nativeApprovals:      map[string]*nativeApprovalState{},
		runningTasks:         map[string]*runningTaskState{},
		busy:                 map[string]bool{},
	}
	msg := channel.Message{
		ChannelID: "test-main",
		Platform:  "fake",
		Scope:     channel.ConversationScope{Key: "test-main:dm:u1", Kind: "dm"},
		Sender:    channel.Sender{ID: "u1"},
		MessageID: "m1",
		ReplyRef:  "reply-1",
	}
	service.setPendingConfirmation(conversationKeyFromMessage(msg), PendingTask{
		Mode:         "write",
		Body:         "请执行 rm -rf build",
		Reason:       "检测到 rm -rf 风险指令",
		ProjectAlias: "codecli-channels",
		ProjectPath:  "/tmp/project",
		ExpiresAt:    time.Now().Add(time.Minute),
	})

	service.handleMessage(context.Background(), channel.Message{
		ChannelID: "test-main",
		Platform:  "fake",
		Scope:     msg.Scope,
		Sender:    msg.Sender,
		MessageID: "m2",
		Text:      "/approve",
		ReplyRef:  "reply-2",
	})

	select {
	case opts := <-calledCh:
		if opts.Prompt != "请执行 rm -rf build" {
			t.Fatalf("expected pending task body to run, got %q", opts.Prompt)
		}
	case <-time.After(time.Second):
		t.Fatal("expected approve to dispatch pending dangerous task")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		service.mu.Lock()
		busy := service.busy[conversationKeyFromMessage(msg)]
		service.mu.Unlock()
		if service.currentRunningTask(conversationKeyFromMessage(msg)) == nil && !busy {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected background task to finish before test cleanup")
}
