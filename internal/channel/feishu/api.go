package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	cfgpkg "codecli-channels/internal/config"
)

const defaultBaseURL = "https://open.feishu.cn"

type APIClient struct {
	cfg        *cfgpkg.Config
	httpClient *http.Client
	baseURL    string

	mu     sync.Mutex
	tokens map[string]tokenInfo
}

type textAPI interface {
	FetchBotOpenID(context.Context) (string, error)
	ReplyText(context.Context, replyRef, string) error
	CreateText(context.Context, string, string) error
}

type channelAPI struct {
	channelID string
	client    *APIClient
}

type tokenInfo struct {
	Token     string
	ExpiresAt time.Time
}

type apiResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type tokenResponse struct {
	Code           int    `json:"code"`
	Msg            string `json:"msg"`
	AppAccessToken string `json:"app_access_token"`
	Expire         int    `json:"expire"`
}

func NewAPIClient(cfg *cfgpkg.Config) *APIClient {
	return &APIClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    defaultBaseURL,
		tokens:     make(map[string]tokenInfo),
	}
}

func newChannelAPI(channelID string, client *APIClient) *channelAPI {
	return &channelAPI{channelID: channelID, client: client}
}

func (c *APIClient) GetAppAccessToken(ctx context.Context, channelID string) (string, error) {
	c.mu.Lock()
	cached, ok := c.tokens[channelID]
	if ok && cached.ExpiresAt.After(time.Now().Add(time.Minute)) {
		c.mu.Unlock()
		return cached.Token, nil
	}
	c.mu.Unlock()

	cfg, err := resolveDriverConfig(c.cfg, channelID)
	if err != nil {
		return "", err
	}

	var resp tokenResponse
	if err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/open-apis/auth/v3/app_access_token/internal", "", map[string]string{
		"app_id":     cfg.AppID,
		"app_secret": cfg.AppSecret,
	}, &resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.AppAccessToken) == "" {
		return "", fmt.Errorf("Feishu token 响应缺少 app_access_token")
	}
	expiresIn := resp.Expire
	if expiresIn <= 0 {
		expiresIn = 7200
	}

	c.mu.Lock()
	c.tokens[channelID] = tokenInfo{
		Token:     resp.AppAccessToken,
		ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
	c.mu.Unlock()
	return resp.AppAccessToken, nil
}

func (a *channelAPI) FetchBotOpenID(ctx context.Context) (string, error) {
	if a == nil || a.client == nil {
		return "", nil
	}
	return a.client.FetchBotOpenID(ctx, a.channelID)
}

func (a *channelAPI) ReplyText(ctx context.Context, ref replyRef, content string) error {
	return a.client.ReplyMessage(ctx, a.channelID, ref, content)
}

func (a *channelAPI) CreateText(ctx context.Context, chatID, content string) error {
	return a.client.CreateMessage(ctx, a.channelID, chatID, content)
}

func (c *APIClient) FetchBotOpenID(ctx context.Context, channelID string) (string, error) {
	token, err := c.GetAppAccessToken(ctx, channelID)
	if err != nil {
		return "", err
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OpenID    string `json:"open_id"`
			BotOpenID string `json:"bot_open_id"`
		} `json:"data"`
	}
	if err := c.doJSON(ctx, http.MethodGet, c.baseURL+"/open-apis/bot/v3/info", token, nil, &resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.Data.OpenID) != "" {
		return strings.TrimSpace(resp.Data.OpenID), nil
	}
	return strings.TrimSpace(resp.Data.BotOpenID), nil
}

func (c *APIClient) ReplyMessage(ctx context.Context, channelID string, ref replyRef, content string) error {
	token, err := c.GetAppAccessToken(ctx, channelID)
	if err != nil {
		return err
	}
	body := map[string]any{
		"msg_type": "text",
		"content":  buildTextContent(content),
	}
	if ref.RootID != "" {
		body["reply_in_thread"] = true
	}
	return c.doJSON(ctx, http.MethodPost, c.baseURL+"/open-apis/im/v1/messages/"+url.PathEscape(ref.MessageID)+"/reply", token, body, nil)
}

func (c *APIClient) CreateMessage(ctx context.Context, channelID, chatID, content string) error {
	token, err := c.GetAppAccessToken(ctx, channelID)
	if err != nil {
		return err
	}
	body := map[string]any{
		"receive_id": chatID,
		"msg_type":   "text",
		"content":    buildTextContent(content),
	}
	return c.doJSON(ctx, http.MethodPost, c.baseURL+"/open-apis/im/v1/messages?receive_id_type=chat_id", token, body, nil)
}

func (c *APIClient) doJSON(ctx context.Context, method, targetURL, token string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, targetURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Feishu API 请求失败（%d）：%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if len(raw) == 0 {
		return nil
	}

	var apiErr apiResponse
	if err := json.Unmarshal(raw, &apiErr); err == nil && apiErr.Code != 0 {
		return fmt.Errorf("Feishu API 请求失败（code=%d）：%s", apiErr.Code, apiErr.Msg)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func encodeTextContent(text string) string {
	data, _ := json.Marshal(map[string]string{"text": text})
	return string(data)
}

func buildTextContent(text string) string {
	return encodeTextContent(text)
}
