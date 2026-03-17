// Command memory-janitor deletes expired memories and old chat messages per retention_days.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logger.New(cfg.LogLevel))
	database, err := db.Connect(cfg.PostgresDSN())
	if err != nil {
		slog.Error("db", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	run := func() {
		c, cancel2 := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel2()
		res, err := database.ExecContext(c, `DELETE FROM memories WHERE expires_at IS NOT NULL AND expires_at < now()`)
		if err != nil {
			slog.Error("delete expired memories", "err", err)
		} else {
			n, _ := res.RowsAffected()
			if n > 0 {
				slog.Info("deleted expired memories", "count", n)
			}
		}
		_, err = database.ExecContext(c, `
			DELETE FROM chat_messages WHERE session_id IN (
				SELECT id FROM chat_sessions
				WHERE retention_days IS NOT NULL AND retention_days > 0
				AND updated_at < now() - (retention_days || ' days')::interval
			)`)
		if err != nil {
			slog.Error("delete old chat messages", "err", err)
		}
		_, _ = database.ExecContext(c, `
			DELETE FROM chat_sessions
			WHERE retention_days IS NOT NULL AND retention_days > 0
			AND updated_at < now() - (retention_days || ' days')::interval
			AND id NOT IN (SELECT session_id FROM chat_messages)`)
	}

	if os.Getenv("MEMORY_JANITOR_ONCE") == "1" {
		run()
		return
	}
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
