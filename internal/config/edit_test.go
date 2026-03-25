package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRawPreservesRelativePaths(t *testing.T) {
	path := writeRawConfigFile(t, `{
  "channels": {
    "default": {
      "type": "qq",
      "enabled": true,
      "options": {
        "appId": "app",
        "clientSecret": "secret"
      }
    }
  },
  "bridge": {
    "backend": "codex",
    "channelIds": ["default"],
    "projects": {
      "demo": {
        "path": "./workspace/demo",
        "description": "demo"
      }
    },
    "defaultProject": "demo",
    "dataDir": "./data"
  }
}`)

	cfg, err := LoadRaw(path)
	if err != nil {
		t.Fatalf("LoadRaw error: %v", err)
	}
	if got := cfg.Bridge.Projects["demo"].Path; got != "./workspace/demo" {
		t.Fatalf("project path was normalized unexpectedly: %q", got)
	}
	if got := cfg.Bridge.DataDir; got != "./data" {
		t.Fatalf("dataDir was normalized unexpectedly: %q", got)
	}
}

func TestUpsertChannelCreatesAndUpdates(t *testing.T) {
	cfg := &Config{
		Bridge: BridgeConfig{
			Projects:       map[string]ProjectConfig{"demo": {Path: "."}},
			DefaultProject: "demo",
		},
	}
	enabled := true

	created, err := cfg.UpsertChannel("weixin-main", ChannelPatch{
		Type:    "weixin",
		Enabled: &enabled,
		Options: map[string]any{
			"token":   "token-1",
			"baseUrl": "https://ilink.test",
		},
		SetOptionsIfEmpty: map[string]any{
			"allowFrom": []string{"user@im.wechat"},
		},
	})
	if err != nil {
		t.Fatalf("UpsertChannel(create) error: %v", err)
	}
	if created.Type != "weixin" {
		t.Fatalf("Type = %q, want weixin", created.Type)
	}
	if !created.Enabled {
		t.Fatal("expected created channel to be enabled")
	}
	if got := created.Options["token"]; got != "token-1" {
		t.Fatalf("token = %#v, want token-1", got)
	}
	if got := created.Options["allowFrom"]; !equalStringSlice(got, []string{"user@im.wechat"}) {
		t.Fatalf("allowFrom = %#v, want preserved slice", got)
	}

	updated, err := cfg.UpsertChannel("weixin-main", ChannelPatch{
		Type: "weixin",
		Options: map[string]any{
			"token": "token-2",
		},
		SetOptionsIfEmpty: map[string]any{
			"allowFrom": []string{"new-user@im.wechat"},
		},
	})
	if err != nil {
		t.Fatalf("UpsertChannel(update) error: %v", err)
	}
	if got := updated.Options["token"]; got != "token-2" {
		t.Fatalf("token = %#v, want token-2", got)
	}
	if got := updated.Options["allowFrom"]; !equalStringSlice(got, []string{"user@im.wechat"}) {
		t.Fatalf("allowFrom should not be overwritten when already set, got %#v", got)
	}
}

func TestEnsureBridgeBindings(t *testing.T) {
	cfg := &Config{
		Bridge: BridgeConfig{
			ChannelIDs:    []string{"default"},
			AllowedScopes: []string{"default:user:openid"},
		},
	}

	cfg.EnsureBridgeChannelID("weixin-main")
	cfg.EnsureBridgeChannelID("weixin-main")
	cfg.EnsureAllowedScope("weixin-main:dm:user@im.wechat")
	cfg.EnsureAllowedScope("weixin-main:dm:user@im.wechat")

	if got := len(cfg.Bridge.ChannelIDs); got != 2 {
		t.Fatalf("ChannelIDs len = %d, want 2", got)
	}
	if cfg.Bridge.ChannelIDs[1] != "weixin-main" {
		t.Fatalf("unexpected ChannelIDs: %#v", cfg.Bridge.ChannelIDs)
	}
	if got := len(cfg.Bridge.AllowedScopes); got != 2 {
		t.Fatalf("AllowedScopes len = %d, want 2", got)
	}
	if cfg.Bridge.AllowedScopes[1] != "weixin-main:dm:user@im.wechat" {
		t.Fatalf("unexpected AllowedScopes: %#v", cfg.Bridge.AllowedScopes)
	}
}

func TestEnsureBridgeBindingsSeedsLegacyFields(t *testing.T) {
	cfg := &Config{
		Bridge: BridgeConfig{
			AccountIDs:     []string{"default", "bot2"},
			AllowedTargets: []string{"default:user:openid-1", "bot2:user:openid-2"},
		},
	}

	cfg.EnsureBridgeChannelID("weixin-main")
	cfg.EnsureAllowedScope("weixin-main:dm:user@im.wechat")

	if got := cfg.Bridge.ChannelIDs; len(got) != 3 || got[0] != "default" || got[1] != "bot2" || got[2] != "weixin-main" {
		t.Fatalf("ChannelIDs = %#v, want legacy ids plus weixin", got)
	}
	if got := cfg.Bridge.AllowedScopes; len(got) != 3 || got[0] != "default:user:openid-1" || got[1] != "bot2:user:openid-2" || got[2] != "weixin-main:dm:user@im.wechat" {
		t.Fatalf("AllowedScopes = %#v, want legacy targets plus weixin scope", got)
	}
}

func TestUpsertChannelRejectsTypeConflict(t *testing.T) {
	cfg := &Config{
		Channels: map[string]ChannelConfig{
			"weixin-main": {
				Alias:   "weixin-main",
				Type:    "qq",
				Enabled: true,
				Options: map[string]any{},
			},
		},
	}
	_, err := cfg.UpsertChannel("weixin-main", ChannelPatch{Type: "weixin"})
	if err == nil {
		t.Fatal("expected type conflict error")
	}
}

func TestSaveRawRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := &Config{
		Channels: map[string]ChannelConfig{
			"weixin-main": {
				Alias:   "weixin-main",
				Type:    "weixin",
				Enabled: true,
				Options: map[string]any{
					"token":     "token-1",
					"allowFrom": []string{"user@im.wechat"},
				},
			},
		},
		Bridge: BridgeConfig{
			Backend:        "codex",
			Projects:       map[string]ProjectConfig{"demo": {Path: "."}},
			DefaultProject: "demo",
		},
	}

	if err := SaveRaw(path, cfg); err != nil {
		t.Fatalf("SaveRaw error: %v", err)
	}
	reloaded, err := LoadRaw(path)
	if err != nil {
		t.Fatalf("LoadRaw after save error: %v", err)
	}
	channel, ok := reloaded.Channel("weixin-main")
	if !ok {
		t.Fatal("expected weixin-main channel after round trip")
	}
	if got := channel.Options["token"]; got != "token-1" {
		t.Fatalf("token = %#v, want token-1", got)
	}
}

func writeRawConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	return path
}

func equalStringSlice(value any, want []string) bool {
	items, ok := value.([]any)
	if ok {
		if len(items) != len(want) {
			return false
		}
		for idx, item := range items {
			text, ok := item.(string)
			if !ok || text != want[idx] {
				return false
			}
		}
		return true
	}
	strs, ok := value.([]string)
	if !ok || len(strs) != len(want) {
		return false
	}
	for idx := range strs {
		if strs[idx] != want[idx] {
			return false
		}
	}
	return true
}
