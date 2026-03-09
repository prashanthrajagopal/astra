package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"astra/internal/messaging"
	"astra/internal/tasks"
	"astra/internal/workers"
	"astra/pkg/config"
	"astra/pkg/db"
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

	database, err := db.Connect(cfg.PostgresDSN())
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	bus := messaging.New(cfg.RedisAddr)
	defer bus.Close()

	taskStore := tasks.NewStore(database)

	hostname, _ := os.Hostname()
	w := workers.New(hostname, bus)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go w.StartHeartbeat(ctx)

	slog.Info("execution worker started", "worker_id", w.ID, "hostname", hostname)

	bus.Consume(ctx, "astra:tasks:shard:0", "worker-group", w.ID.String(), func(msg redis.XMessage) error {
		taskIDVal, ok := msg.Values["task_id"]
		if !ok {
			slog.Warn("message missing task_id", "msg_id", msg.ID)
			return nil
		}
		taskID, ok := taskIDVal.(string)
		if !ok {
			slog.Warn("task_id is not string", "msg_id", msg.ID)
			return nil
		}
		if taskID == "" {
			slog.Warn("empty task_id", "msg_id", msg.ID)
			return nil
		}

		ctx := context.Background()

		if err := taskStore.Transition(ctx, taskID, tasks.StatusQueued, tasks.StatusScheduled, nil); err != nil {
			slog.Error("transition queued->scheduled failed", "task_id", taskID, "err", err)
			return nil
		}
		slog.Info("task scheduled", "task_id", taskID)

		if err := taskStore.Transition(ctx, taskID, tasks.StatusScheduled, tasks.StatusRunning, nil); err != nil {
			slog.Error("transition scheduled->running failed", "task_id", taskID, "err", err)
			return nil
		}
		slog.Info("task running", "task_id", taskID)

		time.Sleep(10 * time.Millisecond)

		if err := taskStore.CompleteTask(ctx, taskID, []byte("{}")); err != nil {
			slog.Error("complete task failed", "task_id", taskID, "err", err)
			return nil
		}
		slog.Info("task completed", "task_id", taskID)
		return nil
	})
}
