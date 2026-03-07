package bridge

import (
	"testing"

	cfgpkg "qq-codex-go/internal/config"
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
