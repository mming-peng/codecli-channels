package qq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	cfgpkg "qq-codex-go/internal/config"
)

const (
	tokenURL = "https://bots.qq.com/app/getAppAccessToken"
	apiBase  = "https://api.sgroup.qq.com"
)

type IncomingMessage struct {
	AccountID string
	ChatType  string
	TargetID  string
	SenderID  string
	MessageID string
	Text      string
	Timestamp string
}

func (m IncomingMessage) ConversationKey() string {
	return m.AccountID + ":" + m.ChatType + ":" + m.TargetID
}

type APIClient struct {
	cfg        *cfgpkg.Config
	httpClient *http.Client
	mu         sync.Mutex
	tokens     map[string]tokenInfo
}

type tokenInfo struct {
	Token     string
	ExpiresAt time.Time
}

type tokenResponse struct {
	AccessToken string          `json:"access_token"`
	ExpiresIn   json.RawMessage `json:"expires_in"`
}

func NewAPIClient(cfg *cfgpkg.Config) *APIClient {
	return &APIClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tokens:     make(map[string]tokenInfo),
	}
}

func (c *APIClient) GetAccessToken(ctx context.Context, accountID string) (string, error) {
	account, err := c.cfg.ResolveAccount(accountID)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	cached, ok := c.tokens[accountID]
	if ok && cached.ExpiresAt.After(time.Now().Add(1*time.Minute)) {
		c.mu.Unlock()
		return cached.Token, nil
	}
	c.mu.Unlock()

	payload := map[string]string{
		"appId":        account.AppID,
		"clientSecret": account.ClientSecret,
	}
	var resp tokenResponse
	if err := c.doJSON(ctx, http.MethodPost, tokenURL, "", payload, &resp); err != nil {
		return "", err
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("QQ token 响应缺少 access_token")
	}
	expiresIn := parseExpiresIn(resp.ExpiresIn)
	if expiresIn <= 0 {
		expiresIn = 7200
	}
	c.mu.Lock()
	c.tokens[accountID] = tokenInfo{Token: resp.AccessToken, ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second)}
	c.mu.Unlock()
	return resp.AccessToken, nil
}

func (c *APIClient) GetGatewayURL(ctx context.Context, accountID string) (string, error) {
	token, err := c.GetAccessToken(ctx, accountID)
	if err != nil {
		return "", err
	}
	var resp struct {
		URL string `json:"url"`
	}
	if err := c.doJSON(ctx, http.MethodGet, apiBase+"/gateway", token, nil, &resp); err != nil {
		return "", err
	}
	if resp.URL == "" {
		return "", fmt.Errorf("QQ gateway 响应缺少 url")
	}
	return resp.URL, nil
}

func (c *APIClient) ReplyMessage(ctx context.Context, accountID, targetType, targetID, msgID, content string) error {
	token, err := c.GetAccessToken(ctx, accountID)
	if err != nil {
		return err
	}
	body := map[string]any{
		"content":  content,
		"msg_type": 0,
		"msg_id":   msgID,
		"msg_seq":  nextMsgSeq(msgID),
	}
	return c.doJSON(ctx, http.MethodPost, apiBase+messagePath(targetType, targetID), token, body, nil)
}

func (c *APIClient) ProactiveMessage(ctx context.Context, accountID, targetType, targetID, content string) error {
	token, err := c.GetAccessToken(ctx, accountID)
	if err != nil {
		return err
	}
	body := map[string]any{
		"content":  content,
		"msg_type": 0,
	}
	return c.doJSON(ctx, http.MethodPost, apiBase+messagePath(targetType, targetID), token, body, nil)
}

func (c *APIClient) doJSON(ctx context.Context, method, url, token string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "QQBot "+token)
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
		var parsed map[string]any
		_ = json.Unmarshal(raw, &parsed)
		message := string(raw)
		if v, ok := parsed["message"].(string); ok && v != "" {
			message = v
		} else if v, ok := parsed["msg"].(string); ok && v != "" {
			message = v
		}
		return fmt.Errorf("QQ API 请求失败（%d）：%s", resp.StatusCode, message)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func messagePath(targetType, targetID string) string {
	if targetType == "group" {
		return "/v2/groups/" + targetID + "/messages"
	}
	return "/v2/users/" + targetID + "/messages"
}

func nextMsgSeq(msgID string) int {
	sum := crc32.ChecksumIEEE([]byte(msgID))
	return int(sum%65535) + 1
}

func parseExpiresIn(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		return number
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		value, err := strconv.Atoi(text)
		if err == nil {
			return value
		}
	}
	return 0
}
