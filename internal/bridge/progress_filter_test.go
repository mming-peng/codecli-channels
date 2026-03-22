package bridge

import "testing"

func TestFilterProgressForChannel(t *testing.T) {
	input := `sandbox: workspace-write [workdir, /tmp, $TMPDIR, /Users/ming/.codex/memories]
reasoning effort: medium
你正在通过官方 QQ 机器人为本机用户提供 Codex 远程代理服务。
hi
mcp: chrome-devtools starting
嗨，我在。请直接说你要我做什么。
2026-03-07T14:54:17Z WARN codex_protocol::openai_models: xxx
45,548
none
ready
exec
/bin/zsh -lc 'find /Users/ming/ai -mindepth 1 -maxdepth 1 -type d | wc -l' in /Users/ming/ai/feishu-connect
succeeded in 52ms:`
	out := filterProgressForChannel(input, "hi")
	if out != "嗨，我在。请直接说你要我做什么。" {
		t.Fatalf("unexpected output: %q", out)
	}
}
