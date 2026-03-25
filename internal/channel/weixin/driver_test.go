package weixin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"codecli-channels/internal/channel"
	cfgpkg "codecli-channels/internal/config"
)

func TestBodyFromItemListBuildsQuotedText(t *testing.T) {
	items := []messageItem{
		{
			Type: messageItemText,
			TextItem: &textItem{
				Text: "新的回复",
			},
			RefMsg: &refMessage{
				Title: "原消息",
				MessageItem: &messageItem{
					Type: messageItemText,
					TextItem: &textItem{
						Text: "旧内容",
					},
				},
			},
		},
	}

	got := bodyFromItemList(items)
	want := "[引用: 原消息 | 旧内容]\n新的回复"
	if got != want {
		t.Fatalf("bodyFromItemList() = %q, want %q", got, want)
	}
}

func TestBodyFromItemListJoinsTextVoiceAndQuote(t *testing.T) {
	items := []messageItem{
		{
			Type: messageItemText,
			TextItem: &textItem{
				Text: "第一句",
			},
		},
		{
			Type: messageItemVoice,
			VoiceItem: &voiceItem{
				Text: "语音转文字",
			},
		},
		{
			Type: messageItemText,
			TextItem: &textItem{
				Text: "第二句",
			},
			RefMsg: &refMessage{
				Title: "上条消息",
				MessageItem: &messageItem{
					Type: messageItemText,
					TextItem: &textItem{
						Text: "被引用内容",
					},
				},
			},
		},
	}

	got := bodyFromItemList(items)
	want := "第一句\n语音转文字\n[引用: 上条消息 | 被引用内容]\n第二句"
	if got != want {
		t.Fatalf("bodyFromItemList() = %q, want %q", got, want)
	}
}

func TestSplitUTF8SplitsLongText(t *testing.T) {
	long := strings.Repeat("你", maxWeixinChunk+5)
	chunks := splitUTF8(long, maxWeixinChunk)
	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2", len(chunks))
	}
	if strings.Join(chunks, "") != long {
		t.Fatalf("unexpected chunks join result")
	}
}

func TestDriverPollsInboundMessages(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/ilink/bot/getupdates" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		mu.Lock()
		requests++
		current := requests
		mu.Unlock()

		if current == 1 {
			return jsonResponse(getUpdatesResp{
				Ret:           0,
				GetUpdatesBuf: "cursor-1",
				Msgs: []weixinMessage{
					{
						MessageID:    101,
						FromUserID:   "user@im.wechat",
						ToUserID:     "bot@im.wechat",
						CreateTimeMs: time.Date(2026, 3, 22, 19, 0, 0, 0, time.Local).UnixMilli(),
						MessageType:  messageTypeUser,
						ContextToken: "ctx-1",
						ItemList: []messageItem{
							{
								Type: messageItemText,
								TextItem: &textItem{
									Text: "你好",
								},
							},
						},
					},
				},
			})
		}
		return jsonResponse(getUpdatesResp{
			Ret:           0,
			GetUpdatesBuf: "cursor-1",
		})
	})}

	cfg := testWeixinConfig("https://weixin.test", []string{"user@im.wechat"})
	driver, err := NewDriver("weixin-main", cfg, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewDriver error: %v", err)
	}
	driver.api.httpClient = client

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgCh := make(chan channel.Message, 1)
	if err := driver.Start(ctx, func(_ context.Context, msg channel.Message) {
		select {
		case msgCh <- msg:
		default:
		}
		cancel()
	}); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer func() {
		if err := driver.Stop(context.Background()); err != nil {
			t.Fatalf("Stop error: %v", err)
		}
	}()

	select {
	case msg := <-msgCh:
		if msg.Platform != "weixin" {
			t.Fatalf("Platform = %q, want weixin", msg.Platform)
		}
		if msg.Scope.Key != "weixin-main:dm:user@im.wechat" {
			t.Fatalf("Scope.Key = %q", msg.Scope.Key)
		}
		if msg.Scope.Kind != "dm" {
			t.Fatalf("Scope.Kind = %q, want dm", msg.Scope.Kind)
		}
		if msg.Sender.ID != "user@im.wechat" {
			t.Fatalf("Sender.ID = %q", msg.Sender.ID)
		}
		if msg.Text != "你好" {
			t.Fatalf("Text = %q, want 你好", msg.Text)
		}
		if msg.Metadata["chatType"] != "dm" {
			t.Fatalf("Metadata[chatType] = %q, want dm", msg.Metadata["chatType"])
		}
		if msg.Metadata["targetId"] != "user@im.wechat" {
			t.Fatalf("Metadata[targetId] = %q", msg.Metadata["targetId"])
		}
		ref, ok := msg.ReplyRef.(replyRef)
		if !ok {
			t.Fatalf("ReplyRef type = %T, want replyRef", msg.ReplyRef)
		}
		if ref.PeerUserID != "user@im.wechat" || ref.ContextToken != "ctx-1" {
			t.Fatalf("unexpected replyRef: %#v", ref)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for inbound message")
	}

	cursor, err := driver.state.LoadCursor()
	if err != nil {
		t.Fatalf("LoadCursor error: %v", err)
	}
	if cursor != "cursor-1" {
		t.Fatalf("cursor = %q, want cursor-1", cursor)
	}
	token, ok, err := driver.state.LoadContextToken("user@im.wechat")
	if err != nil {
		t.Fatalf("LoadContextToken error: %v", err)
	}
	if !ok || token != "ctx-1" {
		t.Fatalf("context token = (%v, %q), want (true, ctx-1)", ok, token)
	}
}

