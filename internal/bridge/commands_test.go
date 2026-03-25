package bridge

import (
	"strings"
	"testing"

	cfgpkg "codecli-channels/internal/config"
)

func TestMatchRemovedCommand(t *testing.T) {
	command, reply, ok := MatchRemovedCommand("/ask 看下项目结构")
	if !ok || command != "/ask" {
		t.Fatalf("expected /ask to be matched, got ok=%v command=%q", ok, command)
	}
	if !strings.Contains(reply, "直接发普通消息") {
		t.Fatalf("expected migration hint, got %q", reply)
	}

	command, reply, ok = MatchRemovedCommand("/confirm")
	if !ok || command != "/confirm" {
		t.Fatalf("expected /confirm to be matched, got ok=%v command=%q", ok, command)
	}
	if !strings.Contains(reply, "/approve") {
		t.Fatalf("expected approve hint, got %q", reply)
	}
}

func TestDetectDangerousTask(t *testing.T) {
	matched := DetectDangerousTask("请执行 rm -rf build 目录")
	if !matched.Matched {
		t.Fatal("expected dangerous task")
	}
	matched = DetectDangerousTask("帮我看看 README")
	if matched.Matched {
		t.Fatal("did not expect dangerous task")
	}
}

func TestBuildHelpTextMatchesReducedCommandSurface(t *testing.T) {
	cfg := cfgpkg.BridgeConfig{ImplicitMessageMode: "write"}
	text := BuildHelpText(cfg)

	for _, want := range []string{
		"普通消息 - 直接发给当前后端",
		"/help - 查看帮助",
		"/history - 查看当前项目最近任务",
		"/stop - 停止当前正在执行的任务",
		"/project use <别名> - 切换项目",
		"/session new [名称] - 新建会话",
		"/backend use <codex|claude> - 切换后端",
		"/approve [session] - 同意当前待处理审批",
		"/deny - 拒绝当前待处理审批",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected help text to contain %q, got %q", want, text)
		}
	}

	for _, unwanted := range []string{"/ping", "/status", "/ask", "/run", "/confirm", "/mode", "/clear"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("expected help text to omit %q, got %q", unwanted, text)
		}
	}
}
