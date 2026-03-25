package feishu

import (
	"context"
	"encoding/json"
	"log/slog"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

type Gateway struct {
	cfg     driverConfig
	logger  *slog.Logger
	handler func(context.Context, incomingMessage)
}

func NewGateway(cfg driverConfig, logger *slog.Logger, handler func(context.Context, incomingMessage)) *Gateway {
	if logger == nil {
		logger = slog.Default()
	}
	return &Gateway{
		cfg:     cfg,
		logger:  logger,
		handler: handler,
	}
}

func (g *Gateway) Start(ctx context.Context) error {
	if g.handler == nil {
		return nil
	}
	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			msg, ok := decodeSDKIncomingMessage(event)
			if !ok || !g.shouldAccept(msg) {
				return nil
			}
			g.handler(ctx, msg)
			return nil
		})

	client := larkws.NewClient(
		g.cfg.AppID,
		g.cfg.AppSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)
	go func() {
		if err := client.Start(ctx); err != nil && ctx.Err() == nil {
			g.logger.Error("飞书长连接失败", "error", err)
		}
	}()
	return nil
}

func (g *Gateway) Consume(ctx context.Context, raw []byte) error {
	var envelope eventEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	msg, ok, err := decodeIncomingMessage(envelope)
	if err != nil {
		return err
	}
	if !ok || !g.shouldAccept(msg) || g.handler == nil {
		return nil
	}
	g.handler(ctx, msg)
	return nil
}

func (g *Gateway) shouldAccept(msg incomingMessage) bool {
	if len(g.cfg.AllowFrom) > 0 {
		allowed := false
		for _, item := range g.cfg.AllowFrom {
			if item == msg.SenderID() {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	if msg.ChatType != chatTypeP2P && !g.cfg.GroupReplyAll && !msg.Mentioned {
		return false
	}
	return msg.Text != ""
}

func decodeSDKIncomingMessage(event *larkim.P2MessageReceiveV1) (incomingMessage, bool) {
	if event == nil || event.Event == nil || event.Event.Message == nil || event.Event.Sender == nil {
		return incomingMessage{}, false
	}
	msg := event.Event.Message
	if msg.Content == nil || stringValue(msg.MessageType) != "text" {
		return incomingMessage{}, false
	}

	var content textContent
	if err := json.Unmarshal([]byte(*msg.Content), &content); err != nil {
		return incomingMessage{}, false
	}

	incoming := incomingMessage{
		ChatID:    stringValue(msg.ChatId),
		ChatType:  stringValue(msg.ChatType),
		UserID:    sdkSenderOpenID(event),
		OpenID:    sdkSenderOpenID(event),
		MessageID: stringValue(msg.MessageId),
		RootID:    stringValue(msg.RootId),
		ThreadID:  stringValue(msg.ThreadId),
		ParentID:  stringValue(msg.ParentId),
		Text:      content.Text,
		Timestamp: stringValue(msg.CreateTime),
		Mentioned: len(msg.Mentions) > 0,
	}
	for _, item := range msg.Mentions {
		if item == nil {
			continue
		}
		mentionItem := mention{
			Key:  stringValue(item.Key),
			Name: stringValue(item.Name),
		}
		if item.Id != nil && item.Id.OpenId != nil {
			mentionItem.OpenID = *item.Id.OpenId
		}
		incoming.Mentions = append(incoming.Mentions, mentionItem)
	}
	if incoming.ChatType == "" {
		incoming.ChatType = "group"
	}
	return incoming, true
}

func sdkSenderOpenID(event *larkim.P2MessageReceiveV1) string {
	if event == nil || event.Event == nil || event.Event.Sender == nil || event.Event.Sender.SenderId == nil || event.Event.Sender.SenderId.OpenId == nil {
		return ""
	}
	return *event.Event.Sender.SenderId.OpenId
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
