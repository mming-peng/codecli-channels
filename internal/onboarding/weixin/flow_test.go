package weixin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRunSetupFlowRefreshesExpiredQRCode(t *testing.T) {
	var (
		getQRCalls     int
		firstQRPolls   int
		secondQRPolls  int
		printedQRs     []string
		savedQRPayload []string
	)
	client := newTestClient(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/ilink/bot/get_bot_qrcode"):
			getQRCalls++
			payload := map[string]any{
				"qrcode":             "qr-1",
				"qrcode_img_content": "weixin://qr/1",
			}
			if getQRCalls > 1 {
				payload["qrcode"] = "qr-2"
				payload["qrcode_img_content"] = "weixin://qr/2"
			}
			return jsonResponse(payload), nil
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/ilink/bot/get_qrcode_status"):
			switch r.URL.Query().Get("qrcode") {
			case "qr-1":
				firstQRPolls++
				return jsonResponse(map[string]any{"status": "expired"}), nil
			case "qr-2":
				secondQRPolls++
				if secondQRPolls == 1 {
					return jsonResponse(map[string]any{"status": "scaned"}), nil
				}
				return jsonResponse(map[string]any{
					"status":        "confirmed",
					"bot_token":     "bot-token",
					"ilink_bot_id":  "bot-id",
					"baseurl":       "https://ilink.test",
					"ilink_user_id": "user@im.wechat",
				}), nil
			default:
				t.Fatalf("unexpected qrcode: %s", r.URL.Query().Get("qrcode"))
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		return nil, nil
	})

	result, err := RunSetupFlow(context.Background(), SetupOptions{
		APIBaseURL: "https://ilink.test",
		Timeout:    2 * time.Second,
		Sleep:      func(time.Duration) {},
		PrintQRCode: func(content string) {
			printedQRs = append(printedQRs, content)
		},
		SaveQRCode: func(content, path string) error {
			savedQRPayload = append(savedQRPayload, content+"@"+path)
			return nil
		},
		QRImagePath: "qr.png",
		HTTPClient:  client,
	})
	if err != nil {
		t.Fatalf("RunSetupFlow error: %v", err)
	}
	if result.BotToken != "bot-token" {
		t.Fatalf("BotToken = %q, want bot-token", result.BotToken)
	}
	if result.IlinkUserID != "user@im.wechat" {
		t.Fatalf("IlinkUserID = %q, want user@im.wechat", result.IlinkUserID)
	}
	if getQRCalls != 2 {
		t.Fatalf("get_bot_qrcode calls = %d, want 2", getQRCalls)
	}
	if firstQRPolls != 1 || secondQRPolls != 2 {
		t.Fatalf("unexpected poll counts: first=%d second=%d", firstQRPolls, secondQRPolls)
	}
	if len(printedQRs) != 2 {
		t.Fatalf("printed QR count = %d, want 2", len(printedQRs))
	}
	if len(savedQRPayload) != 2 {
		t.Fatalf("saved QR count = %d, want 2", len(savedQRPayload))
	}
}

func TestRunSetupFlowStopsAfterTooManyExpiredQRCodes(t *testing.T) {
	client := newTestClient(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/ilink/bot/get_bot_qrcode"):
			return jsonResponse(map[string]any{
				"qrcode":             time.Now().Format("150405.000000"),
				"qrcode_img_content": "weixin://qr/expired",
			}), nil
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/ilink/bot/get_qrcode_status"):
			return jsonResponse(map[string]any{"status": "expired"}), nil
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		return nil, nil
	})

	_, err := RunSetupFlow(context.Background(), SetupOptions{
		APIBaseURL:  "https://ilink.test",
		Timeout:     2 * time.Second,
		Sleep:       func(time.Duration) {},
		PrintQRCode: func(string) {},
		HTTPClient:  client,
	})
	if err == nil || !strings.Contains(err.Error(), "二维码多次过期") {
		t.Fatalf("expected expiry error, got %v", err)
	}
}

func TestVerifyTokenUsesGetUpdates(t *testing.T) {
	var sawAuth bool
	client := newTestClient(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/ilink/bot/getupdates") {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if got := r.Header.Get("AuthorizationType"); got != "ilink_bot_token" {
			t.Fatalf("AuthorizationType = %q, want ilink_bot_token", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q, want Bearer test-token", got)
		}
		if got := r.Header.Get("X-WECHAT-UIN"); strings.TrimSpace(got) == "" {
			t.Fatal("expected X-WECHAT-UIN header")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll error: %v", err)
		}
		if !strings.Contains(string(raw), `"get_updates_buf":""`) {
			t.Fatalf("request body missing empty cursor: %s", string(raw))
		}
		sawAuth = true
		return jsonResponse(map[string]any{"ret": 0}), nil
	})

	if err := VerifyToken(context.Background(), VerifyTokenOptions{
		APIBaseURL: "https://ilink.test",
		Token:      "test-token",
		HTTPClient: client,
	}); err != nil {
		t.Fatalf("VerifyToken error: %v", err)
	}
	if !sawAuth {
		t.Fatal("expected verification request to be sent")
	}
}

func TestVerifyTokenRejectsBusinessError(t *testing.T) {
	client := newTestClient(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(map[string]any{
			"ret":     1,
			"errcode": 401,
			"errmsg":  "invalid token",
		}), nil
	})

	err := VerifyToken(context.Background(), VerifyTokenOptions{
		APIBaseURL: "https://ilink.test",
		Token:      "bad-token",
		HTTPClient: client,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("expected business error, got %v", err)
	}
}

func TestRunSetupFlowHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := RunSetupFlow(ctx, SetupOptions{
		APIBaseURL:  "https://ilink.test",
		PrintQRCode: func(string) {},
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newTestClient(fn roundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func jsonResponse(payload any) *http.Response {
	raw, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(raw))),
	}
}
