package app

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"codecli-channels/internal/bridge"
	cfgpkg "codecli-channels/internal/config"
)

var flagErrHelp = flag.ErrHelp

func runService(args []string, env commandEnv) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(env.stderr)
	configPath := fs.String("config", "config/codecli-channels.json", "配置文件路径")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path := ResolveConfigPath(*configPath)

	logger := slog.New(slog.NewTextHandler(env.stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := cfgpkg.Load(path)
	if err != nil {
		return err
	}
	service, err := bridge.NewService(cfg, logger)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := service.Start(ctx); err != nil {
		return err
	}
	logger.Info("codecli-channels 已启动", "config", path)
	<-ctx.Done()
	logger.Info("codecli-channels 已退出")
	return nil
}

func ResolveConfigPath(path string) string {
	const (
		defaultPath = "config/codecli-channels.json"
		legacyPath  = "config/qqbot.json"
	)
	if path != defaultPath {
		return path
	}
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath
	}
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath
	}
	return defaultPath
}
