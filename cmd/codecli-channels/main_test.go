package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfigPath(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd error: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir error: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	t.Run("prefer new default config", func(t *testing.T) {
		newPath := filepath.Join(configDir, "codecli-channels.json")
		if err := os.WriteFile(newPath, []byte("{}"), 0o644); err != nil {
			t.Fatalf("WriteFile error: %v", err)
		}
		got := resolveConfigPath("config/codecli-channels.json")
		if got != "config/codecli-channels.json" {
			t.Fatalf("unexpected path: %s", got)
		}
		_ = os.Remove(newPath)
	})

	t.Run("fallback to legacy config", func(t *testing.T) {
		legacyPath := filepath.Join(configDir, "qqbot.json")
		if err := os.WriteFile(legacyPath, []byte("{}"), 0o644); err != nil {
			t.Fatalf("WriteFile error: %v", err)
		}
		got := resolveConfigPath("config/codecli-channels.json")
		if got != "config/qqbot.json" {
			t.Fatalf("expected legacy fallback, got %s", got)
		}
	})
}
