package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"qq-codex-go/internal/bridge"
	cfgpkg "qq-codex-go/internal/config"
)

func main() {
	configPath := flag.String("config", "config/qqbot.json", "配置文件路径")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := cfgpkg.Load(*configPath)
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
	logger.Info("qq-codex-go 已启动", "config", *configPath)
	<-ctx.Done()
	logger.Info("qq-codex-go 已退出")
}
