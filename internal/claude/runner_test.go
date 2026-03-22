package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codecli-channels/internal/codex"
)

func TestBuildPromptRespectsMaxPromptChars(t *testing.T) {
	got := buildPrompt(codex.TurnOptions{Prompt: "你好世界", MaxPromptChars: 2})
	if got != "你好" {
		t.Fatalf("unexpected prompt length result: %q", got)
	}
}

func TestAllowedToolsForMode(t *testing.T) {
	if got := allowedToolsForMode("read"); got != "Read,Glob,Grep" {
		t.Fatalf("unexpected read tools: %q", got)
	}
	if got := allowedToolsForMode("write"); !strings.Contains(got, "Bash") || !strings.Contains(got, "Edit") {
		t.Fatalf("unexpected write tools: %q", got)
	}
}

func TestWriteBridgeSettings(t *testing.T) {
	dir := t.TempDir()
	path, err := writeBridgeSettings(dir, "write")
	if err != nil {
		t.Fatalf("write settings: %v", err)
	}
	if path == "" {
		t.Fatalf("expected settings path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse settings json: %v", err)
	}
	permissions, ok := parsed["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("expected permissions object, got %#v", parsed["permissions"])
	}
	deny, ok := permissions["deny"].([]any)
	if !ok || len(deny) == 0 {
		t.Fatalf("expected deny rules, got %#v", permissions["deny"])
	}
}

func TestPrepareScopedClaudeConfigDirCopiesFiles(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "settings.json"), []byte(`{"model":"sonnet"}`), 0o644); err != nil {
		t.Fatalf("write source settings: %v", err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", source)
	runtimeDir, err := prepareScopedClaudeConfigDir(filepath.Join(source, "project"), "s1")
	if err != nil {
		t.Fatalf("prepare runtime dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, "settings.json")); err != nil {
		t.Fatalf("expected copied settings.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, "sessions")); err != nil {
		t.Fatalf("expected sessions dir: %v", err)
	}
}
