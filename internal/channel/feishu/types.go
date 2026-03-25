package feishu

import (
	"encoding/json"
	"fmt"
	"strings"

	cfgpkg "codecli-channels/internal/config"
)

const (
	chatTypeP2P             = "p2p"
	receiveMessageEventType = "im.message.receive_v1"
)

type options = driverConfig

type driverConfig struct {
	AppID                 string
	AppSecret             string
	AllowFrom             []string
	ShareSessionInChannel bool
	ThreadIsolation       bool
	GroupReplyAll         bool
}

type replyRef struct {
	MessageID     string `json:"message_id"`
	ChatID        string `json:"chat_id"`
	ScopeKey      string `json:"scope_key"`
	RootID        string `json:"root_id,omitempty"`
	ReplyInThread bool   `json:"reply_in_thread,omitempty"`
}

type mention struct {
	Key    string `json:"key"`
	OpenID string `json:"open_id"`
	Name   string `json:"name"`
}

type incomingMessage struct {
	ChatID    string
	ChatType  string
	UserID    string
	OpenID    string
	MessageID string
	RootID    string
	ThreadID  string
	ParentID  string
	Text      string
	Timestamp string
	Mentioned bool
	Mentions  []mention
}

func (m incomingMessage) SenderID() string {
	if strings.TrimSpace(m.UserID) != "" {
		return strings.TrimSpace(m.UserID)
	}
	return strings.TrimSpace(m.OpenID)
}

func (m incomingMessage) ThreadRootID() string {
	if strings.TrimSpace(m.RootID) != "" {
		return strings.TrimSpace(m.RootID)
	}
	if strings.TrimSpace(m.ThreadID) != "" {
		return strings.TrimSpace(m.ThreadID)
	}
	return strings.TrimSpace(m.ParentID)
}

type eventEnvelope struct {
	Schema string               `json:"schema"`
	Header eventHeader          `json:"header"`
	Event  *receiveMessageEvent `json:"event,omitempty"`
}

type eventHeader struct {
	EventType string `json:"event_type"`
}

type receiveMessageEvent struct {
	Sender  eventSender  `json:"sender"`
	Message eventMessage `json:"message"`
}

type eventSender struct {
	SenderID senderID `json:"sender_id"`
}

type senderID struct {
	OpenID string `json:"open_id"`
}

type eventMessage struct {
	MessageID   string         `json:"message_id"`
	RootID      string         `json:"root_id"`
	ThreadID    string         `json:"thread_id"`
	ParentID    string         `json:"parent_id"`
	CreateTime  string         `json:"create_time"`
	ChatID      string         `json:"chat_id"`
	ChatType    string         `json:"chat_type"`
	MessageType string         `json:"message_type"`
	Content     string         `json:"content"`
	Mentions    []eventMention `json:"mentions"`
}

type eventMention struct {
	Key  string   `json:"key"`
	Name string   `json:"name"`
	ID   senderID `json:"id"`
}

type textContent struct {
	Text string `json:"text"`
}

func resolveDriverConfig(cfg *cfgpkg.Config, channelID string) (driverConfig, error) {
	if cfg == nil {
		return driverConfig{}, fmt.Errorf("feishu driver 缺少配置")
	}
	channelCfg, ok := cfg.Channel(channelID)
	if !ok {
		return driverConfig{}, fmt.Errorf("未找到 Feishu channel %s", channelID)
	}
	appID, _ := channelCfg.Options["appId"].(string)
	appSecret, _ := channelCfg.Options["appSecret"].(string)
	if strings.TrimSpace(appID) == "" || strings.TrimSpace(appSecret) == "" {
		return driverConfig{}, fmt.Errorf("Feishu channel %s 缺少 appId/appSecret", channelID)
	}

	return driverConfig{
		AppID:                 strings.TrimSpace(appID),
		AppSecret:             strings.TrimSpace(appSecret),
		AllowFrom:             stringSliceOption(channelCfg.Options["allowFrom"]),
		ShareSessionInChannel: boolOption(channelCfg.Options["shareSessionInChannel"]),
		ThreadIsolation:       boolOption(channelCfg.Options["threadIsolation"]),
		GroupReplyAll:         boolOption(channelCfg.Options["groupReplyAll"]),
	}, nil
}

func stringSliceOption(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}

func boolOption(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func decodeIncomingMessage(envelope eventEnvelope) (incomingMessage, bool, error) {
	if envelope.Header.EventType != receiveMessageEventType || envelope.Event == nil {
		return incomingMessage{}, false, nil
	}
	if envelope.Event.Message.MessageType != "text" {
		return incomingMessage{}, false, nil
	}

	var content textContent
	if err := json.Unmarshal([]byte(envelope.Event.Message.Content), &content); err != nil {
		return incomingMessage{}, false, err
	}

	msg := incomingMessage{
		ChatID:    strings.TrimSpace(envelope.Event.Message.ChatID),
		ChatType:  strings.TrimSpace(envelope.Event.Message.ChatType),
		UserID:    strings.TrimSpace(envelope.Event.Sender.SenderID.OpenID),
		OpenID:    strings.TrimSpace(envelope.Event.Sender.SenderID.OpenID),
		MessageID: strings.TrimSpace(envelope.Event.Message.MessageID),
		RootID:    strings.TrimSpace(envelope.Event.Message.RootID),
		ThreadID:  strings.TrimSpace(envelope.Event.Message.ThreadID),
		ParentID:  strings.TrimSpace(envelope.Event.Message.ParentID),
		Text:      strings.TrimSpace(content.Text),
		Timestamp: strings.TrimSpace(envelope.Event.Message.CreateTime),
		Mentioned: len(envelope.Event.Message.Mentions) > 0,
	}
	for _, item := range envelope.Event.Message.Mentions {
		msg.Mentions = append(msg.Mentions, mention{
			Key:    strings.TrimSpace(item.Key),
			OpenID: strings.TrimSpace(item.ID.OpenID),
			Name:   strings.TrimSpace(item.Name),
		})
	}
	if msg.ChatType == "" {
		msg.ChatType = "group"
	}
	return msg, true, nil
}
