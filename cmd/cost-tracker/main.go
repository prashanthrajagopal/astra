package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"astra/internal/cost"
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
		slog.Error("failed to connect db", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	agg := cost.NewAggregator(database)
	port := cfg.CostTrackerPort
	if port == 0 {
		port = 8090
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /cost/daily", func(w http.ResponseWriter, r *http.Request) {
		days := 7
		if v := r.URL.Query().Get("days"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				days = n
			}
		}
		rows, err := agg.DailyByAgentModel(r.Context(), time.Now().Add(-time.Duration(days)*24*time.Hour))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"rows": rows})
	})

	addr := fmt.Sprintf(":%d", port)
	slog.Info("cost-tracker started", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("cost-tracker failed", "err", err)
		os.Exit(1)
	}
}
