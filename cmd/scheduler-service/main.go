package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"astra/internal/messaging"
	"astra/internal/scheduler"
	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logger.New(cfg.LogLevel))

	database, err := db.Connect(cfg.PostgresDSN())
	if err != nil {
		slog.Error("db connection failed", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	bus := messaging.New(cfg.RedisAddr)
	defer bus.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	s := scheduler.New(database, bus)
	slog.Info("scheduler service started")
	if err := s.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("scheduler exited with error", "err", err)
		os.Exit(1)
	}
}
