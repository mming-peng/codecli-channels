package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	cfgpkg "codecli-channels/internal/config"
	onboardingweixin "codecli-channels/internal/onboarding/weixin"
)

func TestRunWeixinSetupWritesChannelConfig(t *testing.T) {
	configPath := writeJSONConfig(t, `{
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
        "path": "."
      }
    },
    "defaultProject": "demo"
  }
}`)
	var stdout bytes.Buffer
	env := commandEnv{
		stdout: &stdout,
		stderr: &bytes.Buffer{},
		runWeixinSetup: func(context.Context, onboardingweixin.SetupOptions) (*onboardingweixin.SetupResult, error) {
			return &onboardingweixin.SetupResult{
				BotToken:    "bot-token",
				BaseURL:     "https://ilink.test",
				IlinkBotID:  "bot-id",
				IlinkUserID: "user@im.wechat",
			}, nil
		},
		verifyWeixinToken: func(context.Context, onboardingweixin.VerifyTokenOptions) error {
			t.Fatal("verify should not be called for setup")
			return nil
		},
	}

	if err := runWeixin([]string{"setup", "-config", configPath}, env); err != nil {
		t.Fatalf("runWeixin(setup) error: %v", err)
	}
	cfg, err := cfgpkg.LoadRaw(configPath)
	if err != nil {
		t.Fatalf("LoadRaw error: %v", err)
	}
	channel, ok := cfg.Channel("weixin-main")
	if !ok {
		t.Fatal("expected weixin-main channel to be created")
	}
	if channel.Type != "weixin" || !channel.Enabled {
		t.Fatalf("unexpected channel: %#v", channel)
	}
	if got := channel.Options["token"]; got != "bot-token" {
		t.Fatalf("token = %#v, want bot-token", got)
	}
	if got := channel.Options["baseUrl"]; got != "https://ilink.test" {
		t.Fatalf("baseUrl = %#v, want https://ilink.test", got)
	}
	if got := channel.Options["allowFrom"]; !equalStringSlice(got, []string{"user@im.wechat"}) {
		t.Fatalf("allowFrom = %#v, want scanned user", got)
	}
	if len(cfg.Bridge.ChannelIDs) != 2 || cfg.Bridge.ChannelIDs[1] != "weixin-main" {
		t.Fatalf("ChannelIDs = %#v, want default+weixin-main", cfg.Bridge.ChannelIDs)
	}
	if len(cfg.Bridge.AllowedScopes) != 1 || cfg.Bridge.AllowedScopes[0] != "weixin-main:dm:user@im.wechat" {
		t.Fatalf("AllowedScopes = %#v, want scanned weixin scope", cfg.Bridge.AllowedScopes)
	}
}

func TestRunWeixinBindVerifiesTokenAndPreservesAllowFrom(t *testing.T) {
	configPath := writeJSONConfig(t, `{
  "channels": {
    "weixin-main": {
      "type": "weixin",
      "enabled": false,
      "options": {
        "allowFrom": ["keep@im.wechat"]
      }
    }
  },
  "bridge": {
    "backend": "codex",
    "channelIds": ["weixin-main"],
    "projects": {
      "demo": {
        "path": "."
      }
    },
    "defaultProject": "demo"
  }
}`)
	var verifiedToken string
	env := commandEnv{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		runWeixinSetup: func(context.Context, onboardingweixin.SetupOptions) (*onboardingweixin.SetupResult, error) {
			t.Fatal("setup flow should not be called for bind")
			return nil, nil
		},
		verifyWeixinToken: func(_ context.Context, opts onboardingweixin.VerifyTokenOptions) error {
			verifiedToken = opts.Token
			return nil
		},
	}

	if err := runWeixin([]string{
		"bind",
		"-config", configPath,
		"-token", "bind-token",
		"-api-url", "https://ilink.bind",
	}, env); err != nil {
		t.Fatalf("runWeixin(bind) error: %v", err)
	}
	if verifiedToken != "bind-token" {
		t.Fatalf("verified token = %q, want bind-token", verifiedToken)
	}
	cfg, err := cfgpkg.LoadRaw(configPath)
	if err != nil {
		t.Fatalf("LoadRaw error: %v", err)
	}
	channel, ok := cfg.Channel("weixin-main")
	if !ok {
		t.Fatal("expected weixin-main channel")
	}
	if !channel.Enabled {
		t.Fatal("expected weixin-main to be enabled after bind")
	}
	if got := channel.Options["token"]; got != "bind-token" {
		t.Fatalf("token = %#v, want bind-token", got)
	}
	if got := channel.Options["allowFrom"]; !equalStringSlice(got, []string{"keep@im.wechat"}) {
		t.Fatalf("allowFrom should be preserved, got %#v", got)
	}
	if len(cfg.Bridge.ChannelIDs) != 1 || cfg.Bridge.ChannelIDs[0] != "weixin-main" {
		t.Fatalf("ChannelIDs = %#v, want existing weixin-main only", cfg.Bridge.ChannelIDs)
	}
}

func TestRunWeixinSetupPreservesLegacyQQRouting(t *testing.T) {
	configPath := writeJSONConfig(t, `{
  "defaultAccountId": "default",
  "accounts": {
    "default": {
      "enabled": true,
      "appId": "app-1",
      "clientSecret": "secret-1"
    },
    "bot2": {
      "enabled": true,
      "appId": "app-2",
      "clientSecret": "secret-2"
    }
  },
  "bridge": {
    "accountIds": ["default", "bot2"],
    "allowedTargets": [
      "default:user:openid-1",
      "bot2:user:openid-2"
    ],
    "projects": {
      "demo": {
        "path": "."
      }
    },
    "defaultProject": "demo"
  }
}`)
	env := commandEnv{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		runWeixinSetup: func(context.Context, onboardingweixin.SetupOptions) (*onboardingweixin.SetupResult, error) {
			return &onboardingweixin.SetupResult{
				BotToken:    "bot-token",
				BaseURL:     "https://ilink.test",
				IlinkBotID:  "bot-id",
				IlinkUserID: "user@im.wechat",
			}, nil
		},
		verifyWeixinToken: func(context.Context, onboardingweixin.VerifyTokenOptions) error {
			t.Fatal("verify should not be called for setup")
			return nil
		},
	}

	if err := runWeixin([]string{"setup", "-config", configPath}, env); err != nil {
		t.Fatalf("runWeixin(setup legacy) error: %v", err)
	}
	cfg, err := cfgpkg.LoadRaw(configPath)
	if err != nil {
		t.Fatalf("LoadRaw error: %v", err)
	}
	if _, ok := cfg.Channel("default"); !ok {
		t.Fatal("expected legacy default account to be materialized into channels")
	}
	if _, ok := cfg.Channel("bot2"); !ok {
		t.Fatal("expected legacy bot2 account to be materialized into channels")
	}
	if _, ok := cfg.Channel("weixin-main"); !ok {
		t.Fatal("expected weixin-main channel to be created")
	}
	if got := cfg.Bridge.ChannelIDs; len(got) != 3 || got[0] != "default" || got[1] != "bot2" || got[2] != "weixin-main" {
		t.Fatalf("ChannelIDs = %#v, want legacy QQ ids plus weixin-main", got)
	}
	if got := cfg.Bridge.AllowedScopes; len(got) != 3 || got[0] != "default:user:openid-1" || got[1] != "bot2:user:openid-2" || got[2] != "weixin-main:dm:user@im.wechat" {
		t.Fatalf("AllowedScopes = %#v, want legacy targets plus weixin scope", got)
	}
}

func writeJSONConfig(t *testing.T, content string) string {
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
