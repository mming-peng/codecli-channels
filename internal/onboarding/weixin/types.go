package weixin

import (
	"io"
	"net/http"
	"time"
)

const (
	DefaultBaseURL   = "https://ilinkai.weixin.qq.com"
	DefaultBotType   = "3"
	defaultTimeout   = 15 * time.Second
	defaultPollSleep = time.Second
	maxQRRefreshes   = 2
)

// SetupOptions configures the QR-code based onboarding flow.
type SetupOptions struct {
	APIBaseURL string
	RouteTag   string
	BotType    string

	// Timeout limits the entire setup flow; 0 means no extra timeout beyond ctx.
	Timeout time.Duration

	// Sleep is used between status polls. If nil, time.Sleep is used.
	Sleep func(time.Duration)

	// PrintQRCode is called with the QR content (typically qrcode_img_content).
	// If nil, the caller can still read the returned content via logs or implement a default elsewhere.
	PrintQRCode func(content string)

	// PrintWriter receives an ASCII QR when PrintQRCode is nil.
	PrintWriter io.Writer

	// SaveQRCode is an optional hook for persisting a representation of the QR code.
	// This worker only guarantees it's invoked; the default behavior is no-op when nil.
	SaveQRCode func(content, path string) error

	// QRImagePath is passed back to SaveQRCode when non-empty.
	QRImagePath string

	// HTTPClient is optional and mainly used by tests.
	HTTPClient *http.Client
}

type SetupResult struct {
	BotToken      string
	IlinkBotID    string
	BaseURL       string
	IlinkUserID   string
	QRCode        string
	QRCodeContent string
}

type VerifyTokenOptions struct {
	APIBaseURL string
	RouteTag   string
	Token      string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// BusinessError is returned when ilink returns a non-zero ret.
type BusinessError struct {
	Ret     int
	Errcode int
	Errmsg  string
}

func (e *BusinessError) Error() string {
	if e == nil {
		return ""
	}
	if e.Errmsg != "" {
		return e.Errmsg
	}
	return "weixin: business error"
}

type getBotQRCodeResp struct {
	Ret              int    `json:"ret"`
	Errcode          int    `json:"errcode"`
	Errmsg           string `json:"errmsg"`
	QRCode           string `json:"qrcode"`
	QRCodeImgContent string `json:"qrcode_img_content"`
}

type getQRCodeStatusResp struct {
	Ret         int    `json:"ret"`
	Errcode     int    `json:"errcode"`
	Errmsg      string `json:"errmsg"`
	Status      string `json:"status"`
	BotToken    string `json:"bot_token"`
	IlinkBotID  string `json:"ilink_bot_id"`
	BaseURL     string `json:"baseurl"`
	IlinkUserID string `json:"ilink_user_id"`
}

type getUpdatesReq struct {
	GetUpdatesBuf string   `json:"get_updates_buf"`
	BaseInfo      baseInfo `json:"base_info,omitempty"`
}

type getUpdatesResp struct {
	Ret     int    `json:"ret"`
	Errcode int    `json:"errcode"`
	Errmsg  string `json:"errmsg"`
}

type baseInfo struct {
	ChannelVersion string `json:"channel_version,omitempty"`
}
