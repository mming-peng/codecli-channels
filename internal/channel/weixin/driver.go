package weixin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"codecli-channels/internal/channel"
	cfgpkg "codecli-channels/internal/config"
)

type replyRef struct {
	PeerUserID   string
	ContextToken string
}

type options struct {
	Token             string
	BaseURL           string
	RouteTag          string
	AllowFrom         []string
	LongPollTimeoutMS int
	ChannelVersion    string
}

type Driver struct {
	id      string
	options options
	api     *apiClient
	state   *State
	logger  *slog.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func Register(registry *channel.Registry, cfg *cfgpkg.Config, logger *slog.Logger) {
	if registry == nil {
		return
	}
	registry.Register("weixin", func(id string, channelCfg cfgpkg.ChannelConfig, dataDir string) (channel.Driver, error) {
		_ = channelCfg
		return NewDriver(id, cfg, dataDir, logger)
	})
}

func NewDriver(id string, cfg *cfgpkg.Config, dataDir string, logger *slog.Logger) (*Driver, error) {
	if logger == nil {
		logger = slog.Default()
	}
	state, err := NewState(dataDir)
	if err != nil {
		return nil, err
	}
	opts, err := resolveOptions(cfg, id)
	if err != nil {
		return nil, err
	}
	return &Driver{
		id:      id,
		options: opts,
		api:     newAPIClient(opts.BaseURL, opts.Token, opts.RouteTag, opts.ChannelVersion, nil),
		state:   state,
		logger:  logger.With("channelId", id, "platform", "weixin"),
	}, nil
}

func (d *Driver) ID() string { return d.id }

func (d *Driver) Platform() string { return "weixin" }

func (d *Driver) Start(ctx context.Context, sink channel.MessageSink) error {
	if sink == nil {
		return fmt.Errorf("weixin driver 需要 message sink")
	}
	runCtx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	if d.cancel != nil {
		d.mu.Unlock()
		cancel()
		return nil
	}
	d.cancel = cancel
	d.done = make(chan struct{})
	done := d.done
	d.mu.Unlock()

	go func() {
		defer close(done)
		d.pollLoop(runCtx, sink)
	}()
	return nil
}

func (d *Driver) Reply(ctx context.Context, ref any, content string) error {
	return d.sendText(ctx, ref, content)
}

func (d *Driver) Send(ctx context.Context, ref any, content string) error {
	return d.sendText(ctx, ref, content)
}

func (d *Driver) Stop(ctx context.Context) error {
	d.mu.Lock()
	cancel := d.cancel
	done := d.done
	d.cancel = nil
	d.done = nil
	d.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *Driver) pollLoop(ctx context.Context, sink channel.MessageSink) {
	cursor, err := d.state.LoadCursor()
	if err != nil {
		d.logger.Error("加载微信游标失败", "error", err)
	}
	for {
		if ctx.Err() != nil {
			return
		}
		resp, err := d.api.getUpdates(ctx, cursor, d.options.LongPollTimeoutMS)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			d.logger.Error("微信 getUpdates 失败", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		if resp.GetUpdatesBuf != "" && resp.GetUpdatesBuf != cursor {
			if err := d.state.SaveCursor(resp.GetUpdatesBuf); err != nil {
				d.logger.Error("保存微信游标失败", "error", err)
			} else {
				cursor = resp.GetUpdatesBuf
			}
		}
		for _, inbound := range resp.Msgs {
			msg, ok := d.toChannelMessage(inbound)
			if !ok {
				continue
			}
			sink(ctx, msg)
		}
	}
}

func (d *Driver) toChannelMessage(inbound weixinMessage) (channel.Message, bool) {
	if inbound.MessageType == messageTypeBot {
		return channel.Message{}, false
	}
	if inbound.MessageType != 0 && inbound.MessageType != messageTypeUser {
		return channel.Message{}, false
	}
	from := strings.TrimSpace(inbound.FromUserID)
	if from == "" {
		return channel.Message{}, false
	}
	if !d.isAllowedSender(from) {
		return channel.Message{}, false
	}
	if token := strings.TrimSpace(inbound.ContextToken); token != "" {
		if err := d.state.SaveContextToken(from, token); err != nil {
			d.logger.Error("保存微信 context token 失败", "peer", from, "error", err)
		}
	}
	body := strings.TrimSpace(bodyFromItemList(inbound.ItemList))
	if body == "" && mediaOnlyItems(inbound.ItemList) {
		body = "[暂不支持媒体消息，请改发文字]"
	}
	if body == "" {
		return channel.Message{}, false
	}
	messageID := strconv.FormatInt(inbound.MessageID, 10)
	if strings.TrimSpace(messageID) == "" || inbound.MessageID == 0 {
		messageID = randomHex(8)
	}
	timestamp := ""
	if inbound.CreateTimeMs > 0 {
		timestamp = time.UnixMilli(inbound.CreateTimeMs).Format(time.RFC3339)
	}
	return channel.Message{
		ChannelID: d.id,
		Platform:  "weixin",
		Scope: channel.ConversationScope{
			Key:    d.id + ":dm:" + from,
			Kind:   "dm",
			ChatID: from,
			UserID: from,
		},
		Sender: channel.Sender{
			ID:          from,
			DisplayName: from,
		},
		MessageID: messageID,
		Text:      body,
		Timestamp: timestamp,
		ReplyRef: replyRef{
			PeerUserID:   from,
			ContextToken: strings.TrimSpace(inbound.ContextToken),
		},
		Metadata: map[string]string{
			"chatType": "dm",
			"targetId": from,
			"senderId": from,
		},
	}, true
}

func (d *Driver) sendText(ctx context.Context, ref any, content string) error {
	reply, err := coerceReplyRef(ref)
	if err != nil {
		return err
	}
	if reply.ContextToken == "" {
		token, ok, loadErr := d.state.LoadContextToken(reply.PeerUserID)
		if loadErr != nil {
			return loadErr
		}
		if ok {
			reply.ContextToken = token
		}
	}
	if strings.TrimSpace(reply.ContextToken) == "" {
		return fmt.Errorf("weixin: missing context_token for peer %q", reply.PeerUserID)
	}
	for _, chunk := range splitUTF8(content, maxWeixinChunk) {
		if err := d.api.sendText(ctx, reply.PeerUserID, chunk, reply.ContextToken, "cc-"+randomHex(6)); err != nil {
			return err
		}
	}
	return nil
}

func (d *Driver) isAllowedSender(sender string) bool {
	if len(d.options.AllowFrom) == 0 {
		return true
	}
	for _, candidate := range d.options.AllowFrom {
		if strings.TrimSpace(candidate) == sender {
			return true
		}
	}
	return false
}

func resolveOptions(cfg *cfgpkg.Config, id string) (options, error) {
	channelCfg, ok := cfg.Channel(id)
	if !ok {
		return options{}, fmt.Errorf("未找到 weixin channel %s", id)
	}
	token := lookupString(channelCfg.Options, "token")
	if strings.TrimSpace(token) == "" {
		return options{}, fmt.Errorf("weixin channel %s 缺少 token", id)
	}
	return options{
		Token:             token,
		BaseURL:           lookupString(channelCfg.Options, "baseUrl", "base_url"),
		RouteTag:          lookupString(channelCfg.Options, "routeTag", "route_tag"),
		AllowFrom:         lookupStringSlice(channelCfg.Options, "allowFrom", "allow_from"),
		LongPollTimeoutMS: lookupInt(channelCfg.Options, "longPollTimeoutMs", "long_poll_timeout_ms"),
		ChannelVersion:    lookupString(channelCfg.Options, "channelVersion", "channel_version"),
	}, nil
}

func coerceReplyRef(value any) (replyRef, error) {
	switch ref := value.(type) {
	case replyRef:
		return ref, nil
	case *replyRef:
		if ref == nil {
			return replyRef{}, fmt.Errorf("weixin: nil reply ref")
		}
		return *ref, nil
	default:
		return replyRef{}, fmt.Errorf("weixin: invalid reply ref %T", value)
	}
}

func lookupString(options map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := options[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func lookupStringSlice(options map[string]any, keys ...string) []string {
	for _, key := range keys {
		raw, ok := options[key]
		if !ok {
			continue
		}
		switch values := raw.(type) {
		case []string:
			return append([]string(nil), values...)
		case []any:
			items := make([]string, 0, len(values))
			for _, item := range values {
				text, ok := item.(string)
				if ok && strings.TrimSpace(text) != "" {
					items = append(items, strings.TrimSpace(text))
				}
			}
			return items
		case string:
			if strings.TrimSpace(values) != "" {
				return []string{strings.TrimSpace(values)}
			}
		}
	}
	return nil
}

func lookupInt(options map[string]any, keys ...string) int {
	for _, key := range keys {
		raw, ok := options[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case int:
			return value
		case int64:
			return int(value)
		case float64:
			return int(value)
		}
	}
	return 0
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}
