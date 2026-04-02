package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"astra/internal/cost"
	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/httpx"
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
		slog.Error("failed to connect db", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	agg := cost.NewAggregator(database)
	port := cfg.CostTrackerPort
	if port == 0 {
		port = 8090
	}

	hdlr := &server{agg: agg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", hdlr.handleHealth)
	mux.HandleFunc("GET /cost/daily", hdlr.handleDailyByAgentModel)

	addr := fmt.Sprintf(":%d", port)
	slog.Info("cost-tracker started", "addr", addr)
	srv := &http.Server{Addr: addr, Handler: mux}
	if err := httpx.ListenAndServe(srv, cfg); err != nil {
		slog.Error("cost-tracker failed", "err", err)
		os.Exit(1)
	}
}
