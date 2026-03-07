package qq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"

	cfgpkg "qq-codex-go/internal/config"

	"github.com/gorilla/websocket"
)

const (
	intentPublicGuildMessages = 1 << 30
	intentDirectMessage       = 1 << 12
	intentGroupAndC2C         = 1 << 25
	identifyIntents           = intentPublicGuildMessages | intentDirectMessage | intentGroupAndC2C
)

var mentionRegexp = regexp.MustCompile(`<@!?\d+>`)

type Gateway struct {
	cfg       *cfgpkg.Config
	api       *APIClient
	accountID string
	handler   func(context.Context, IncomingMessage)
	logger    *slog.Logger

	mu      sync.Mutex
	conn    *websocket.Conn
	lastSeq *int64
}

type gatewayPayload struct {
	Op int             `json:"op"`
	S  *int64          `json:"s"`
	T  string          `json:"t"`
	D  json.RawMessage `json:"d"`
}

func NewGateway(cfg *cfgpkg.Config, api *APIClient, accountID string, logger *slog.Logger, handler func(context.Context, IncomingMessage)) *Gateway {
	return &Gateway{cfg: cfg, api: api, accountID: accountID, logger: logger, handler: handler}
}

func (g *Gateway) Start(ctx context.Context) {
	go g.loop(ctx)
}

func (g *Gateway) loop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := g.runOnce(ctx); err != nil && ctx.Err() == nil {
			g.logger.Error("QQ 网关连接失败", "accountId", g.accountID, "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
		}
	}
}

func (g *Gateway) runOnce(ctx context.Context) error {
	gatewayURL, err := g.api.GetGatewayURL(ctx, g.accountID)
	if err != nil {
		return err
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, gatewayURL, nil)
	if err != nil {
		return err
	}
	g.mu.Lock()
	g.conn = conn
	g.lastSeq = nil
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		if g.conn == conn {
			g.conn = nil
		}
		g.mu.Unlock()
		_ = conn.Close()
	}()

	var heartbeatCancel context.CancelFunc
	defer func() {
		if heartbeatCancel != nil {
			heartbeatCancel()
		}
	}()

	for {
		if ctx.Err() != nil {
			return nil
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var payload gatewayPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			continue
		}
		if payload.S != nil {
			g.mu.Lock()
			value := *payload.S
			g.lastSeq = &value
			g.mu.Unlock()
		}
		switch payload.Op {
		case 10:
			var hello struct {
				HeartbeatInterval int `json:"heartbeat_interval"`
			}
			if err := json.Unmarshal(payload.D, &hello); err != nil {
				return err
			}
			if heartbeatCancel != nil {
				heartbeatCancel()
			}
			hCtx, cancel := context.WithCancel(ctx)
			heartbeatCancel = cancel
			go g.heartbeatLoop(hCtx, conn, time.Duration(hello.HeartbeatInterval)*time.Millisecond)
			if err := g.identify(ctx, conn); err != nil {
				return err
			}
		case 0:
			if err := g.handleDispatch(ctx, payload.T, payload.D); err != nil {
				g.logger.Error("处理 QQ 事件失败", "accountId", g.accountID, "type", payload.T, "error", err)
			}
		case 7:
			return fmt.Errorf("服务端要求重连")
		case 9:
			return fmt.Errorf("invalid session")
		case 11:
			continue
		}
	}
}

func (g *Gateway) heartbeatLoop(ctx context.Context, conn *websocket.Conn, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.mu.Lock()
			seq := g.lastSeq
			g.mu.Unlock()
			payload := map[string]any{"op": 1, "d": nil}
			if seq != nil {
				payload["d"] = *seq
			}
			if err := conn.WriteJSON(payload); err != nil {
				g.logger.Error("发送心跳失败", "accountId", g.accountID, "error", err)
				return
			}
		}
	}
}

func (g *Gateway) identify(ctx context.Context, conn *websocket.Conn) error {
	token, err := g.api.GetAccessToken(ctx, g.accountID)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"op": 2,
		"d": map[string]any{
			"token":   "QQBot " + token,
			"intents": identifyIntents,
			"shard":   []int{0, 1},
		},
	}
	return conn.WriteJSON(payload)
}

func (g *Gateway) handleDispatch(ctx context.Context, eventType string, raw json.RawMessage) error {
	switch eventType {
	case "READY":
		g.logger.Info("QQ 网关 READY", "accountId", g.accountID)
		return nil
	case "C2C_MESSAGE_CREATE":
		var payload struct {
			ID        string `json:"id"`
			Content   string `json:"content"`
			Timestamp string `json:"timestamp"`
			Author    struct {
				UserOpenID string `json:"user_openid"`
			} `json:"author"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return err
		}
		go g.handler(context.Background(), IncomingMessage{
			AccountID: g.accountID,
			ChatType:  "user",
			TargetID:  payload.Author.UserOpenID,
			SenderID:  payload.Author.UserOpenID,
			MessageID: payload.ID,
			Text:      stripMentions(payload.Content),
			Timestamp: payload.Timestamp,
		})
		return nil
	case "GROUP_AT_MESSAGE_CREATE":
		var payload struct {
			ID          string `json:"id"`
			Content     string `json:"content"`
			Timestamp   string `json:"timestamp"`
			GroupOpenID string `json:"group_openid"`
			Author      struct {
				MemberOpenID string `json:"member_openid"`
			} `json:"author"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return err
		}
		go g.handler(context.Background(), IncomingMessage{
			AccountID: g.accountID,
			ChatType:  "group",
			TargetID:  payload.GroupOpenID,
			SenderID:  payload.Author.MemberOpenID,
			MessageID: payload.ID,
			Text:      stripMentions(payload.Content),
			Timestamp: payload.Timestamp,
		})
		return nil
	default:
		return nil
	}
}

func stripMentions(text string) string {
	text = mentionRegexp.ReplaceAllString(text, "")
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return text
}
