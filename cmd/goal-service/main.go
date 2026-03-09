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

	"astra/internal/events"
	"astra/internal/planner"
	"astra/internal/tasks"
	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/httpx"
	"astra/pkg/logger"

	"github.com/google/uuid"
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

	taskStore := tasks.NewStore(database)
	eventStore := events.NewStore(database)
	p := planner.New()

	port := cfg.GoalServicePort
	if port == 0 {
		port = 8088
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("POST /goals", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AgentID  string `json:"agent_id"`
			GoalText string `json:"goal_text"`
			Priority int    `json:"priority"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		agentID, err := uuid.Parse(req.AgentID)
		if err != nil {
			http.Error(w, `{"error":"invalid agent_id"}`, http.StatusBadRequest)
			return
		}
		if req.GoalText == "" {
			http.Error(w, `{"error":"goal_text required"}`, http.StatusBadRequest)
			return
		}
		priority := req.Priority
		if priority <= 0 {
			priority = 100
		}

		ctx := r.Context()
		goalID := uuid.New()

		_, err = database.ExecContext(ctx,
			`INSERT INTO goals (id, agent_id, goal_text, priority, status) VALUES ($1, $2, $3, $4, 'pending')`,
			goalID, agentID, req.GoalText, priority)
		if err != nil {
			slog.Error("insert goal failed", "err", err)
			http.Error(w, `{"error":"insert goal failed"}`, http.StatusInternalServerError)
			return
		}

		phaseRunID := uuid.New()
		_, err = database.ExecContext(ctx,
			`INSERT INTO phase_runs (id, goal_id, agent_id, status) VALUES ($1, $2, $3, 'running')`,
			phaseRunID, goalID, agentID)
		if err != nil {
			slog.Error("insert phase_run failed", "err", err)
			http.Error(w, `{"error":"insert phase_run failed"}`, http.StatusInternalServerError)
			return
		}

		phasePayload, _ := json.Marshal(map[string]interface{}{
			"goal_id":   goalID.String(),
			"phase_id":  phaseRunID.String(),
			"agent_id":  agentID.String(),
			"goal_text": req.GoalText,
		})
		_, err = eventStore.Append(ctx, "PhaseStarted", phaseRunID.String(), phasePayload)
		if err != nil {
			slog.Warn("append PhaseStarted event failed", "err", err)
		}

		graph, err := p.Plan(ctx, goalID, req.GoalText, agentID)
		if err != nil {
			slog.Error("plan failed", "err", err)
			http.Error(w, `{"error":"plan failed"}`, http.StatusInternalServerError)
			return
		}

		if err := taskStore.CreateGraph(ctx, &graph); err != nil {
			slog.Error("CreateGraph failed", "err", err)
			http.Error(w, `{"error":"create graph failed"}`, http.StatusInternalServerError)
			return
		}

		_, err = database.ExecContext(ctx,
			`UPDATE goals SET status = 'active' WHERE id = $1`, goalID)
		if err != nil {
			slog.Warn("update goal status failed", "err", err)
		}

		resp := map[string]interface{}{
			"goal_id":      goalID.String(),
			"phase_run_id": phaseRunID.String(),
			"task_count":   len(graph.Tasks),
			"graph_id":     graph.ID.String(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("GET /goals/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
			return
		}
		var goal struct {
			ID        string `json:"id"`
			AgentID   string `json:"agent_id"`
			GoalText  string `json:"goal_text"`
			Priority  int    `json:"priority"`
			Status    string `json:"status"`
			CreatedAt string `json:"created_at"`
		}
		err = database.QueryRowContext(r.Context(),
			`SELECT id, agent_id, goal_text, priority, status, created_at::text FROM goals WHERE id = $1`,
			id).Scan(&goal.ID, &goal.AgentID, &goal.GoalText, &goal.Priority, &goal.Status, &goal.CreatedAt)
		if err != nil {
			slog.Error("get goal failed", "id", idStr, "err", err)
			http.Error(w, `{"error":"goal not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(goal)
	})

	mux.HandleFunc("GET /goals", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.URL.Query().Get("agent_id")
		if agentID == "" {
			http.Error(w, `{"error":"agent_id query required"}`, http.StatusBadRequest)
			return
		}
		if _, err := uuid.Parse(agentID); err != nil {
			http.Error(w, `{"error":"invalid agent_id"}`, http.StatusBadRequest)
			return
		}
		rows, err := database.QueryContext(r.Context(),
			`SELECT id, agent_id, goal_text, priority, status, created_at::text FROM goals WHERE agent_id = $1 ORDER BY created_at DESC`,
			agentID)
		if err != nil {
			slog.Error("list goals failed", "err", err)
			http.Error(w, `{"error":"list failed"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var goals []map[string]interface{}
		for rows.Next() {
			var id, aID, text, status, createdAt string
			var priority int
			if err := rows.Scan(&id, &aID, &text, &priority, &status, &createdAt); err != nil {
				continue
			}
			goals = append(goals, map[string]interface{}{
				"id":         id,
				"agent_id":   aID,
				"goal_text":  text,
				"priority":   priority,
				"status":     status,
				"created_at": createdAt,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"goals": goals})
	})

	mux.HandleFunc("POST /goals/{id}/finalize", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		goalID, err := uuid.Parse(idStr)
		if err != nil {
			http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
			return
		}
		ctx := r.Context()

		rows, err := database.QueryContext(ctx,
			`SELECT status FROM tasks WHERE goal_id = $1`, goalID)
		if err != nil {
			slog.Error("query tasks failed", "err", err)
			http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var hasFailed bool
		var hasCompleted bool
		var total int
		for rows.Next() {
			var status string
			if err := rows.Scan(&status); err != nil {
				continue
			}
			total++
			if status == "failed" {
				hasFailed = true
			}
			if status == "completed" {
				hasCompleted = true
			}
		}

		phaseStatus := "completed"
		eventType := "PhaseCompleted"
		summary := "All tasks completed"
		if hasFailed {
			phaseStatus = "failed"
			eventType = "PhaseFailed"
			summary = "One or more tasks failed"
		} else if total == 0 {
			summary = "No tasks"
		} else if !hasCompleted {
			summary = "Tasks still in progress"
		}

		var phaseRunID uuid.UUID
		err = database.QueryRowContext(ctx,
			`SELECT id FROM phase_runs WHERE goal_id = $1 AND status = 'running' ORDER BY started_at DESC LIMIT 1`,
			goalID).Scan(&phaseRunID)
		if err != nil {
			slog.Error("find phase_run failed", "err", err)
			http.Error(w, `{"error":"phase_run not found"}`, http.StatusNotFound)
			return
		}

		_, err = database.ExecContext(ctx,
			`UPDATE phase_runs SET status = $1, ended_at = now(), summary = $2, updated_at = now() WHERE id = $3`,
			phaseStatus, summary, phaseRunID)
		if err != nil {
			slog.Error("update phase_run failed", "err", err)
			http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
			return
		}

		payload, _ := json.Marshal(map[string]interface{}{
			"goal_id":  goalID.String(),
			"phase_id": phaseRunID.String(),
			"status":   phaseStatus,
			"summary":  summary,
		})
		_, err = eventStore.Append(ctx, eventType, phaseRunID.String(), payload)
		if err != nil {
			slog.Warn("append event failed", "err", err)
		}

		resp := map[string]interface{}{
			"goal_id":      goalID.String(),
			"phase_run_id": phaseRunID.String(),
			"status":       phaseStatus,
			"summary":      summary,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	srv := &http.Server{Addr: ":" + strconv.Itoa(port), Handler: mux}
	go func() {
		slog.Info("goal service listening", "port", port)
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
	slog.Info("goal service stopped")
}
