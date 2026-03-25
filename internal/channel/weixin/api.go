package weixin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL          = "https://ilinkai.weixin.qq.com"
	defaultChannelVersion   = "codecli-channels-weixin/1.0"
	defaultLongPollTimeout  = 35 * time.Second
	defaultAPITimeout       = 15 * time.Second
	maxIlinkHTTPResponseBody = 64 << 20
	maxWeixinChunk          = 3800
)

type apiClient struct {
	baseURL        string
	token          string
	routeTag       string
	channelVersion string
	httpClient     *http.Client
}

func newAPIClient(baseURL, token, routeTag, channelVersion string, httpClient *http.Client) *apiClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	if strings.TrimSpace(channelVersion) == "" {
		channelVersion = defaultChannelVersion
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultAPITimeout}
	}
	return &apiClient{
		baseURL:        strings.TrimRight(baseURL, "/") + "/",
		token:          strings.TrimSpace(token),
		routeTag:       strings.TrimSpace(routeTag),
		channelVersion: channelVersion,
		httpClient:     httpClient,
	}
}

func (c *apiClient) longPollClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultLongPollTimeout
	}
	var transport http.RoundTripper = http.DefaultTransport
	if c.httpClient != nil && c.httpClient.Transport != nil {
		transport = c.httpClient.Transport
	} else if cloned, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = cloned.Clone()
	}
	return &http.Client{
		Timeout:   timeout + 5*time.Second,
		Transport: transport,
	}
}

func randomWechatUIN() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return base64.StdEncoding.EncodeToString([]byte("0000"))
	}
	value := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", value)))
}

func (c *apiClient) post(ctx context.Context, endpoint string, payload []byte, timeout time.Duration, label string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+strings.TrimPrefix(endpoint, "/"), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("weixin: %s: new request: %w", label, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(payload)))
	req.Header.Set("X-WECHAT-UIN", randomWechatUIN())
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.routeTag != "" {
		req.Header.Set("SKRouteTag", c.routeTag)
	}

	client := c.httpClient
	if timeout > 0 {
		client = c.longPollClient(timeout)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weixin: %s: %w", label, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxIlinkHTTPResponseBody+1))
	if err != nil {
		return nil, fmt.Errorf("weixin: %s: read body: %w", label, err)
	}
	if len(raw) > maxIlinkHTTPResponseBody {
		return nil, fmt.Errorf("weixin: %s: response body exceeds %d bytes", label, maxIlinkHTTPResponseBody)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weixin: %s: http %d: %s", label, resp.StatusCode, truncateForLog(raw, 256))
	}
	return raw, nil
}

func (c *apiClient) getUpdates(ctx context.Context, cursor string, timeoutMs int) (*getUpdatesResp, error) {
	timeout := defaultLongPollTimeout
	if timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}
	payload, err := json.Marshal(getUpdatesReq{
		GetUpdatesBuf: cursor,
		BaseInfo:      baseInfo{ChannelVersion: c.channelVersion},
	})
	if err != nil {
		return nil, err
	}
	raw, err := c.post(ctx, "ilink/bot/getupdates", payload, timeout, "getUpdates")
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return &getUpdatesResp{Ret: 0, GetUpdatesBuf: cursor}, nil
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return &getUpdatesResp{Ret: 0, GetUpdatesBuf: cursor}, nil
		}
		return nil, err
	}
	var resp getUpdatesResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("weixin: getUpdates json: %w", err)
	}
	return &resp, nil
}

func (c *apiClient) sendMessage(ctx context.Context, req sendMessageReq) error {
	req.BaseInfo = baseInfo{ChannelVersion: c.channelVersion}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	raw, err := c.post(ctx, "ilink/bot/sendmessage", payload, 0, "sendMessage")
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var resp sendMessageResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("weixin: sendMessage json: %w", err)
	}
	if resp.Ret != 0 {
		return fmt.Errorf("weixin: sendMessage: ret=%d errcode=%d errmsg=%s", resp.Ret, resp.Errcode, resp.Errmsg)
	}
	return nil
}

func (c *apiClient) sendText(ctx context.Context, to, text, contextToken, clientID string) error {
	if strings.TrimSpace(contextToken) == "" {
		return fmt.Errorf("weixin: context_token is required for send")
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return c.sendMessage(ctx, sendMessageReq{
		Msg: weixinOutboundMsg{
			ToUserID:     to,
			ClientID:     clientID,
			MessageType:  messageTypeBot,
			MessageState: messageStateFinish,
			ItemList: []messageItem{
				{
					Type:     messageItemText,
					TextItem: &textItem{Text: text},
				},
			},
			ContextToken: contextToken,
		},
	})
}

func truncateForLog(raw []byte, max int) string {
	text := string(raw)
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}
