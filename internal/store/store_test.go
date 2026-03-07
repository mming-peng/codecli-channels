package store

import (
	"path/filepath"
	"testing"
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
