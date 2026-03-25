package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"codecli-channels/internal/channel"
	cfgpkg "codecli-channels/internal/config"
)

func TestStripMentionsRemovesBotAndKeepsOtherUsers(t *testing.T) {
	text := stripMentions("@_user_1 hello @_user_2", []mention{
		{Key: "@_user_1", OpenID: "ou_bot", Name: "机器人"},
		{Key: "@_user_2", OpenID: "ou_user_2", Name: "张三"},
	}, "ou_bot")
	if text != "hello @张三" {
		t.Fatalf("stripMentions() = %q, want %q", text, "hello @张三")
	}
}

func TestToChannelMessageP2P(t *testing.T) {
	driver := &Driver{
		id: "feishu-main",
		options: options{
			GroupReplyAll: true,
		},
	}
	msg, ok := driver.toChannelMessage(incomingMessage{
		ChatID:    "oc_1",
		ChatType:  "p2p",
		UserID:    "ou_1",
		MessageID: "om_1",
		Text:      "你好",
	})
	if !ok {
		t.Fatal("expected message")
	}
	if msg.Scope.Key != "feishu-main:p2p:oc_1" {
		t.Fatalf("Scope.Key = %q", msg.Scope.Key)
	}
	if msg.Scope.Kind != "dm" {
		t.Fatalf("Scope.Kind = %q, want dm", msg.Scope.Kind)
	}
	ref := msg.ReplyRef.(replyRef)
	if ref.ChatID != "oc_1" || ref.MessageID != "om_1" {
		t.Fatalf("unexpected replyRef: %#v", ref)
	}
}

func TestToChannelMessageGroupThreadIsolation(t *testing.T) {
	driver := &Driver{
		id:        "feishu-main",
		botOpenID: "ou_bot",
		options: options{
			ThreadIsolation: true,
		},
	}
	msg, ok := driver.toChannelMessage(incomingMessage{
		ChatID:    "oc_group",
		ChatType:  "group",
		UserID:    "ou_1",
		MessageID: "om_1",
		RootID:    "om_root_1",
		Text:      "@_user_1 帮我看下",
		Mentions: []mention{
			{Key: "@_user_1", OpenID: "ou_bot", Name: "机器人"},
		},
	})
	if !ok {
		t.Fatal("expected threaded group message")
	}
	if msg.Scope.Key != "feishu-main:group-thread:oc_group:om_root_1" {
		t.Fatalf("Scope.Key = %q", msg.Scope.Key)
	}
	if msg.Scope.Kind != "thread" {
		t.Fatalf("Scope.Kind = %q, want thread", msg.Scope.Kind)
	}
	ref := msg.ReplyRef.(replyRef)
	if !ref.ReplyInThread || ref.RootID != "om_root_1" {
		t.Fatalf("unexpected replyRef: %#v", ref)
	}
}

func TestToChannelMessageGroupSharedSession(t *testing.T) {
	driver := &Driver{
		id:        "feishu-main",
		botOpenID: "ou_bot",
		options: options{
			GroupReplyAll:         true,
			ShareSessionInChannel: true,
		},
	}
	msg, ok := driver.toChannelMessage(incomingMessage{
		ChatID:    "oc_group",
		ChatType:  "group",
		UserID:    "ou_1",
		MessageID: "om_1",
		Text:      "共享会话",
	})
	if !ok {
		t.Fatal("expected group message")
	}
	if msg.Scope.Key != "feishu-main:group:oc_group" {
		t.Fatalf("Scope.Key = %q", msg.Scope.Key)
	}
}

func TestToChannelMessageDropsUnmentionedGroupMessage(t *testing.T) {
	driver := &Driver{
		id:        "feishu-main",
		botOpenID: "ou_bot",
	}
	_, ok := driver.toChannelMessage(incomingMessage{
		ChatID:    "oc_group",
		ChatType:  "group",
		UserID:    "ou_1",
		MessageID: "om_1",
		Text:      "没有提到机器人",
	})
	if ok {
		t.Fatal("expected unmentioned group message to be ignored")
	}
}

func TestReplyAndSendRouteByMessageContext(t *testing.T) {
	api := &fakeAPI{}
	driver := &Driver{api: api}

	err := driver.Reply(context.Background(), replyRef{
		ChatID:    "oc_1",
		MessageID: "om_1",
	}, "回复")
	if err != nil {
		t.Fatalf("Reply error: %v", err)
	}
	if len(api.replyCalls) != 1 || api.replyCalls[0].Content != "回复" {
		t.Fatalf("unexpected reply calls: %#v", api.replyCalls)
	}

	err = driver.Send(context.Background(), replyRef{
		ChatID:    "oc_1",
		MessageID: "om_2",
	}, "进度")
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if len(api.replyCalls) != 2 {
		t.Fatalf("expected send with messageID to reuse reply path")
	}

	err = driver.Send(context.Background(), replyRef{
		ChatID: "oc_2",
	}, "新消息")
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if len(api.createCalls) != 1 || api.createCalls[0].ChatID != "oc_2" {
		t.Fatalf("unexpected create calls: %#v", api.createCalls)
	}
}

func TestBuildTextContent(t *testing.T) {
	if got := buildTextContent("你好"); got != `{"text":"你好"}` {
		t.Fatalf("buildTextContent() = %q", got)
	}
}

type fakeAPI struct {
	replyCalls  []replyCall
	createCalls []createCall
}

type replyCall struct {
	Ref     replyRef
	Content string
}

type createCall struct {
	ChatID  string
	Content string
}

