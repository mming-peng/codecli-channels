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
	"net/url"
	"strings"
	"time"

	"codecli-channels/internal/onboarding"
)

func RunSetupFlow(ctx context.Context, opts SetupOptions) (*SetupResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	baseURL := normalizeBaseURL(opts.APIBaseURL)
	botType := strings.TrimSpace(opts.BotType)
	if botType == "" {
		botType = DefaultBotType
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	refreshes := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		qrResp, err := fetchBotQRCode(ctx, client, baseURL, strings.TrimSpace(opts.RouteTag), botType)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(qrResp.QRCode) == "" {
			return nil, fmt.Errorf("微信扫码初始化失败: qrcode 为空")
		}
		if strings.TrimSpace(qrResp.QRCodeImgContent) == "" {
			return nil, fmt.Errorf("微信扫码初始化失败: qrcode_img_content 为空")
		}
		renderQRCode(opts, qrResp.QRCodeImgContent)
		if opts.SaveQRCode != nil && strings.TrimSpace(opts.QRImagePath) != "" {
			if err := opts.SaveQRCode(qrResp.QRCodeImgContent, opts.QRImagePath); err != nil {
				return nil, err
			}
		}

		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			statusResp, err := getQRCodeStatus(ctx, client, baseURL, strings.TrimSpace(opts.RouteTag), qrResp.QRCode)
			if err != nil {
				return nil, err
			}
			switch strings.TrimSpace(statusResp.Status) {
			case "", "wait":
				sleep(defaultPollSleep)
			case "scaned":
				sleep(defaultPollSleep)
			case "expired":
				if refreshes >= maxQRRefreshes {
					return nil, fmt.Errorf("二维码多次过期，请重试")
				}
				refreshes++
				goto refresh
			case "confirmed":
				if strings.TrimSpace(statusResp.BotToken) == "" {
					return nil, fmt.Errorf("扫码成功但 bot_token 为空")
				}
				if strings.TrimSpace(statusResp.IlinkBotID) == "" {
					return nil, fmt.Errorf("扫码成功但 ilink_bot_id 为空")
				}
				return &SetupResult{
					BotToken:      strings.TrimSpace(statusResp.BotToken),
					IlinkBotID:    strings.TrimSpace(statusResp.IlinkBotID),
					BaseURL:       strings.TrimSpace(statusResp.BaseURL),
					IlinkUserID:   strings.TrimSpace(statusResp.IlinkUserID),
					QRCode:        strings.TrimSpace(qrResp.QRCode),
					QRCodeContent: strings.TrimSpace(qrResp.QRCodeImgContent),
				}, nil
			default:
				sleep(defaultPollSleep)
			}
		}
	refresh:
	}
}

func VerifyToken(ctx context.Context, opts VerifyTokenOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	verifyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(getUpdatesReq{
		GetUpdatesBuf: "",
		BaseInfo:      baseInfo{ChannelVersion: "codecli-channels-weixin-setup/1.0"},
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(verifyCtx, http.MethodPost, buildEndpoint(normalizeBaseURL(opts.APIBaseURL), "ilink/bot/getupdates"), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(opts.Token))
	req.Header.Set("X-WECHAT-UIN", randomWechatUIN())
	if strings.TrimSpace(opts.RouteTag) != "" {
		req.Header.Set("SKRouteTag", strings.TrimSpace(opts.RouteTag))
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("weixin verify token: http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var parsed getUpdatesResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("weixin verify token: decode response: %w", err)
	}
	// Business-level failures are reported via ret/errcode.
	if parsed.Ret != 0 || parsed.Errcode != 0 {
		return &BusinessError{Ret: parsed.Ret, Errcode: parsed.Errcode, Errmsg: parsed.Errmsg}
	}
	return nil
}

func fetchBotQRCode(ctx context.Context, client *http.Client, baseURL, routeTag, botType string) (*getBotQRCodeResp, error) {
	u, err := url.Parse(buildEndpoint(baseURL, "ilink/bot/get_bot_qrcode"))
	if err != nil {
		return nil, err
	}
	query := u.Query()
	query.Set("bot_type", botType)
	u.RawQuery = query.Encode()
	var resp getBotQRCodeResp
	if err := doJSONRequest(ctx, client, http.MethodGet, u.String(), routeTag, nil, &resp, nil); err != nil {
		return nil, err
	}
	if resp.Ret != 0 {
		return nil, &BusinessError{Ret: resp.Ret, Errcode: resp.Errcode, Errmsg: resp.Errmsg}
	}
	return &resp, nil
}

func getQRCodeStatus(ctx context.Context, client *http.Client, baseURL, routeTag, qrCode string) (*getQRCodeStatusResp, error) {
	u, err := url.Parse(buildEndpoint(baseURL, "ilink/bot/get_qrcode_status"))
	if err != nil {
		return nil, err
	}
	query := u.Query()
	query.Set("qrcode", qrCode)
	u.RawQuery = query.Encode()
	extraHeaders := map[string]string{"iLink-App-ClientVersion": "1"}
	var resp getQRCodeStatusResp
	if err := doJSONRequest(ctx, client, http.MethodGet, u.String(), routeTag, nil, &resp, extraHeaders); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return &getQRCodeStatusResp{Status: "wait"}, nil
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return &getQRCodeStatusResp{Status: "wait"}, nil
		}
		return nil, err
	}
	if resp.Ret != 0 {
		return nil, &BusinessError{Ret: resp.Ret, Errcode: resp.Errcode, Errmsg: resp.Errmsg}
	}
	return &resp, nil
}

func doJSONRequest(ctx context.Context, client *http.Client, method, fullURL, routeTag string, body []byte, out any, extraHeaders map[string]string) error {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(routeTag) != "" {
		req.Header.Set("SKRouteTag", strings.TrimSpace(routeTag))
	}
	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("weixin onboarding: http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("weixin onboarding: decode response: %w", err)
	}
	return nil
}

func renderQRCode(opts SetupOptions, content string) {
	if opts.PrintQRCode != nil {
		opts.PrintQRCode(content)
		return
	}
	if opts.PrintWriter != nil {
		onboarding.PrintTerminalQRCode(opts.PrintWriter, content)
	}
}

func normalizeBaseURL(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		trimmed = DefaultBaseURL
	}
	return strings.TrimRight(trimmed, "/")
}

func buildEndpoint(baseURL, endpoint string) string {
	return normalizeBaseURL(baseURL) + "/" + strings.TrimLeft(endpoint, "/")
}

func randomWechatUIN() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return base64.StdEncoding.EncodeToString([]byte("0000"))
	}
	value := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", value)))
}
