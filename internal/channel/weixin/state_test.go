package weixin

import (
	"path/filepath"
	"testing"
)

func TestStatePersistsCursorAndContextToken(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "weixin-main")
	state, err := NewState(dir)
	if err != nil {
		t.Fatalf("NewState error: %v", err)
	}
	if err := state.SaveCursor("cursor-1"); err != nil {
		t.Fatalf("SaveCursor error: %v", err)
	}
	if err := state.SaveContextToken("user@im.wechat", "ctx-1"); err != nil {
		t.Fatalf("SaveContextToken error: %v", err)
	}

	reloaded, err := NewState(dir)
	if err != nil {
		t.Fatalf("NewState reload error: %v", err)
	}
	cursor, err := reloaded.LoadCursor()
	if err != nil {
		t.Fatalf("LoadCursor error: %v", err)
	}
	if cursor != "cursor-1" {
		t.Fatalf("unexpected cursor: %s", cursor)
	}
	token, ok, err := reloaded.LoadContextToken("user@im.wechat")
	if err != nil {
		t.Fatalf("LoadContextToken error: %v", err)
	}
	if !ok || token != "ctx-1" {
		t.Fatalf("unexpected token: ok=%v token=%s", ok, token)
	}
}
