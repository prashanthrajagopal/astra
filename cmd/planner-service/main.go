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

	"astra/internal/planner"
	"astra/pkg/config"
	"astra/pkg/httpx"
	"astra/pkg/logger"

	"github.com/google/uuid"
)

type planRequest struct {
	GoalID   string `json:"goal_id"`
	AgentID  string `json:"agent_id"`
	GoalText string `json:"goal_text"`
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logger.New(cfg.LogLevel))

	p := planner.New()

	port := cfg.PlannerPort
	if port == 0 {
		port = 8087
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /plan", func(w http.ResponseWriter, r *http.Request) {
		var req planRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		goalID, err := uuid.Parse(req.GoalID)
		if err != nil {
			http.Error(w, `{"error":"invalid goal_id"}`, http.StatusBadRequest)
			return
		}
		agentID, err := uuid.Parse(req.AgentID)
		if err != nil {
			http.Error(w, `{"error":"invalid agent_id"}`, http.StatusBadRequest)
			return
		}
		graph, err := p.Plan(r.Context(), goalID, req.GoalText, agentID)
		if err != nil {
			slog.Error("plan failed", "err", err)
			http.Error(w, `{"error":"plan failed"}`, http.StatusInternalServerError)
			return
		}

		tasksJSON := make([]map[string]interface{}, len(graph.Tasks))
		for i, t := range graph.Tasks {
			tasksJSON[i] = map[string]interface{}{
				"id":          t.ID.String(),
				"graph_id":    t.GraphID.String(),
				"goal_id":     t.GoalID.String(),
				"agent_id":    t.AgentID.String(),
				"type":        t.Type,
				"status":      string(t.Status),
				"priority":    t.Priority,
				"max_retries": t.MaxRetries,
			}
		}
		depsJSON := make([]map[string]interface{}, len(graph.Dependencies))
		for i, d := range graph.Dependencies {
			depsJSON[i] = map[string]interface{}{
				"task_id":    d.TaskID.String(),
				"depends_on": d.DependsOn.String(),
			}
		}
		resp := map[string]interface{}{
			"graph_id":     graph.ID.String(),
			"tasks":        tasksJSON,
			"dependencies": depsJSON,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	srv := &http.Server{Addr: ":" + strconv.Itoa(port), Handler: mux}
	go func() {
		slog.Info("planner service listening", "port", port)
		if err := httpx.ListenAndServe(srv, cfg); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "err", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	slog.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	slog.Info("planner service stopped")
}
