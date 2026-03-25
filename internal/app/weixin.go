package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	cfgpkg "codecli-channels/internal/config"
	onboardingweixin "codecli-channels/internal/onboarding/weixin"
)

const defaultWeixinChannelAlias = "weixin-main"

func runWeixin(args []string, env commandEnv) error {
	if len(args) == 0 {
		printWeixinUsage(env.stdout)
		return nil
	}
	switch args[0] {
	case "setup":
		return runWeixinSetup(args[1:], env)
	case "bind":
		return runWeixinBind(args[1:], env)
	case "help", "-h", "--help":
		printWeixinUsage(env.stdout)
		return nil
	default:
		printWeixinUsage(env.stderr)
		return fmt.Errorf("未知 weixin 子命令: %s", args[0])
	}
}

func runWeixinSetup(args []string, env commandEnv) error {
	fs := flag.NewFlagSet("weixin setup", flag.ContinueOnError)
	fs.SetOutput(env.stderr)
	configPath := fs.String("config", "config/codecli-channels.json", "配置文件路径")
	channelAlias := fs.String("channel", defaultWeixinChannelAlias, "要写回的 channel alias")
	apiURL := fs.String("api-url", onboardingweixin.DefaultBaseURL, "ilink API 根地址")
	routeTag := fs.String("route-tag", "", "可选的 SKRouteTag")
	botType := fs.String("bot-type", onboardingweixin.DefaultBotType, "二维码 bot_type")
	timeout := fs.Duration("timeout", 8*time.Minute, "扫码等待超时")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := env.runWeixinSetup(context.Background(), onboardingweixin.SetupOptions{
		APIBaseURL:  strings.TrimSpace(*apiURL),
		RouteTag:    strings.TrimSpace(*routeTag),
		BotType:     strings.TrimSpace(*botType),
		Timeout:     *timeout,
		PrintWriter: env.stdout,
	})
	if err != nil {
		return err
	}
	baseURL := strings.TrimSpace(result.BaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(*apiURL)
	}
	if err := persistWeixinConfig(ResolveConfigPath(*configPath), *channelAlias, persistWeixinRequest{
		Token:         strings.TrimSpace(result.BotToken),
		BaseURL:       baseURL,
		RouteTag:      strings.TrimSpace(*routeTag),
		ScannedUserID: strings.TrimSpace(result.IlinkUserID),
	}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(env.stdout, "微信扫码绑定已写入 %s -> %s\n", *channelAlias, ResolveConfigPath(*configPath))
	return nil
}

func runWeixinBind(args []string, env commandEnv) error {
	fs := flag.NewFlagSet("weixin bind", flag.ContinueOnError)
	fs.SetOutput(env.stderr)
	configPath := fs.String("config", "config/codecli-channels.json", "配置文件路径")
	channelAlias := fs.String("channel", defaultWeixinChannelAlias, "要写回的 channel alias")
	token := fs.String("token", "", "已有的 ilink Bearer token")
	apiURL := fs.String("api-url", onboardingweixin.DefaultBaseURL, "ilink API 根地址")
	routeTag := fs.String("route-tag", "", "可选的 SKRouteTag")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*token) == "" {
		return fmt.Errorf("weixin bind 需要 -token")
	}
	if err := env.verifyWeixinToken(context.Background(), onboardingweixin.VerifyTokenOptions{
		APIBaseURL: strings.TrimSpace(*apiURL),
		RouteTag:   strings.TrimSpace(*routeTag),
		Token:      strings.TrimSpace(*token),
	}); err != nil {
		return err
	}
	if err := persistWeixinConfig(ResolveConfigPath(*configPath), *channelAlias, persistWeixinRequest{
		Token:    strings.TrimSpace(*token),
		BaseURL:  strings.TrimSpace(*apiURL),
		RouteTag: strings.TrimSpace(*routeTag),
	}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(env.stdout, "微信 token 已写入 %s -> %s\n", *channelAlias, ResolveConfigPath(*configPath))
	return nil
}

type persistWeixinRequest struct {
	Token         string
	BaseURL       string
	RouteTag      string
	ScannedUserID string
}

func persistWeixinConfig(path, alias string, req persistWeixinRequest) error {
	cfg, err := cfgpkg.LoadRaw(path)
	if err != nil {
		return err
	}
	enabled := true
	patch := cfgpkg.ChannelPatch{
		Type:    "weixin",
		Enabled: &enabled,
		Options: map[string]any{
			"token":   req.Token,
			"baseUrl": req.BaseURL,
		},
	}
	if strings.TrimSpace(req.RouteTag) != "" {
		patch.Options["routeTag"] = req.RouteTag
	}
	if strings.TrimSpace(req.ScannedUserID) != "" {
		patch.SetOptionsIfEmpty = map[string]any{
			"allowFrom": []string{req.ScannedUserID},
		}
	}
	if _, err := cfg.UpsertChannel(alias, patch); err != nil {
		return err
	}
	cfg.EnsureBridgeChannelID(alias)
	if strings.TrimSpace(req.ScannedUserID) != "" {
		cfg.EnsureAllowedScope(alias + ":dm:" + req.ScannedUserID)
	}
	return cfgpkg.SaveRaw(path, cfg)
}

func printWeixinUsage(w io.Writer) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  codecli-channels weixin setup [-config path] [-channel weixin-main] [-api-url url] [-route-tag tag] [-bot-type 3] [-timeout 8m]")
	_, _ = fmt.Fprintln(w, "  codecli-channels weixin bind -token <token> [-config path] [-channel weixin-main] [-api-url url] [-route-tag tag]")
}
