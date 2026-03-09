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
	"astra/internal/tools"
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
	registry := workers.NewRegistry(database)

	runtime := tools.NewNoopRuntime()
	hostname, _ := os.Hostname()
	w := workers.New(hostname, bus)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := registry.Register(ctx, w.ID.String(), hostname, []string{"browser"}); err != nil {
		slog.Error("failed to register worker", "err", err)
		os.Exit(1)
	}
	slog.Info("browser worker registered", "worker_id", w.ID, "hostname", hostname)

	go w.StartHeartbeat(ctx)
	go runRegistryHeartbeat(ctx, registry, w.ID.String())

	slog.Info("browser worker started", "worker_id", w.ID, "hostname", hostname)

	bus.Consume(ctx, "astra:tasks:browser", "browser-worker-group", w.ID.String(), func(msg redis.XMessage) error {
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

		runCtx := context.Background()

		if err := taskStore.Transition(runCtx, taskID, tasks.StatusQueued, tasks.StatusScheduled, nil); err != nil {
			slog.Error("transition queued->scheduled failed", "task_id", taskID, "err", err)
			return nil
		}
		if err := taskStore.SetWorkerID(runCtx, taskID, w.ID.String()); err != nil {
			slog.Error("set worker_id failed", "task_id", taskID, "err", err)
		}
		slog.Info("task scheduled", "task_id", taskID)

		if err := taskStore.Transition(runCtx, taskID, tasks.StatusScheduled, tasks.StatusRunning, nil); err != nil {
			slog.Error("transition scheduled->running failed", "task_id", taskID, "err", err)
			return nil
		}
		slog.Info("task running", "task_id", taskID)

		task, err := taskStore.GetTask(runCtx, taskID)
		if err != nil {
			slog.Error("get task failed", "task_id", taskID, "err", err)
			_ = taskStore.FailTask(runCtx, taskID, err.Error())
			return nil
		}
		if task == nil {
			slog.Warn("task not found", "task_id", taskID)
			_ = taskStore.FailTask(runCtx, taskID, "task not found")
			return nil
		}

		payload := task.Payload
		if payload == nil {
			payload = []byte("{}")
		}

		toolReq := tools.ToolRequest{
			Name:        "browser:" + task.Type,
			Input:       payload,
			Timeout:     30 * time.Second,
			MemoryLimit: 256 * 1024 * 1024,
			CPULimit:    1.0,
		}

		toolResult, err := runtime.Execute(runCtx, toolReq)
		if err != nil {
			slog.Error("tool execution failed", "task_id", taskID, "err", err)
			_ = taskStore.FailTask(runCtx, taskID, err.Error())
			return nil
		}

		if toolResult.ExitCode == 0 {
			result := toolResult.Output
			if result == nil {
				result = []byte("{}")
			}
			if err := taskStore.CompleteTask(runCtx, taskID, result); err != nil {
				slog.Error("complete task failed", "task_id", taskID, "err", err)
				return nil
			}
			slog.Info("task completed", "task_id", taskID)
		} else {
			errMsg := string(toolResult.Output)
			if errMsg == "" {
				errMsg = "tool exited with non-zero code"
			}
			if err := taskStore.FailTask(runCtx, taskID, errMsg); err != nil {
				slog.Error("fail task failed", "task_id", taskID, "err", err)
				return nil
			}
			slog.Info("task failed", "task_id", taskID, "error", errMsg)
		}
		return nil
	})

	_ = registry.MarkOffline(context.Background(), w.ID.String())
	slog.Info("browser worker stopped", "worker_id", w.ID)
}

func runRegistryHeartbeat(ctx context.Context, registry *workers.Registry, workerID string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := registry.UpdateHeartbeat(ctx, workerID); err != nil {
				slog.Error("registry heartbeat failed", "worker_id", workerID, "err", err)
			}
		}
	}
}