func (f *fakeAPI) FetchBotOpenID(context.Context) (string, error) {
	return "ou_bot", nil
}

func (f *fakeAPI) ReplyText(_ context.Context, ref replyRef, content string) error {
	f.replyCalls = append(f.replyCalls, replyCall{Ref: ref, Content: content})
	return nil
}

func (f *fakeAPI) CreateText(_ context.Context, chatID, content string) error {
	f.createCalls = append(f.createCalls, createCall{ChatID: chatID, Content: content})
	return nil
}

func TestToChannelMessageFillsBridgeMetadata(t *testing.T) {
	driver := &Driver{
		id:        "feishu-main",
		botOpenID: "ou_bot",
		options: options{
			GroupReplyAll: true,
		},
	}
	msg, ok := driver.toChannelMessage(incomingMessage{
		ChatID:    "oc_group",
		ChatType:  "group",
		UserID:    "ou_1",
		MessageID: "om_1",
		RootID:    "om_root_1",
		ThreadID:  "omt_1",
		ParentID:  "om_parent_1",
		Text:      "hello",
	})
	if !ok {
		t.Fatal("expected message")
	}
	if msg.Metadata["chatType"] != "group" || msg.Metadata["targetId"] != "oc_group" || msg.Metadata["senderId"] != "ou_1" {
		t.Fatalf("unexpected metadata: %#v", msg.Metadata)
	}
	if msg.Scope.ChatID != "oc_group" {
		t.Fatalf("Scope.ChatID = %q", msg.Scope.ChatID)
	}
}

func TestAPIClientBuildsReplyAndCreateRequests(t *testing.T) {
	type recordedRequest struct {
		Path          string
		Query         string
		Authorization string
		Body          map[string]any
	}

	var (
		mu       sync.Mutex
		requests []recordedRequest
	)

	cfg := &cfgpkg.Config{
		Channels: map[string]cfgpkg.ChannelConfig{
			"feishu-main": {
				Alias:   "feishu-main",
				Type:    "feishu",
				Enabled: true,
				Options: map[string]any{
					"appId":           "cli_xxx",
					"appSecret":       "sec_xxx",
					"threadIsolation": true,
				},
			},
		},
	}

	driver := NewDriver("feishu-main", cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	api := driver.api.(*channelAPI)
	api.client.baseURL = "https://feishu.test"
	api.client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/open-apis/auth/v3/app_access_token/internal" {
				return jsonResponse(`{"code":0,"msg":"ok","app_access_token":"app-token","expire":7200}`), nil
			}

			defer r.Body.Close()
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				return nil, fmt.Errorf("decode request body: %w", err)
			}

			mu.Lock()
			requests = append(requests, recordedRequest{
				Path:          r.URL.Path,
				Query:         r.URL.RawQuery,
				Authorization: r.Header.Get("Authorization"),
				Body:          body,
			})
			mu.Unlock()

			return jsonResponse(`{"code":0,"msg":"ok","data":{"message_id":"om_resp"}}`), nil
		}),
	}

	if err := driver.Reply(context.Background(), replyRef{
		ChatID:        "oc_group",
		MessageID:     "om_source",
		RootID:        "om_root",
		ReplyInThread: true,
	}, "回复消息"); err != nil {
		t.Fatalf("Reply error: %v", err)
	}

	if err := driver.Send(context.Background(), replyRef{
		ChatID:    "oc_group",
		MessageID: "om_progress",
	}, "进度消息"); err != nil {
		t.Fatalf("Send reuse reply error: %v", err)
	}

	if err := driver.Send(context.Background(), replyRef{
		ChatID: "oc_group",
	}, "主动消息"); err != nil {
		t.Fatalf("Send create error: %v", err)
	}

	mu.Lock()
	seen := append([]recordedRequest(nil), requests...)
	mu.Unlock()

	if len(seen) != 3 {
		t.Fatalf("expected 3 API calls, got %d", len(seen))
	}

	for idx := range seen[:2] {
		req := seen[idx]
		if req.Path != "/open-apis/im/v1/messages/"+[]string{"om_source", "om_progress"}[idx]+"/reply" {
			t.Fatalf("request %d unexpected reply path: %s", idx, req.Path)
		}
		if req.Authorization != "Bearer app-token" {
			t.Fatalf("request %d unexpected auth header: %s", idx, req.Authorization)
		}
		if req.Body["msg_type"] != "text" {
			t.Fatalf("request %d unexpected msg_type: %#v", idx, req.Body["msg_type"])
		}
		content, _ := req.Body["content"].(string)
		if !strings.Contains(content, "\"text\":") {
			t.Fatalf("request %d expected JSON content string, got %#v", idx, req.Body["content"])
		}
	}

	if seen[0].Body["reply_in_thread"] != true {
		t.Fatalf("expected threaded reply body, got %#v", seen[0].Body)
	}
	if seen[1].Body["reply_in_thread"] != nil {
		t.Fatalf("did not expect plain reply to force thread mode, got %#v", seen[1].Body)
	}

	if seen[2].Path != "/open-apis/im/v1/messages" {
		t.Fatalf("unexpected create path: %s", seen[2].Path)
	}
	if seen[2].Query != "receive_id_type=chat_id" {
		t.Fatalf("unexpected create query: %s", seen[2].Query)
	}
	if seen[2].Body["receive_id"] != "oc_group" {
		t.Fatalf("unexpected receive_id: %#v", seen[2].Body["receive_id"])
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

var _ channel.Driver = (*Driver)(nil)
