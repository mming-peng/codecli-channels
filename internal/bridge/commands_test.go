package bridge

import (
	"strings"
	"testing"

	cfgpkg "codecli-channels/internal/config"
)

func TestParseBridgeCommand(t *testing.T) {
	cfg := cfgpkg.BridgeConfig{RequireCommandPrefix: true}
	parsed := ParseBridgeCommand("/ask 看下项目结构", cfg)
	if parsed.Type != "execute" || parsed.Mode != "read" || parsed.Body != "看下项目结构" {
		t.Fatalf("unexpected parsed result: %+v", parsed)
	}
	parsed = ParseBridgeCommand("/run 修复这个 bug", cfg)
	if parsed.Type != "execute" || parsed.Mode != "write" {
		t.Fatalf("unexpected parsed result: %+v", parsed)
	}
	parsed = ParseBridgeCommand("/confirm", cfg)
	if parsed.Type != "confirm" {
		t.Fatalf("unexpected parsed result: %+v", parsed)
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

func TestBuildHelpTextIncludesStatusHistoryAndStop(t *testing.T) {
	cfg := cfgpkg.BridgeConfig{RequireCommandPrefix: true}
	text := BuildHelpText(cfg)
	for _, want := range []string{"/status", "/stop", "/history"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected help text to contain %s, got %q", want, text)
		}
	}
	if !strings.Contains(text, "常用操作") {
		t.Fatalf("expected scenario-oriented help text, got %q", text)
	}
}
