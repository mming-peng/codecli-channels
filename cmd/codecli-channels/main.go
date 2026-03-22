package main

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

func main() {
	configPath := flag.String("config", "config/codecli-channels.json", "配置文件路径")
	flag.Parse()
	path := resolveConfigPath(*configPath)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := cfgpkg.Load(path)
	if err != nil {
		logger.Error("加载配置失败", "error", err)
		os.Exit(1)
	}
	service, err := bridge.NewService(cfg, logger)
	if err != nil {
		logger.Error("初始化服务失败", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := service.Start(ctx); err != nil {
		logger.Error("启动服务失败", "error", err)
		os.Exit(1)
	}
	logger.Info("codecli-channels 已启动", "config", path)
	<-ctx.Done()
	logger.Info("codecli-channels 已退出")
}

func resolveConfigPath(path string) string {
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
