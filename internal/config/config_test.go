package config

import "testing"

func TestNormalizeLegacyAccountsIntoChannels(t *testing.T) {
	cfg := &Config{
		DefaultAccountID: "default",
		Accounts: map[string]Account{
			"default": {Enabled: true, AppID: "app", ClientSecret: "secret"},
		},
		Bridge: BridgeConfig{
			Backend:        "codex",
			AccountIDs:     []string{"default"},
			AllowedTargets: []string{"default:user:u1"},
			Projects:       map[string]ProjectConfig{"demo": {Path: "."}},
			DefaultProject: "demo",
		},
	}
	if err := cfg.Normalize("."); err != nil {
		t.Fatalf("Normalize error: %v", err)
	}
	channel, ok := cfg.Channels["default"]
	if !ok {
		t.Fatal("expected legacy account to become channel")
	}
	if channel.Type != "qq" {
		t.Fatalf("unexpected channel type: %s", channel.Type)
	}
	if got := channel.Options["appId"]; got != "app" {
		t.Fatalf("unexpected appId: %#v", got)
	}
	if len(cfg.Bridge.ChannelIDs) != 1 || cfg.Bridge.ChannelIDs[0] != "default" {
		t.Fatalf("unexpected channel ids: %#v", cfg.Bridge.ChannelIDs)
	}
	if len(cfg.Bridge.AllowedScopes) != 1 || cfg.Bridge.AllowedScopes[0] != "default:user:u1" {
		t.Fatalf("unexpected allowed scopes: %#v", cfg.Bridge.AllowedScopes)
	}
}

func TestNormalizePreservesExplicitChannels(t *testing.T) {
	cfg := &Config{
		Channels: map[string]ChannelConfig{
			"feishu-main": {
				Type:    "feishu",
				Enabled: true,
				Options: map[string]any{"appId": "cli_xxx"},
			},
		},
		Bridge: BridgeConfig{
			Backend:        "codex",
			ChannelIDs:     []string{"feishu-main"},
			AllowedScopes:  []string{"feishu-main:p2p:chat_1"},
			Projects:       map[string]ProjectConfig{"demo": {Path: "."}},
			DefaultProject: "demo",
		},
	}
	if err := cfg.Normalize("."); err != nil {
		t.Fatalf("Normalize error: %v", err)
	}
	if len(cfg.Channels) != 1 {
		t.Fatalf("expected explicit channels to be preserved, got %d", len(cfg.Channels))
	}
	if got := cfg.Channels["feishu-main"].Type; got != "feishu" {
		t.Fatalf("unexpected explicit channel type: %s", got)
	}
	if len(cfg.Bridge.ChannelIDs) != 1 || cfg.Bridge.ChannelIDs[0] != "feishu-main" {
		t.Fatalf("unexpected channel ids: %#v", cfg.Bridge.ChannelIDs)
	}
}

func TestNormalizeMergesLegacyAccountsIntoExistingChannels(t *testing.T) {
	cfg := &Config{
		Accounts: map[string]Account{
			"default": {Enabled: true, AppID: "app", ClientSecret: "secret"},
		},
		Channels: map[string]ChannelConfig{
			"weixin-main": {
				Type:    "weixin",
				Enabled: true,
				Options: map[string]any{"token": "token"},
			},
		},
		Bridge: BridgeConfig{
			Backend:        "codex",
			ChannelIDs:     []string{"default", "weixin-main"},
			AllowedScopes:  []string{"default:user:u1", "weixin-main:dm:user@im.wechat"},
			Projects:       map[string]ProjectConfig{"demo": {Path: "."}},
			DefaultProject: "demo",
		},
	}
	if err := cfg.Normalize("."); err != nil {
		t.Fatalf("Normalize error: %v", err)
	}
	defaultChannel, ok := cfg.Channels["default"]
	if !ok {
		t.Fatal("expected legacy default account to be merged into channels")
	}
	if defaultChannel.Type != "qq" {
		t.Fatalf("expected merged legacy channel type qq, got %s", defaultChannel.Type)
	}
	if got := cfg.Channels["weixin-main"].Type; got != "weixin" {
		t.Fatalf("expected explicit channel to remain weixin, got %s", got)
	}
}

func TestNormalizeReplyCharSettings(t *testing.T) {
	t.Run("prefer maxReplyChars when present", func(t *testing.T) {
		cfg := &Config{
			DefaultAccountID: "default",
			Accounts: map[string]Account{
				"default": {Enabled: true, AppID: "app", ClientSecret: "secret"},
			},
			Bridge: BridgeConfig{
				Backend:         "codex",
				Projects:        map[string]ProjectConfig{"demo": {Path: "."}},
				DefaultProject:  "demo",
				MaxReplyChars:   1200,
				QQMaxReplyChars: 600,
				ChannelIDs:      []string{"default"},
			},
		}
		if err := cfg.Normalize("."); err != nil {
			t.Fatalf("Normalize error: %v", err)
		}
		if cfg.Bridge.MaxReplyChars != 1200 {
			t.Fatalf("expected maxReplyChars to win, got %d", cfg.Bridge.MaxReplyChars)
		}
	})

	t.Run("fallback to qqMaxReplyChars for compatibility", func(t *testing.T) {
		cfg := &Config{
			DefaultAccountID: "default",
			Accounts: map[string]Account{
				"default": {Enabled: true, AppID: "app", ClientSecret: "secret"},
			},
			Bridge: BridgeConfig{
				Backend:         "codex",
				Projects:        map[string]ProjectConfig{"demo": {Path: "."}},
				DefaultProject:  "demo",
				QQMaxReplyChars: 900,
				ChannelIDs:      []string{"default"},
			},
		}
		if err := cfg.Normalize("."); err != nil {
			t.Fatalf("Normalize error: %v", err)
		}
		if cfg.Bridge.MaxReplyChars != 900 {
			t.Fatalf("expected compatibility fallback, got %d", cfg.Bridge.MaxReplyChars)
		}
	})

	t.Run("default when neither is set", func(t *testing.T) {
		cfg := &Config{
			DefaultAccountID: "default",
			Accounts: map[string]Account{
				"default": {Enabled: true, AppID: "app", ClientSecret: "secret"},
			},
			Bridge: BridgeConfig{
				Backend:        "codex",
				Projects:       map[string]ProjectConfig{"demo": {Path: "."}},
				DefaultProject: "demo",
				ChannelIDs:     []string{"default"},
			},
		}
		if err := cfg.Normalize("."); err != nil {
			t.Fatalf("Normalize error: %v", err)
		}
		if cfg.Bridge.MaxReplyChars != DefaultMaxReplyChars {
			t.Fatalf("expected default maxReplyChars, got %d", cfg.Bridge.MaxReplyChars)
		}
	})
}
