package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"astra/internal/messaging"
	"astra/internal/workers"
	"astra/pkg/config"
	"astra/pkg/logger"

	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logger.New(cfg.LogLevel))

	bus := messaging.New(cfg.RedisAddr)
	defer bus.Close()

	hostname, _ := os.Hostname()
	w := workers.New(hostname, bus)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go w.StartHeartbeat(ctx)

	slog.Info("execution worker started", "worker_id", w.ID, "hostname", hostname)

	bus.Consume(ctx, "astra:tasks:shard:0", "worker-group", w.ID.String(), func(msg redis.XMessage) error {
		slog.Info("processing task", "task_id", msg.Values["task_id"])
		return nil
	})
}
