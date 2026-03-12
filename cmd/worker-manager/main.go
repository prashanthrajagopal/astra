package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"astra/internal/messaging"
	"astra/internal/tasks"
	"astra/internal/workers"
	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/httpx"
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

	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		slog.Error("failed to connect to redis", "err", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	registry := workers.NewRegistry(database)
	taskStore := tasks.NewStore(database)
	bus := messaging.New(cfg.RedisAddr)
	defer bus.Close()

	port := 8082
	if p := os.Getenv("WORKER_MANAGER_PORT"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil {
			port = parsed
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stale, err := registry.FindStaleWorkers(ctx, 30*time.Second)
				if err != nil {
					slog.Error("find stale workers failed", "err", err)
					continue
				}
				for _, workerID := range stale {
					if err := registry.MarkOffline(ctx, workerID); err != nil {
						slog.Error("mark offline failed", "worker_id", workerID, "err", err)
						continue
					}
					slog.Info("marked worker offline", "worker_id", workerID)
				}

				orphaned, err := taskStore.FindOrphanedRunningTasks(ctx)
				if err != nil {
					slog.Error("find orphaned tasks failed", "err", err)
				}
				for _, taskID := range orphaned {
					if err := taskStore.RequeueTask(ctx, taskID); err != nil {
						slog.Error("requeue task failed", "task_id", taskID, "err", err)
						continue
					}
					if err := bus.Publish(ctx, "astra:tasks:shard:0", map[string]interface{}{"task_id": taskID}); err != nil {
						slog.Error("republish requeued task failed", "task_id", taskID, "err", err)
						continue
					}
					slog.Info("requeued orphaned task", "task_id", taskID)
				}
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /workers", func(w http.ResponseWriter, r *http.Request) {
		var active []workers.WorkerInfo
		var err error
		if orgID := r.URL.Query().Get("org_id"); orgID != "" {
			active, err = registry.ListActiveByOrg(r.Context(), orgID)
		} else {
			active, err = registry.ListActive(r.Context())
		}
		if err != nil {
			slog.Error("list active workers failed", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(active)
	})

	srv := &http.Server{Addr: ":" + strconv.Itoa(port), Handler: mux}
	go func() {
		slog.Info("worker manager listening", "port", port)
		if err := httpx.ListenAndServe(srv, cfg); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "err", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down worker manager")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}