func TestDriverReplyUsesStoredContextToken(t *testing.T) {
	var received sendMessageReq
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/ilink/bot/sendmessage" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Fatalf("Authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return jsonResponse(sendMessageResp{Ret: 0})
	})}

	driver, err := NewDriver("weixin-main", testWeixinConfig("https://weixin.test", nil), t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewDriver error: %v", err)
	}
	driver.api.httpClient = client
	if err := driver.state.SaveContextToken("user@im.wechat", "ctx-from-state"); err != nil {
		t.Fatalf("SaveContextToken error: %v", err)
	}

	err = driver.Reply(context.Background(), replyRef{
		PeerUserID: "user@im.wechat",
	}, "回复内容")
	if err != nil {
		t.Fatalf("Reply error: %v", err)
	}

	if received.Msg.ToUserID != "user@im.wechat" {
		t.Fatalf("ToUserID = %q", received.Msg.ToUserID)
	}
	if received.Msg.ContextToken != "ctx-from-state" {
		t.Fatalf("ContextToken = %q", received.Msg.ContextToken)
	}
	if len(received.Msg.ItemList) != 1 || received.Msg.ItemList[0].TextItem == nil || received.Msg.ItemList[0].TextItem.Text != "回复内容" {
		t.Fatalf("unexpected item list: %#v", received.Msg.ItemList)
	}
}

func TestDriverSendSplitsLongText(t *testing.T) {
	var mu sync.Mutex
	var bodies []sendMessageReq
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/ilink/bot/sendmessage" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body sendMessageReq
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		return jsonResponse(sendMessageResp{Ret: 0})
	})}

	driver, err := NewDriver("weixin-main", testWeixinConfig("https://weixin.test", nil), t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewDriver error: %v", err)
	}
	driver.api.httpClient = client

	text := strings.Repeat("你", maxWeixinChunk+8)
	err = driver.Send(context.Background(), replyRef{
		PeerUserID:   "user@im.wechat",
		ContextToken: "ctx-1",
	}, text)
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("len(bodies) = %d, want 2", len(bodies))
	}
	if got := bodies[0].Msg.ItemList[0].TextItem.Text + bodies[1].Msg.ItemList[0].TextItem.Text; got != text {
		t.Fatalf("joined text mismatch")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func jsonResponse(payload any) (*http.Response, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}, nil
}

func testWeixinConfig(baseURL string, allowFrom []string) *cfgpkg.Config {
	options := map[string]any{
		"token":   "token-1",
		"baseUrl": baseURL,
	}
	if allowFrom != nil {
		options["allowFrom"] = allowFrom
	}
	return &cfgpkg.Config{
		Channels: map[string]cfgpkg.ChannelConfig{
			"weixin-main": {
				Alias:   "weixin-main",
				Type:    "weixin",
				Enabled: true,
				Options: options,
			},
		},
	}
}
