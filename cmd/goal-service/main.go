package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"astra/internal/agentdocs"
	"astra/internal/events"
	"astra/internal/goaladmission"
	"astra/internal/goals"
	"astra/internal/llm"
	"astra/internal/messaging"
	"astra/internal/planner"
	"astra/internal/tasks"
	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/health"
	"astra/pkg/httpx"
	"astra/pkg/logger"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// planPayloadSpec matches docs/approval-system-extension-spec.md §3.2.
type planPayloadSpec struct {
	GoalID       string            `json:"goal_id"`
	GraphID      string            `json:"graph_id"`
	AgentID      string            `json:"agent_id"`
	GoalText     string            `json:"goal_text"`
	Tasks        []planPayloadTask `json:"tasks"`
	Dependencies []planPayloadDep  `json:"dependencies"`
}

type planPayloadTask struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	Priority   int             `json:"priority"`
	MaxRetries int             `json:"max_retries"`
}

type planPayloadDep struct {
	TaskID    string `json:"task_id"`
	DependsOn string `json:"depends_on"`
}

func buildPlanPayload(graph *tasks.Graph, goalID, agentID uuid.UUID, goalText string) *planPayloadSpec {
	taskList := make([]planPayloadTask, len(graph.Tasks))
	for i := range graph.Tasks {
		t := &graph.Tasks[i]
		payload := t.Payload
		if payload == nil {
			payload = []byte("{}")
		}
		taskList[i] = planPayloadTask{
			ID:         t.ID.String(),
			Type:       t.Type,
			Payload:    payload,
			Priority:   t.Priority,
			MaxRetries: t.MaxRetries,
		}
	}
	deps := make([]planPayloadDep, len(graph.Dependencies))
	for i := range graph.Dependencies {
		deps[i] = planPayloadDep{
			TaskID:    graph.Dependencies[i].TaskID.String(),
			DependsOn: graph.Dependencies[i].DependsOn.String(),
		}
	}
	return &planPayloadSpec{
		GoalID:       goalID.String(),
		GraphID:      graph.ID.String(),
		AgentID:      agentID.String(),
		GoalText:     goalText,
		Tasks:        taskList,
		Dependencies: deps,
	}
}

