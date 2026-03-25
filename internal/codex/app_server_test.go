package codex

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCommandApprovalDecisionValue(t *testing.T) {
	if got := commandApprovalDecisionValue(ApprovalAllowForSession); got != "acceptForSession" {
		t.Fatalf("unexpected session decision: %v", got)
	}
	if got := commandApprovalDecisionValue(ApprovalDeny); got != "decline" {
		t.Fatalf("unexpected deny decision: %v", got)
	}
}

func TestLegacyApprovalDecisionValue(t *testing.T) {
	if got := legacyApprovalDecisionValue(ApprovalAllow); got != "approved" {
		t.Fatalf("unexpected allow decision: %v", got)
	}
	if got := legacyApprovalDecisionValue(ApprovalAllowForSession); got != "approved_for_session" {
		t.Fatalf("unexpected session decision: %v", got)
	}
}

func TestScopedRunnerSessionKey(t *testing.T) {
	path := filepath.Join("/tmp", "project")
	if got := scopedRunnerSessionKey(path, "s1"); got != path+"|s1" {
		t.Fatalf("unexpected scoped key: %s", got)
	}
	if got := scopedRunnerSessionKey(path, ""); got != path {
		t.Fatalf("unexpected default key: %s", got)
	}
}

func TestAppServerTurnEmitsFinalAnswerProgress(t *testing.T) {
	got := make([]string, 0, 2)
	turn := newAppServerTurn(TurnOptions{ProgressHandler: func(event ProgressEvent) {
		got = append(got, event.Text)
	}})
	turn.setResponseText("第一版", "final_answer")
	turn.setResponseText("第一版", "final_answer")
	turn.setResponseText("最终版", "final_answer")
	if len(got) != 2 {
		t.Fatalf("expected 2 progress events, got %d", len(got))
	}
	if got[0] != "第一版" || got[1] != "最终版" {
		t.Fatalf("unexpected progress events: %#v", got)
	}
}

func TestAppServerRunnerCollectReapableSessionsLocked(t *testing.T) {
	now := time.Now()
	oldSession := newAppServerSession("old", "/tmp/project", "/tmp/codex-home-old", "codex")
	oldSession.lastUsedAt = now.Add(-appServerSessionIdleTTL - time.Minute)
	freshSession := newAppServerSession("fresh", "/tmp/project", "/tmp/codex-home-fresh", "codex")
	freshSession.lastUsedAt = now
	runningSession := newAppServerSession("running", "/tmp/project", "/tmp/codex-home-running", "codex")
	runningSession.lastUsedAt = now.Add(-appServerSessionIdleTTL - time.Minute)
	runningSession.running = true
	closedSession := newAppServerSession("closed", "/tmp/project", "/tmp/codex-home-closed", "codex")
	closedSession.closed = true

	runner := &AppServerRunner{
		sessions: map[string]*appServerSession{
			"old":     oldSession,
			"fresh":   freshSession,
			"running": runningSession,
			"closed":  closedSession,
		},
	}

	stale := runner.collectReapableSessionsLocked(now.Add(-appServerSessionIdleTTL))
	if len(stale) != 2 {
		t.Fatalf("expected 2 stale sessions, got %d", len(stale))
	}
	if len(runner.sessions) != 2 {
		t.Fatalf("expected 2 live sessions left, got %d", len(runner.sessions))
	}
	if runner.sessions["fresh"] == nil || runner.sessions["running"] == nil {
		t.Fatalf("expected fresh and running sessions to remain: %#v", runner.sessions)
	}
}

func TestAppServerRunnerCloseClosesSessions(t *testing.T) {
	runner := NewAppServerRunner()
	session := newAppServerSession("s1", "/tmp/project", "/tmp/codex-home", "codex")
	session.pending["1"] = make(chan rpcEnvelope, 1)
	turn := newAppServerTurn(TurnOptions{})
	session.current = turn
	runner.sessions["s1"] = session

	if err := runner.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if !session.isClosed() {
		t.Fatal("expected session to be closed")
	}
	if len(runner.sessions) != 0 {
		t.Fatalf("expected sessions map to be cleared, got %d", len(runner.sessions))
	}
	select {
	case <-turn.done:
	default:
		t.Fatal("expected current turn to be finished")
	}
}
