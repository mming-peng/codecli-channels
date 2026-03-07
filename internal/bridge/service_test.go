package bridge

import (
	"strings"
	"testing"
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
