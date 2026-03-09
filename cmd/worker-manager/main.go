package main

import (
	"log/slog"
	"os"

	"astra/pkg/config"
	"astra/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logger.New(cfg.LogLevel))
	slog.Info("worker manager started")
	select {}
}
