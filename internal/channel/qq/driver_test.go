package qq

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	cfgpkg "codecli-channels/internal/config"
)

func TestToChannelMessageBuildsScopeAndReplyRef(t *testing.T) {
	msg := toChannelMessage("default", IncomingMessage{
		ChannelID: "default",
		ChatType:  "group",
		TargetID:  "group-1",
		SenderID:  "user-1",
		MessageID: "m-1",
		Text:      "/ping",
		Timestamp: "2026-03-22T12:00:00+08:00",
	})
	if msg.ChannelID != "default" {
		t.Fatalf("unexpected channel id: %s", msg.ChannelID)
	}
	if msg.Scope.Key != "default:group:group-1" {
		t.Fatalf("unexpected scope key: %s", msg.Scope.Key)
	}
	if msg.Scope.Kind != "group" {
		t.Fatalf("unexpected scope kind: %s", msg.Scope.Kind)
	}
	reply, ok := msg.ReplyRef.(replyRef)
	if !ok {
		t.Fatalf("expected qq reply ref, got %#v", msg.ReplyRef)
	}
	if reply.TargetType != "group" || reply.TargetID != "group-1" || reply.MessageID != "m-1" {
		t.Fatalf("unexpected reply ref: %#v", reply)
	}
}

func TestAPIClientResolveCredentialsUsesChannelOptions(t *testing.T) {
	cfg := &cfgpkg.Config{
		Channels: map[string]cfgpkg.ChannelConfig{
			"default": {
				Alias:   "default",
				Type:    "qq",
				Enabled: true,
				Options: map[string]any{
					"appId":        "app-id",
					"clientSecret": "secret-id",
				},
			},
		},
	}
	client := NewAPIClient(cfg)
	appID, secret, err := client.resolveCredentials("default")
	if err != nil {
		t.Fatalf("resolveCredentials error: %v", err)
	}
	if appID != "app-id" || secret != "secret-id" {
		t.Fatalf("unexpected credentials: %s %s", appID, secret)
	}
}

func TestDriverStartRequiresSink(t *testing.T) {
	driver := NewDriver("default", &cfgpkg.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := driver.Start(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "message sink") {
		t.Fatalf("expected sink validation error, got %v", err)
	}
}
