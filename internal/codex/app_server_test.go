package codex

import (
	"path/filepath"
	"testing"
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
