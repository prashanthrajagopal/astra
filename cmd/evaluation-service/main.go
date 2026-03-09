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

	"astra/internal/evaluation"
	"astra/pkg/config"
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

	eval := evaluation.NewDefault()

	port := cfg.EvaluationPort
	if port == 0 {
		port = 8089
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /evaluate", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TaskID   string `json:"task_id"`
			Result   string `json:"result"`
			Criteria string `json:"criteria"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if req.TaskID == "" {
			http.Error(w, `{"error":"task_id required"}`, http.StatusBadRequest)
			return
		}

		output := []byte(req.Result)
		res, err := eval.EvaluateWithCriteria(r.Context(), req.TaskID, output, req.Criteria)
		if err != nil {
			slog.Error("evaluate failed", "err", err)
			http.Error(w, `{"error":"evaluation failed"}`, http.StatusInternalServerError)
			return
		}

		passed := res.Result == evaluation.Pass
		resp := map[string]interface{}{
			"passed":   passed,
			"feedback": res.Notes,
			"score":    res.Score,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	srv := &http.Server{Addr: ":" + strconv.Itoa(port), Handler: mux}
	go func() {
		slog.Info("evaluation service listening", "port", port)
		if err := httpx.ListenAndServe(srv, cfg); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "err", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
	slog.Info("evaluation service stopped")
}
