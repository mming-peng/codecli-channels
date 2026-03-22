package weixin

import (
	"context"
	"fmt"
	"log/slog"

	"codecli-channels/internal/channel"
	cfgpkg "codecli-channels/internal/config"
)

type Driver struct {
	id      string
	cfg     *cfgpkg.Config
	dataDir string
	state   *State
	logger  *slog.Logger
}

func Register(registry *channel.Registry, cfg *cfgpkg.Config, logger *slog.Logger) {
	if registry == nil {
		return
	}
	registry.Register("weixin", func(id string, channelCfg cfgpkg.ChannelConfig, dataDir string) (channel.Driver, error) {
		_ = channelCfg
		return NewDriver(id, cfg, dataDir, logger)
	})
}

func NewDriver(id string, cfg *cfgpkg.Config, dataDir string, logger *slog.Logger) (*Driver, error) {
	if logger == nil {
		logger = slog.Default()
	}
	state, err := NewState(dataDir)
	if err != nil {
		return nil, err
	}
	return &Driver{
		id:      id,
		cfg:     cfg,
		dataDir: dataDir,
		state:   state,
		logger:  logger.With("channelId", id, "platform", "weixin"),
	}, nil
}

func (d *Driver) ID() string { return d.id }

func (d *Driver) Platform() string { return "weixin" }

func (d *Driver) Start(context.Context, channel.MessageSink) error {
	return fmt.Errorf("weixin driver 尚未实现长轮询接入")
}

func (d *Driver) Reply(context.Context, any, string) error {
	return fmt.Errorf("weixin driver 尚未实现回复")
}

func (d *Driver) Send(context.Context, any, string) error {
	return fmt.Errorf("weixin driver 尚未实现主动发送")
}

func (d *Driver) Stop(context.Context) error { return nil }