func handleApplyPlan(w http.ResponseWriter, r *http.Request, db *sql.DB, taskStore *tasks.Store) {
	var req struct {
		ApprovalID string `json:"approval_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ApprovalID == "" {
		http.Error(w, `{"error":"approval_id required"}`, http.StatusBadRequest)
		return
	}
	approvalUUID, err := uuid.Parse(req.ApprovalID)
	if err != nil {
		http.Error(w, `{"error":"invalid approval_id"}`, http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var requestType, status string
	var planPayload []byte
	var goalID uuid.UUID
	err = db.QueryRowContext(ctx,
		`SELECT request_type, status, plan_payload, goal_id FROM approval_requests WHERE id = $1`,
		approvalUUID).Scan(&requestType, &status, &planPayload, &goalID)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"approval not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		slog.Error("apply-plan: load approval failed", "err", err)
		http.Error(w, `{"error":"load approval failed"}`, http.StatusInternalServerError)
		return
	}
	if requestType != "plan" {
		http.Error(w, `{"error":"not a plan approval"}`, http.StatusBadRequest)
		return
	}
	if status != "approved" {
		http.Error(w, `{"error":"approval not approved"}`, http.StatusBadRequest)
		return
	}
	if len(planPayload) == 0 {
		http.Error(w, `{"error":"missing plan_payload"}`, http.StatusBadRequest)
		return
	}
	var spec planPayloadSpec
	if err := json.Unmarshal(planPayload, &spec); err != nil {
		slog.Error("apply-plan: unmarshal plan_payload failed", "err", err)
		http.Error(w, `{"error":"invalid plan_payload"}`, http.StatusBadRequest)
		return
	}
	graphID, err := uuid.Parse(spec.GraphID)
	if err != nil {
		http.Error(w, `{"error":"invalid graph_id in payload"}`, http.StatusBadRequest)
		return
	}
	goalIDParsed, err := uuid.Parse(spec.GoalID)
	if err != nil {
		http.Error(w, `{"error":"invalid goal_id in payload"}`, http.StatusBadRequest)
		return
	}
	agentID, err := uuid.Parse(spec.AgentID)
	if err != nil {
		http.Error(w, `{"error":"invalid agent_id in payload"}`, http.StatusBadRequest)
		return
	}
	taskList := make([]tasks.Task, len(spec.Tasks))
	for i := range spec.Tasks {
		pt := &spec.Tasks[i]
		taskID, err := uuid.Parse(pt.ID)
		if err != nil {
			http.Error(w, `{"error":"invalid task id in payload"}`, http.StatusBadRequest)
			return
		}
		payload := pt.Payload
		if payload == nil {
			payload = []byte("{}")
		}
		taskList[i] = tasks.Task{
			ID:         taskID,
			GraphID:    graphID,
			GoalID:     goalIDParsed,
			AgentID:    agentID,
			Type:       pt.Type,
			Status:     tasks.StatusCreated,
			Payload:    payload,
			Priority:   pt.Priority,
			MaxRetries: pt.MaxRetries,
		}
	}
	var deps []tasks.Dependency
	for _, d := range spec.Dependencies {
		taskID, err1 := uuid.Parse(d.TaskID)
		dependsOn, err2 := uuid.Parse(d.DependsOn)
		if err1 != nil || err2 != nil {
			continue
		}
		deps = append(deps, tasks.Dependency{TaskID: taskID, DependsOn: dependsOn})
	}
	graph := tasks.Graph{ID: graphID, Tasks: taskList, Dependencies: deps}
	if err := taskStore.CreateGraph(ctx, &graph); err != nil {
		slog.Error("apply-plan: CreateGraph failed", "err", err)
		http.Error(w, `{"error":"create graph failed"}`, http.StatusInternalServerError)
		return
	}
	_, err = db.ExecContext(ctx, `UPDATE goals SET status = 'active' WHERE id = $1`, goalIDParsed)
	if err != nil {
		slog.Error("apply-plan: update goal status failed", "err", err)
		http.Error(w, `{"error":"update goal failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func assignApprovalToAdmin(ctx context.Context, database *sql.DB, approvalID, agentID uuid.UUID) {
	// Single-platform: no agent_admins or org_memberships; approval stays unassigned or caller sets assigned_to.
}

// goalInitialStatus returns 'blocked' if any dependency goal is not yet completed,
// otherwise returns 'pending'.
func goalInitialStatus(ctx context.Context, database *sql.DB, depIDs []uuid.UUID) (string, error) {
	if len(depIDs) == 0 {
		return "pending", nil
	}
	// Build array literal for use with ANY($1::uuid[])
	arrayLiteral := uuidSliceToArrayLiteral(depIDs)
	var unmetCount int
	err := database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM goals WHERE id = ANY($1::uuid[]) AND status != 'completed'`,
		arrayLiteral).Scan(&unmetCount)
	if err != nil {
		return "", fmt.Errorf("goalInitialStatus: %w", err)
	}
	if unmetCount > 0 {
		return "blocked", nil
	}
	return "pending", nil
}

// uuidSliceToArrayLiteral converts a []uuid.UUID to a PostgreSQL array literal string.
func uuidSliceToArrayLiteral(ids []uuid.UUID) string {
	if len(ids) == 0 {
		return "{}"
	}
	out := make([]byte, 0, 2+len(ids)*37)
	out = append(out, '{')
	for i, id := range ids {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, id.String()...)
	}
	out = append(out, '}')
	return string(out)
}

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

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer rdb.Close()

	bus := messaging.New(cfg.RedisAddr)
	defer bus.Close()

	depEngine := goals.NewStore(database, bus)

	docStore := agentdocs.NewStore(database, rdb)
	taskStore := tasks.NewStore(database)
	eventStore := events.NewStore(database)

	backend := llm.NewEndpointBackendFromEnv()
	mc := memcache.New(cfg.MemcachedAddr)
	router := llm.NewRouterWithCache(backend, mc, 86400)
	p := planner.NewWithRouter(router)

	port := cfg.GoalServicePort
	if port == 0 {
		port = 8088
	}

	mux := http.NewServeMux()
	autoApprovePlans := os.Getenv("AUTO_APPROVE_PLANS") == "true"

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /ready", health.ReadyHandler(database, rdb))

	mux.HandleFunc("POST /internal/apply-plan", func(w http.ResponseWriter, r *http.Request) {
		handleApplyPlan(w, r, database, taskStore)
	})

	// POST /internal/goals — agent-to-agent goal creation (service-to-service, no user JWT).
	mux.HandleFunc("POST /internal/goals", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AgentID          string   `json:"agent_id"`
			GoalText         string   `json:"goal_text"`
			Priority         int      `json:"priority"`
			SourceAgentID    string   `json:"source_agent_id"`
			CascadeID        string   `json:"cascade_id,omitempty"`
			DependsOnGoalIDs []string `json:"depends_on_goal_ids,omitempty"`
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
		sourceAgentID, err := uuid.Parse(req.SourceAgentID)
		if err != nil {
			http.Error(w, `{"error":"invalid source_agent_id"}`, http.StatusBadRequest)
			return
		}

		// Rate limiting: 100 goals/minute per source agent.
		ctx := r.Context()
		rateKey := fmt.Sprintf("agent:goals:rate:%s", sourceAgentID.String())
		count, err := rdb.Incr(ctx, rateKey).Result()
		if err != nil {
			slog.Error("rate limit check failed", "source_agent_id", sourceAgentID, "err", err)
			http.Error(w, `{"error":"rate limit check failed"}`, http.StatusInternalServerError)
			return
		}
		if count == 1 {
			rdb.Expire(ctx, rateKey, time.Minute)
		}
		if count > 100 {
			http.Error(w, `{"error":"rate_limit_exceeded","message":"100 goals/minute limit reached"}`, http.StatusTooManyRequests)
			return
		}

		priority := req.Priority
		if priority <= 0 {
			priority = 100
		}

		var cascadeVal sql.NullString
		if req.CascadeID != "" {
			parsed, err := uuid.Parse(req.CascadeID)
			if err != nil {
				http.Error(w, `{"error":"invalid cascade_id"}`, http.StatusBadRequest)
				return
			}
			cascadeVal = sql.NullString{String: parsed.String(), Valid: true}
		}

		depIDs := make([]uuid.UUID, 0, len(req.DependsOnGoalIDs))
		for _, s := range req.DependsOnGoalIDs {
			parsed, err := uuid.Parse(s)
			if err != nil {
				http.Error(w, `{"error":"invalid depends_on_goal_ids entry"}`, http.StatusBadRequest)
				return
			}
			depIDs = append(depIDs, parsed)
		}

		// Verify all referenced goals exist.
		if len(depIDs) > 0 {
			arrayLit := uuidSliceToArrayLiteral(depIDs)
			var foundCount int
			if err := database.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM goals WHERE id = ANY($1::uuid[])`, arrayLit).Scan(&foundCount); err != nil {
				slog.Error("check dep goals existence failed", "err", err)
				http.Error(w, `{"error":"dependency check failed"}`, http.StatusInternalServerError)
				return
			}
			if foundCount != len(depIDs) {
				http.Error(w, `{"error":"one or more depends_on_goal_ids not found"}`, http.StatusBadRequest)
				return
			}
		}

		initialStatus, err := goalInitialStatus(ctx, database, depIDs)
		if err != nil {
			slog.Error("determine initial goal status failed", "err", err)
			http.Error(w, `{"error":"status check failed"}`, http.StatusInternalServerError)
			return
		}

		goalID := uuid.New()
		depArrayLit := uuidSliceToArrayLiteral(depIDs)

		_, err = database.ExecContext(ctx,
			`INSERT INTO goals (id, agent_id, goal_text, priority, status, cascade_id, depends_on_goal_ids, source_agent_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7::uuid[], $8)`,
			goalID, agentID, req.GoalText, priority, initialStatus,
			cascadeVal, depArrayLit, sourceAgentID)
		if err != nil {
			slog.Error("insert internal goal failed", "err", err)
			http.Error(w, `{"error":"insert goal failed"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"goal_id":  goalID.String(),
			"status":   initialStatus,
		})
	})

	mux.HandleFunc("POST /goals", func(w http.ResponseWriter, r *http.Request) {
		idempotencyKey := r.Header.Get("Idempotency-Key")
		if idempotencyKey != "" {
			if statusCode, body := getCachedGoalResponse(r.Context(), rdb, idempotencyKey); statusCode != 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(statusCode)
				_, _ = w.Write([]byte(body))
				return
			}
		}

		var req struct {
			AgentID          string   `json:"agent_id"`
			GoalText         string   `json:"goal_text"`
			Priority         int      `json:"priority"`
			Workspace        string   `json:"workspace"`
			AutoApprove      bool     `json:"auto_approve"`
			UserID           string   `json:"user_id"`
			CascadeID        string   `json:"cascade_id,omitempty"`
			DependsOnGoalIDs []string `json:"depends_on_goal_ids,omitempty"`
			SourceAgentID    string   `json:"source_agent_id,omitempty"`
			Documents        []struct {
				DocType  string          `json:"doc_type"`
				Name     string          `json:"name"`
				Content  *string         `json:"content,omitempty"`
				URI      *string         `json:"uri,omitempty"`
				Metadata json.RawMessage `json:"metadata,omitempty"`
				Priority int             `json:"priority"`
			} `json:"documents,omitempty"`
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
		if err := goaladmission.CheckBeforeNewGoal(r.Context(), database, rdb, agentID); err != nil {
			switch {
			case errors.Is(err, goaladmission.ErrDrainMode):
				http.Error(w, `{"error":"agent_draining","message":"agent is draining; no new goals accepted"}`, http.StatusServiceUnavailable)
			case errors.Is(err, goaladmission.ErrConcurrentCap):
				http.Error(w, `{"error":"concurrent_goals_cap"}`, http.StatusTooManyRequests)
			case errors.Is(err, goaladmission.ErrTokenBudget):
				http.Error(w, `{"error":"daily_token_budget_exceeded"}`, http.StatusTooManyRequests)
			default:
				slog.Error("goal admission failed", "err", err)
				http.Error(w, `{"error":"admission failed"}`, http.StatusInternalServerError)
			}
			return
		}
		priority := req.Priority
		if priority <= 0 {
			priority = 100
		}

		var userVal sql.NullString
		if req.UserID != "" {
			parsed, err := uuid.Parse(req.UserID)
			if err != nil {
				http.Error(w, `{"error":"invalid user_id"}`, http.StatusBadRequest)
				return
			}
			userVal = sql.NullString{String: parsed.String(), Valid: true}
		}

		var cascadeVal sql.NullString
		if req.CascadeID != "" {
			parsed, err := uuid.Parse(req.CascadeID)
			if err != nil {
				http.Error(w, `{"error":"invalid cascade_id"}`, http.StatusBadRequest)
				return
			}
			cascadeVal = sql.NullString{String: parsed.String(), Valid: true}
		}

		var sourceAgentVal sql.NullString
		if req.SourceAgentID != "" {
			parsed, err := uuid.Parse(req.SourceAgentID)
			if err != nil {
				http.Error(w, `{"error":"invalid source_agent_id"}`, http.StatusBadRequest)
				return
			}
			sourceAgentVal = sql.NullString{String: parsed.String(), Valid: true}
		}

		depIDs := make([]uuid.UUID, 0, len(req.DependsOnGoalIDs))
		for _, s := range req.DependsOnGoalIDs {
			parsed, err := uuid.Parse(s)
			if err != nil {
				http.Error(w, `{"error":"invalid depends_on_goal_ids entry"}`, http.StatusBadRequest)
				return
			}
			depIDs = append(depIDs, parsed)
		}

		ctx := r.Context()

		// Verify all referenced goals exist.
		if len(depIDs) > 0 {
			arrayLit := uuidSliceToArrayLiteral(depIDs)
			var foundCount int
			if err := database.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM goals WHERE id = ANY($1::uuid[])`, arrayLit).Scan(&foundCount); err != nil {
				slog.Error("check dep goals existence failed", "err", err)
				http.Error(w, `{"error":"dependency check failed"}`, http.StatusInternalServerError)
				return
			}
			if foundCount != len(depIDs) {
				http.Error(w, `{"error":"one or more depends_on_goal_ids not found"}`, http.StatusBadRequest)
				return
			}
		}

		initialStatus, err := goalInitialStatus(ctx, database, depIDs)
		if err != nil {
			slog.Error("determine initial goal status failed", "err", err)
			http.Error(w, `{"error":"status check failed"}`, http.StatusInternalServerError)
			return
		}

		goalID := uuid.New()
		depArrayLit := uuidSliceToArrayLiteral(depIDs)

		_, err = database.ExecContext(ctx,
			`INSERT INTO goals (id, agent_id, goal_text, priority, status, cascade_id, depends_on_goal_ids, source_agent_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7::uuid[], $8)`,
			goalID, agentID, req.GoalText, priority, initialStatus,
			cascadeVal, depArrayLit, sourceAgentVal)
		if err != nil {
			slog.Error("insert goal failed", "err", err)
			http.Error(w, `{"error":"insert goal failed"}`, http.StatusInternalServerError)
			return
		}

		for _, d := range req.Documents {
			if d.DocType == "" || d.Name == "" || (d.Content == nil && d.URI == nil) {
				continue
			}
			dt := agentdocs.DocType(d.DocType)
			if dt != agentdocs.DocTypeRule && dt != agentdocs.DocTypeSkill && dt != agentdocs.DocTypeContextDoc && dt != agentdocs.DocTypeReference {
				continue
			}
			pri := d.Priority
			if pri == 0 {
				pri = 100
			}
			doc := &agentdocs.Document{
				AgentID:  agentID,
				GoalID:   &goalID,
				DocType:  dt,
				Name:     d.Name,
				Content:  d.Content,
				URI:      d.URI,
				Metadata: d.Metadata,
				Priority: pri,
			}
			_ = docStore.CreateDocument(ctx, doc)
		}

		// If the goal is blocked, skip planning — the planner runs when the goal is activated.
		if initialStatus == "blocked" {
			resp := map[string]interface{}{
				"goal_id": goalID.String(),
				"status":  initialStatus,
			}
			if idempotencyKey != "" {
				setCachedGoalResponse(r.Context(), rdb, idempotencyKey, http.StatusCreated, resp)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(resp)
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

		agentCtx, err := docStore.AssembleContext(ctx, agentID, &goalID)
		var agentCtxJSON json.RawMessage
		if err == nil && agentCtx != nil {
			agentCtxJSON, _ = agentdocs.SerializeContext(agentCtx)
		}

		planOpts := &planner.PlanOptions{Workspace: req.Workspace, AgentContext: agentCtxJSON}
		graph, err := p.Plan(ctx, goalID, req.GoalText, agentID, planOpts)
		if err != nil {
			slog.Error("plan failed", "err", err)
			http.Error(w, `{"error":"plan failed"}`, http.StatusInternalServerError)
			return
		}

		if !autoApprovePlans && !req.AutoApprove {
			planPayload := buildPlanPayload(&graph, goalID, agentID, req.GoalText)
			planPayloadJSON, err := json.Marshal(planPayload)
			if err != nil {
				slog.Error("marshal plan_payload failed", "err", err)
				http.Error(w, `{"error":"serialize plan failed"}`, http.StatusInternalServerError)
				return
			}
			approvalID := uuid.New()
			_, err = database.ExecContext(ctx,
				`INSERT INTO approval_requests (id, request_type, goal_id, graph_id, plan_payload, status, requested_by)
			 VALUES ($1, 'plan', $2, $3, $4, 'pending', $5)`,
				approvalID, goalID, graph.ID, planPayloadJSON, userVal)
			if err != nil {
				slog.Error("insert plan approval failed", "err", err)
				http.Error(w, `{"error":"create approval request failed"}`, http.StatusInternalServerError)
				return
			}

			assignApprovalToAdmin(ctx, database, approvalID, agentID)

			resp202 := map[string]interface{}{
				"goal_id":             goalID.String(),
				"approval_request_id": approvalID.String(),
				"message":             "Plan pending approval",
				"graph_id":            graph.ID.String(),
			}
			if idempotencyKey != "" {
				setCachedGoalResponse(r.Context(), rdb, idempotencyKey, http.StatusAccepted, resp202)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(resp202)
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
		if idempotencyKey != "" {
			setCachedGoalResponse(r.Context(), rdb, idempotencyKey, http.StatusCreated, resp)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
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
			ID               string  `json:"id"`
			AgentID          string  `json:"agent_id"`
			GoalText         string  `json:"goal_text"`
			Priority         int     `json:"priority"`
			Status           string  `json:"status"`
			CreatedAt        string  `json:"created_at"`
			CascadeID        *string `json:"cascade_id,omitempty"`
			SourceAgentID    *string `json:"source_agent_id,omitempty"`
			CompletedAt      *string `json:"completed_at,omitempty"`
			DependsOnGoalIDs []string `json:"depends_on_goal_ids,omitempty"`
		}
		var cascadeID sql.NullString
		var sourceAgentID sql.NullString
		var completedAt sql.NullString
		var depArrayLit sql.NullString

		err = database.QueryRowContext(r.Context(),
			`SELECT id, agent_id, goal_text, priority, status, created_at::text,
			        COALESCE(cascade_id::text, ''), COALESCE(source_agent_id::text, ''),
			        COALESCE(completed_at::text, ''), COALESCE(depends_on_goal_ids::text, '')
			 FROM goals WHERE id = $1`,
			id).Scan(&goal.ID, &goal.AgentID, &goal.GoalText, &goal.Priority, &goal.Status, &goal.CreatedAt,
			&cascadeID.String, &sourceAgentID.String, &completedAt.String, &depArrayLit.String)
		if err != nil {
			slog.Error("get goal failed", "id", idStr, "err", err)
			http.Error(w, `{"error":"goal not found"}`, http.StatusNotFound)
			return
		}

		if cascadeID.String != "" {
			s := cascadeID.String
			goal.CascadeID = &s
		}
		if sourceAgentID.String != "" {
			s := sourceAgentID.String
			goal.SourceAgentID = &s
		}
		if completedAt.String != "" {
			s := completedAt.String
			goal.CompletedAt = &s
		}
		if depArrayLit.String != "" && depArrayLit.String != "{}" {
			// Parse the PostgreSQL array literal "{uuid1,uuid2,...}"
			lit := depArrayLit.String
			if len(lit) >= 2 && lit[0] == '{' && lit[len(lit)-1] == '}' {
				inner := lit[1 : len(lit)-1]
				if inner != "" {
					var parsed []string
					start := 0
					for i := 0; i <= len(inner); i++ {
						if i == len(inner) || inner[i] == ',' {
							parsed = append(parsed, inner[start:i])
							start = i + 1
						}
					}
					goal.DependsOnGoalIDs = parsed
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(goal)
	})

	// GET /goals/{id}/details — full goal + tasks for dashboard modal (actions + failure logs).
	mux.HandleFunc("GET /goals/{id}/details", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
			return
		}
		ctx := r.Context()
		var goal struct {
			ID        string `json:"id"`
			AgentID   string `json:"agent_id"`
			GoalText  string `json:"goal_text"`
			Priority  int    `json:"priority"`
			Status    string `json:"status"`
			CreatedAt string `json:"created_at"`
		}
		err = database.QueryRowContext(ctx,
			`SELECT id, agent_id, goal_text, priority, status, created_at::text FROM goals WHERE id = $1`,
			id).Scan(&goal.ID, &goal.AgentID, &goal.GoalText, &goal.Priority, &goal.Status, &goal.CreatedAt)
		if err != nil {
			slog.Error("get goal details failed", "id", idStr, "err", err)
			http.Error(w, `{"error":"goal not found"}`, http.StatusNotFound)
			return
		}
		taskList, err := taskStore.ListTasksByGoalID(ctx, idStr)
		if err != nil {
			slog.Error("list tasks for goal failed", "goal_id", idStr, "err", err)
			http.Error(w, `{"error":"failed to load tasks"}`, http.StatusInternalServerError)
			return
		}
		tasksPayload := make([]map[string]interface{}, 0, len(taskList))
		for _, t := range taskList {
			payload := map[string]interface{}{
				"id":         t.ID.String(),
				"type":       string(t.Type),
				"status":     string(t.Status),
				"priority":   t.Priority,
				"created_at": t.CreatedAt.Format(time.RFC3339),
				"updated_at": t.UpdatedAt.Format(time.RFC3339),
			}
			if len(t.Payload) > 0 {
				payload["payload"] = json.RawMessage(t.Payload)
			}
			if len(t.Result) > 0 {
				payload["result"] = json.RawMessage(t.Result)
			}
			tasksPayload = append(tasksPayload, payload)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"goal":  goal,
			"tasks": tasksPayload,
		})
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
		var goalsList []map[string]interface{}
		for rows.Next() {
			var id, aID, text, status, createdAt string
			var priority int
			if err := rows.Scan(&id, &aID, &text, &priority, &status, &createdAt); err != nil {
				continue
			}
			goalsList = append(goalsList, map[string]interface{}{
				"id":         id,
				"agent_id":   aID,
				"goal_text":  text,
				"priority":   priority,
				"status":     status,
				"created_at": createdAt,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"goals": goalsList})
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

		// Fetch cascade_id for GoalCompleted event.
		var cascadeIDStr string
		_ = database.QueryRowContext(ctx,
			`SELECT COALESCE(cascade_id::text, '') FROM goals WHERE id = $1`, goalID).Scan(&cascadeIDStr)

		// Publish GoalCompleted event to Redis stream.
		var cascadeUUID uuid.UUID
		if cascadeIDStr != "" {
			cascadeUUID, _ = uuid.Parse(cascadeIDStr)
		}
		if err := depEngine.PublishGoalCompleted(ctx, goalID, cascadeUUID, phaseStatus, summary); err != nil {
			slog.Warn("publish GoalCompleted failed", "goal_id", goalID, "err", err)
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

	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		stats := map[string]any{}

		var totalGoals, activeGoals, completedGoals, failedGoals int
		_ = database.QueryRowContext(ctx, `SELECT count(*) FROM goals`).Scan(&totalGoals)
		_ = database.QueryRowContext(ctx, `SELECT count(*) FROM goals WHERE status = 'active'`).Scan(&activeGoals)
		_ = database.QueryRowContext(ctx, `SELECT count(*) FROM goals WHERE status = 'completed'`).Scan(&completedGoals)
		_ = database.QueryRowContext(ctx, `SELECT count(*) FROM goals WHERE status = 'failed'`).Scan(&failedGoals)
		stats["goals"] = map[string]int{
			"total": totalGoals, "active": activeGoals,
			"completed": completedGoals, "failed": failedGoals,
			"pending": totalGoals - activeGoals - completedGoals - failedGoals,
		}

		taskRows, _ := database.QueryContext(ctx, `SELECT status, count(*) FROM tasks GROUP BY status`)
		taskCounts := map[string]int{}
		if taskRows != nil {
			defer taskRows.Close()
			for taskRows.Next() {
				var status string
				var count int
				if taskRows.Scan(&status, &count) == nil {
					taskCounts[status] = count
				}
			}
		}
		stats["tasks"] = taskCounts

		var recentGoals []map[string]any
		recentRows, _ := database.QueryContext(ctx,
			`SELECT id, agent_id, goal_text, status, created_at::text FROM goals ORDER BY created_at DESC LIMIT 10`)
		if recentRows != nil {
			defer recentRows.Close()
			for recentRows.Next() {
				var id, agentID, text, status, createdAt string
				if recentRows.Scan(&id, &agentID, &text, &status, &createdAt) == nil {
					goalText := text
					if len(goalText) > 100 {
						goalText = goalText[:100] + "..."
					}
					recentGoals = append(recentGoals, map[string]any{
						"id": id, "agent_id": agentID, "goal_text": goalText,
						"status": status, "created_at": createdAt,
					})
				}
			}
		}
		stats["recent_goals"] = recentGoals

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
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
	_ = srv.Shutdown(shutdownCtx)
	slog.Info("goal service stopped")
}

const idempotencyKeyPrefix = "idempotency:goal:"
const idempotencyTTL = 24 * time.Hour

func getCachedGoalResponse(ctx context.Context, rdb *redis.Client, key string) (statusCode int, body string) {
	if rdb == nil || key == "" {
		return 0, ""
	}
	val, err := rdb.Get(ctx, idempotencyKeyPrefix+key).Result()
	if err != nil {
		return 0, ""
	}
	var cached struct {
		StatusCode int    `json:"status_code"`
		Body       string `json:"body"`
	}
	if json.Unmarshal([]byte(val), &cached) != nil || cached.StatusCode < 200 || cached.StatusCode >= 300 {
		return 0, ""
	}
	return cached.StatusCode, cached.Body
}

func setCachedGoalResponse(ctx context.Context, rdb *redis.Client, key string, statusCode int, resp interface{}) {
	if rdb == nil || key == "" {
		return
	}
	bodyBytes, err := json.Marshal(resp)
	if err != nil {
		return
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"status_code": statusCode,
		"body":        string(bodyBytes),
	})
	rdb.Set(ctx, idempotencyKeyPrefix+key, payload, idempotencyTTL)
}
