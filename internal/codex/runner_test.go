package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPromptKeepsOnlyUserPrompt(t *testing.T) {
	got := buildPrompt(TurnOptions{
		Prompt:         "  hi  ",
		ProjectAlias:   "qq-codex-go",
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

func TestParseSessionLineTaskComplete(t *testing.T) {
	line := `{"timestamp":"2026-03-08T02:01:06.916Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"abc","last_agent_message":"最终回复"}}`
	var update sessionWatchUpdate
	parseSessionLine(line, &update)
	if !update.TaskComplete {
		t.Fatalf("expected task complete")
	}
	if update.LastAgentText != "最终回复" {
		t.Fatalf("unexpected last agent text: %q", update.LastAgentText)
	}
}

func TestParseSessionLineAssistantMessage(t *testing.T) {
	line := `{"timestamp":"2026-03-08T02:01:06.709Z","type":"response_item","payload":{"type":"message","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":"你好"},{"type":"output_text","text":"，世界"}]}}`
	var update sessionWatchUpdate
	parseSessionLine(line, &update)
	if update.FinalAnswerText != "你好，世界" {
		t.Fatalf("unexpected final answer text: %q", update.FinalAnswerText)
	}
}

func TestParseSessionLineApprovalFunctionCall(t *testing.T) {
	line := `{"timestamp":"2026-03-08T02:47:59.421Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"touch ~/.codex/permission_probe_should_fail\",\"justification\":\"要在你的主目录下创建探测文件，这超出了当前工作区写权限。你要允许我执行吗？\",\"sandbox_permissions\":\"require_escalated\",\"max_output_tokens\":200}","call_id":"call_123"}}`
	var update sessionWatchUpdate
	parseSessionLine(line, &update)
	if len(update.ApprovalRequests) != 1 {
		t.Fatalf("expected 1 approval request, got %d", len(update.ApprovalRequests))
	}
	request := update.ApprovalRequests[0]
	if request.Command != "touch ~/.codex/permission_probe_should_fail" {
		t.Fatalf("unexpected command: %q", request.Command)
	}
	if !strings.Contains(request.Reason, "主目录") {
		t.Fatalf("unexpected reason: %q", request.Reason)
	}
}

func TestResolveRunnableThreadID(t *testing.T) {
	base := t.TempDir()
	codexHome := filepath.Join(base, ".codex")
	sessionDir := filepath.Join(codexHome, "sessions", "2026", "03", "08")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	sessionPath := filepath.Join(sessionDir, "rollout.jsonl")
	line := `{"type":"session_meta","payload":{"id":"thread-1","cwd":"/tmp/project","source":"cli"}}
`
	if err := os.WriteFile(sessionPath, []byte(line), 0o644); err != nil {
		t.Fatalf("write session failed: %v", err)
	}
	if got := resolveRunnableThreadID(codexHome, "thread-1"); got != "thread-1" {
		t.Fatalf("expected existing thread to be kept, got %q", got)
	}
	if got := resolveRunnableThreadID(codexHome, "missing-thread"); got != "" {
		t.Fatalf("expected missing thread to be cleared, got %q", got)
	}
}
