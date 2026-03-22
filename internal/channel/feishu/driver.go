package feishu

import (
	"context"
	"fmt"
	"log/slog"

	"codecli-channels/internal/channel"
	cfgpkg "codecli-channels/internal/config"
)

type Driver struct {
	id     string
	cfg    *cfgpkg.Config
	logger *slog.Logger
}

func Register(registry *channel.Registry, cfg *cfgpkg.Config, logger *slog.Logger) {
	if registry == nil {
		return
	}
	registry.Register("feishu", func(id string, channelCfg cfgpkg.ChannelConfig, dataDir string) (channel.Driver, error) {
		_ = channelCfg
		_ = dataDir
		return NewDriver(id, cfg, logger), nil
	})
}

func NewDriver(id string, cfg *cfgpkg.Config, logger *slog.Logger) *Driver {
	if logger == nil {
		logger = slog.Default()
	}
	return &Driver{id: id, cfg: cfg, logger: logger.With("channelId", id, "platform", "feishu")}
}

func (d *Driver) ID() string { return d.id }

func (d *Driver) Platform() string { return "feishu" }

func (d *Driver) Start(context.Context, channel.MessageSink) error {
	return fmt.Errorf("feishu driver 尚未实现协议接入，请先完成 SDK/长连接集成")
}

func (d *Driver) Reply(context.Context, any, string) error {
	return fmt.Errorf("feishu driver 尚未实现回复")
}

func (d *Driver) Send(context.Context, any, string) error {
	return fmt.Errorf("feishu driver 尚未实现主动发送")
}

func (d *Driver) Stop(context.Context) error { return nil }
