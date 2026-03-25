package codex

import (
	"strings"
	"testing"
)

func TestBuildPromptKeepsOnlyUserPrompt(t *testing.T) {
	got := buildPrompt(TurnOptions{
		Prompt:         "  hi  ",
		ProjectAlias:   "codecli-channels",
		ProjectPath:    "/tmp/project",
		TargetType:     "group",
		SenderID:       "u1",
		TargetID:       "t1",
		MaxPromptChars: 100,
	})
	if got != "hi" {
		t.Fatalf("unexpected prompt: %q", got)
	}
	for _, banned := range []string{"QQ 机器人", "请使用中文回答", "当前项目目录", "发送者 ID"} {
		if strings.Contains(got, banned) {
			t.Fatalf("prompt should not contain %q: %q", banned, got)
		}
	}
}

func TestBuildPromptRespectsMaxPromptChars(t *testing.T) {
	got := buildPrompt(TurnOptions{Prompt: "你好世界", MaxPromptChars: 2})
	if got != "你好" {
		t.Fatalf("unexpected prompt length result: %q", got)
	}
}

func TestBuildCleanCodexEnv(t *testing.T) {
	env := strings.Join(buildCleanCodexEnv("/tmp/qq-codex-test-home"), "\n")
	for _, required := range []string{"TERM=xterm-256color", "COLORTERM=truecolor", "CODEX_DISABLE_TELEMETRY=1"} {
		if !strings.Contains(env, required) {
			t.Fatalf("expected env to contain %q", required)
		}
	}
}
