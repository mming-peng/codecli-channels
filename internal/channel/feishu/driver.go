package feishu

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"codecli-channels/internal/channel"
	cfgpkg "codecli-channels/internal/config"
)

type Driver struct {
	id        string
	cfg       *cfgpkg.Config
	api       textAPI
	gateway   *Gateway
	botOpenID string
	options   options
	logger    *slog.Logger
}

func Register(registry *channel.Registry, cfg *cfgpkg.Config, logger *slog.Logger) {
	if registry == nil {
		return
	}
	registry.Register("feishu", func(id string, channelCfg cfgpkg.ChannelConfig, dataDir string) (channel.Driver, error) {
		_ = channelCfg
		_ = dataDir
		return NewDriver(id, cfg, logger), nil
	})
}

func NewDriver(id string, cfg *cfgpkg.Config, logger *slog.Logger) *Driver {
	if logger == nil {
		logger = slog.Default()
	}
	return &Driver{
		id:     id,
		cfg:    cfg,
		api:    newChannelAPI(id, NewAPIClient(cfg)),
		logger: logger.With("channelId", id, "platform", "feishu"),
	}
}

func (d *Driver) ID() string { return d.id }

func (d *Driver) Platform() string { return "feishu" }

func (d *Driver) Start(ctx context.Context, sink channel.MessageSink) error {
	if sink == nil {
		return fmt.Errorf("feishu driver 需要 message sink")
	}
	driverCfg, err := resolveDriverConfig(d.cfg, d.id)
	if err != nil {
		return err
	}
	d.options = options(driverCfg)
	if d.botOpenID == "" && d.api != nil {
		botOpenID, err := d.api.FetchBotOpenID(ctx)
		if err == nil {
			d.botOpenID = botOpenID
		}
	}
	d.gateway = NewGateway(driverCfg, d.logger, func(ctx context.Context, msg incomingMessage) {
		channelMsg, ok := d.toChannelMessage(msg)
		if ok {
			sink(ctx, channelMsg)
		}
	})
	return d.gateway.Start(ctx)
}

func (d *Driver) Reply(ctx context.Context, ref any, content string) error {
	reply, err := parseReplyRef(ref)
	if err != nil {
		return err
	}
	return d.api.ReplyText(ctx, reply, content)
}

func (d *Driver) Send(ctx context.Context, ref any, content string) error {
	reply, err := parseReplyRef(ref)
	if err != nil {
		return err
	}
	if reply.MessageID != "" {
		return d.api.ReplyText(ctx, reply, content)
	}
	if reply.ChatID == "" {
		return fmt.Errorf("feishu driver 缺少 chat_id，无法主动发送")
	}
	return d.api.CreateText(ctx, reply.ChatID, content)
}

func (d *Driver) Stop(context.Context) error { return nil }

func (d *Driver) toChannelMessage(msg incomingMessage) (channel.Message, bool) {
	if msg.ChatType != chatTypeP2P && !d.options.GroupReplyAll && !mentionsBot(msg.Mentions, d.botOpenID) {
		return channel.Message{}, false
	}

	msg.Text = strings.TrimSpace(stripMentions(msg.Text, msg.Mentions, d.botOpenID))
	if msg.Text == "" {
		return channel.Message{}, false
	}

	return toChannelMessage(d.id, driverConfig(d.options), msg), true
}

func toChannelMessage(channelID string, cfg driverConfig, msg incomingMessage) channel.Message {
	scope, rootID := scopeForMessage(channelID, cfg, msg)
	senderID := msg.SenderID()
	return channel.Message{
		ChannelID: channelID,
		Platform:  "feishu",
		Scope:     scope,
		Sender: channel.Sender{
			ID: senderID,
		},
		MessageID: msg.MessageID,
		Text:      msg.Text,
		Timestamp: msg.Timestamp,
		ReplyRef: replyRef{
			MessageID:     msg.MessageID,
			ChatID:        msg.ChatID,
			ScopeKey:      scope.Key,
			RootID:        rootID,
			ReplyInThread: rootID != "",
		},
		Metadata: map[string]string{
			"chatId":   msg.ChatID,
			"chatType": msg.ChatType,
			"targetId": msg.ChatID,
			"senderId": senderID,
			"rootId":   rootID,
		},
	}
}

func parseReplyRef(ref any) (replyRef, error) {
	switch value := ref.(type) {
	case replyRef:
		return value, nil
	case *replyRef:
		if value == nil {
			return replyRef{}, fmt.Errorf("feishu driver 收到空 reply ref")
		}
		return *value, nil
	default:
		return replyRef{}, fmt.Errorf("feishu driver 收到未知 reply ref")
	}
}

func scopeForMessage(channelID string, cfg driverConfig, msg incomingMessage) (channel.ConversationScope, string) {
	if msg.ChatType == chatTypeP2P {
		return channel.ConversationScope{
			Key:    channelID + ":p2p:" + msg.ChatID,
			Kind:   "dm",
			ChatID: msg.ChatID,
			UserID: msg.SenderID(),
		}, ""
	}

	rootID := msg.ThreadRootID()
	if cfg.ThreadIsolation && rootID != "" {
		return channel.ConversationScope{
			Key:      channelID + ":group-thread:" + msg.ChatID + ":" + rootID,
			Kind:     "thread",
			ChatID:   msg.ChatID,
			UserID:   msg.SenderID(),
			ThreadID: rootID,
		}, rootID
	}
	if cfg.ShareSessionInChannel {
		return channel.ConversationScope{
			Key:    channelID + ":group:" + msg.ChatID,
			Kind:   "group",
			ChatID: msg.ChatID,
			UserID: msg.SenderID(),
		}, ""
	}
	return channel.ConversationScope{
		Key:    channelID + ":group-user:" + msg.ChatID + ":" + msg.SenderID(),
		Kind:   "group",
		ChatID: msg.ChatID,
		UserID: msg.SenderID(),
	}, ""
}

func stripMentions(text string, mentions []mention, botOpenID string) string {
	result := text
	for _, item := range mentions {
		replacement := ""
		if botOpenID == "" || strings.TrimSpace(item.OpenID) != strings.TrimSpace(botOpenID) {
			name := strings.TrimSpace(item.Name)
			if name == "" {
				name = strings.TrimPrefix(strings.TrimSpace(item.Key), "@")
			}
			if name != "" {
				replacement = "@" + name
			}
		}
		if item.Key != "" {
			result = strings.ReplaceAll(result, item.Key, replacement)
		}
	}
	return strings.Join(strings.Fields(result), " ")
}

func mentionsBot(mentions []mention, botOpenID string) bool {
	if len(mentions) == 0 {
		return false
	}
	if strings.TrimSpace(botOpenID) == "" {
		return true
	}
	for _, item := range mentions {
		if strings.TrimSpace(item.OpenID) == strings.TrimSpace(botOpenID) {
			return true
		}
	}
	return false
}
