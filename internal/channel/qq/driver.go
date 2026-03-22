package qq

import (
	"context"
	"fmt"
	"log/slog"

	"codecli-channels/internal/channel"
	cfgpkg "codecli-channels/internal/config"
)

type replyRef struct {
	TargetType string
	TargetID   string
	MessageID  string
}

type Driver struct {
	id      string
	cfg     *cfgpkg.Config
	api     *APIClient
	gateway *Gateway
	logger  *slog.Logger
}

func Register(registry *channel.Registry, cfg *cfgpkg.Config, logger *slog.Logger) {
	if registry == nil {
		return
	}
	registry.Register("qq", func(id string, channelCfg cfgpkg.ChannelConfig, dataDir string) (channel.Driver, error) {
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
		api:    NewAPIClient(cfg),
		logger: logger.With("channelId", id, "platform", "qq"),
	}
}

func (d *Driver) ID() string { return d.id }

func (d *Driver) Platform() string { return "qq" }

func (d *Driver) Start(ctx context.Context, sink channel.MessageSink) error {
	if sink == nil {
		return fmt.Errorf("qq driver 需要 message sink")
	}
	d.gateway = NewGateway(d.cfg, d.api, d.id, d.logger, func(ctx context.Context, msg IncomingMessage) {
		sink(ctx, toChannelMessage(d.id, msg))
	})
	d.gateway.Start(ctx)
	return nil
}

func (d *Driver) Reply(ctx context.Context, ref any, content string) error {
	reply, ok := ref.(replyRef)
	if !ok {
		if replyPtr, ok := ref.(*replyRef); ok && replyPtr != nil {
			reply = *replyPtr
		} else {
			return fmt.Errorf("qq driver 收到未知 reply ref")
		}
	}
	return d.api.ReplyMessage(ctx, d.id, reply.TargetType, reply.TargetID, reply.MessageID, content)
}

func (d *Driver) Send(ctx context.Context, ref any, content string) error {
	reply, ok := ref.(replyRef)
	if !ok {
		if replyPtr, ok := ref.(*replyRef); ok && replyPtr != nil {
			reply = *replyPtr
		} else {
			return fmt.Errorf("qq driver 收到未知 proactive ref")
		}
	}
	return d.api.ProactiveMessage(ctx, d.id, reply.TargetType, reply.TargetID, content)
}

func (d *Driver) Stop(context.Context) error {
	return nil
}

func toChannelMessage(channelID string, msg IncomingMessage) channel.Message {
	kind := "dm"
	if msg.ChatType == "group" {
		kind = "group"
	}
	return channel.Message{
		ChannelID: channelID,
		Platform:  "qq",
		Scope: channel.ConversationScope{
			Key:  scopeKey(channelID, msg.ChatType, msg.TargetID),
			Kind: kind,
		},
		Sender: channel.Sender{
			ID: msg.SenderID,
		},
		MessageID: msg.MessageID,
		Text:      msg.Text,
		Timestamp: msg.Timestamp,
		ReplyRef: replyRef{
			TargetType: msg.ChatType,
			TargetID:   msg.TargetID,
			MessageID:  msg.MessageID,
		},
		Metadata: map[string]string{
			"targetId": msg.TargetID,
			"senderId": msg.SenderID,
			"chatType": msg.ChatType,
		},
	}
}

func scopeKey(channelID, chatType, targetID string) string {
	return channelID + ":" + chatType + ":" + targetID
}
