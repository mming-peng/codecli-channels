package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreSessionLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	conversationKey := ConversationKey("default", "user", "u1")
	if alias := store.GetProjectAlias(conversationKey, "demo"); alias != "demo" {
		t.Fatalf("unexpected project alias: %s", alias)
	}
	session, err := store.GetOrCreateActiveSession(conversationKey, "demo", "/tmp/demo", "write")
	if err != nil {
		t.Fatalf("GetOrCreateActiveSession error: %v", err)
	}
	if session.ID == "" {
		t.Fatal("expected session id")
	}
	created, err := store.CreateSession(conversationKey, "demo", "/tmp/demo", "feature", "danger")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if created.Name != "feature" {
		t.Fatalf("unexpected name: %s", created.Name)
	}
	listed := store.ListSessions(conversationKey, "demo")
	if len(listed) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(listed))
	}
	switched, err := store.SwitchSession(conversationKey, "demo", created.ID)
	if err != nil {
		t.Fatalf("SwitchSession error: %v", err)
	}
	if switched.ID != created.ID {
		t.Fatalf("unexpected switched session: %s", switched.ID)
	}
}

func TestStoreBackendPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	conversationKey := ConversationKey("default", "user", "u1")
	if got := store.GetBackend(conversationKey, "codex"); got != "codex" {
		t.Fatalf("unexpected backend default: %s", got)
	}
	if err := store.SetBackend(conversationKey, "claude"); err != nil {
		t.Fatalf("SetBackend error: %v", err)
	}
	if got := store.GetBackend(conversationKey, "codex"); got != "claude" {
		t.Fatalf("unexpected backend after set: %s", got)
	}
	reloaded, err := New(path)
	if err != nil {
		t.Fatalf("New reload error: %v", err)
	}
	if got := reloaded.GetBackend(conversationKey, "codex"); got != "claude" {
		t.Fatalf("unexpected backend after reload: %s", got)
	}
}

func TestUpdateSessionStoresClaudeSessionID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	conversationKey := ConversationKey("default", "user", "u1")
	session, err := store.GetOrCreateActiveSession(conversationKey, "demo", "/tmp/demo", "write")
	if err != nil {
		t.Fatalf("GetOrCreateActiveSession error: %v", err)
	}
	session.ClaudeSessionID = "session-1"
	if err := store.UpdateSession(session); err != nil {
		t.Fatalf("UpdateSession error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	if string(data) == "" || !strings.Contains(string(data), "claudeSessionId") {
		t.Fatalf("expected claudeSessionId in state json, got: %s", string(data))
	}
}

func TestStoreTaskSummaryPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	conversationKey := ConversationKey("default", "user", "u1")
	session, err := store.GetOrCreateActiveSession(conversationKey, "demo", "/tmp/demo", "write")
	if err != nil {
		t.Fatalf("GetOrCreateActiveSession error: %v", err)
	}
	at := time.Date(2026, 3, 22, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))
	err = store.UpdateSessionTaskSummary(session.ID, SessionTaskSummary{
		At:      at,
		Backend: "codex",
		Mode:    "write",
		Status:  "success",
		Summary: "修复状态面板",
	})
	if err != nil {
		t.Fatalf("UpdateSessionTaskSummary error: %v", err)
	}

	reloaded, err := New(path)
	if err != nil {
		t.Fatalf("reload New error: %v", err)
	}
	loaded, err := reloaded.GetOrCreateActiveSession(conversationKey, "demo", "/tmp/demo", "write")
	if err != nil {
		t.Fatalf("GetOrCreateActiveSession reload error: %v", err)
	}
	if loaded.LastBackend != "codex" {
		t.Fatalf("unexpected backend: %s", loaded.LastBackend)
	}
	if loaded.LastTaskMode != "write" {
		t.Fatalf("unexpected mode: %s", loaded.LastTaskMode)
	}
	if loaded.LastTaskStatus != "success" {
		t.Fatalf("unexpected status: %s", loaded.LastTaskStatus)
	}
	if loaded.LastTaskSummary != "修复状态面板" {
		t.Fatalf("unexpected summary: %s", loaded.LastTaskSummary)
	}
	if !loaded.LastTaskAt.Equal(at) {
		t.Fatalf("unexpected task time: %v", loaded.LastTaskAt)
	}
}
