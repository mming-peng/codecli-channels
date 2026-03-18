package bridge

import (
	"strings"
	"testing"

	"qq-codex-go/internal/codex"
)

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

func TestSplitForQQDoesNotTruncate(t *testing.T) {
	got := splitForQQ("abcdef", 4)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}
	if strings.Join(got, "") != "abcdef" {
		t.Fatalf("unexpected chunks: %#v", got)
	}
}
