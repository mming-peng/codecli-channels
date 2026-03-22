package channel

import "context"

type Sender struct {
	ID          string
	DisplayName string
}

type ConversationScope struct {
	Key      string
	Kind     string
	ChatID   string
	UserID   string
	ThreadID string
}

type Message struct {
	ChannelID string
	Platform  string
	Scope     ConversationScope
	Sender    Sender
	MessageID string
	Text      string
	Timestamp string
	ReplyRef  any
	Metadata  map[string]string
}

type MessageSink func(context.Context, Message)

type Driver interface {
	ID() string
	Platform() string
	Start(context.Context, MessageSink) error
	Reply(context.Context, any, string) error
	Send(context.Context, any, string) error
	Stop(context.Context) error
}
