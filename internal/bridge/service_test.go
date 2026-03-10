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
