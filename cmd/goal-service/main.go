package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"astra/internal/agentdocs"
	"astra/internal/events"
	"astra/internal/llm"
	"astra/internal/planner"
	"astra/internal/tasks"
	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/httpx"
	"astra/pkg/logger"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// planPayloadSpec matches docs/approval-system-extension-spec.md §3.2.
type planPayloadSpec struct {
	GoalID       string              `json:"goal_id"`
	GraphID      string              `json:"graph_id"`
	AgentID      string              `json:"agent_id"`
	GoalText     string              `json:"goal_text"`
	Tasks        []planPayloadTask   `json:"tasks"`
	Dependencies []planPayloadDep    `json:"dependencies"`
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
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("POST /internal/apply-plan", func(w http.ResponseWriter, r *http.Request) {
		handleApplyPlan(w, r, database, taskStore)
	})

	mux.HandleFunc("POST /goals", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AgentID     string `json:"agent_id"`
			GoalText    string `json:"goal_text"`
			Priority    int    `json:"priority"`
			Workspace   string `json:"workspace"`
			AutoApprove bool   `json:"auto_approve"`
			OrgID       string `json:"org_id"`
			UserID      string `json:"user_id"`
			Documents   []struct {
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
		priority := req.Priority
		if priority <= 0 {
			priority = 100
		}

		var orgID uuid.NullUUID
		if req.OrgID != "" {
			parsed, err := uuid.Parse(req.OrgID)
			if err != nil {
				http.Error(w, `{"error":"invalid org_id"}`, http.StatusBadRequest)
				return
			}
			orgID = uuid.NullUUID{UUID: parsed, Valid: true}
		}
		var userID uuid.NullUUID
		if req.UserID != "" {
			parsed, err := uuid.Parse(req.UserID)
			if err != nil {
				http.Error(w, `{"error":"invalid user_id"}`, http.StatusBadRequest)
				return
			}
			userID = uuid.NullUUID{UUID: parsed, Valid: true}
		}

		ctx := r.Context()
		goalID := uuid.New()

		orgVal := sql.NullString{String: orgID.UUID.String(), Valid: orgID.Valid}
		userVal := sql.NullString{String: userID.UUID.String(), Valid: userID.Valid}
		_, err = database.ExecContext(ctx,
			`INSERT INTO goals (id, agent_id, goal_text, priority, status, org_id, user_id)
			 VALUES ($1, $2, $3, $4, 'pending', $5, $6)`,
			goalID, agentID, req.GoalText, priority, orgVal, userVal)
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
				`INSERT INTO approval_requests (id, request_type, goal_id, graph_id, plan_payload, status)
				 VALUES ($1, 'plan', $2, $3, $4, 'pending')`,
				approvalID, goalID, graph.ID, planPayloadJSON)
			if err != nil {
				slog.Error("insert plan approval failed", "err", err)
				http.Error(w, `{"error":"create approval request failed"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"goal_id":             goalID.String(),
				"approval_request_id": approvalID.String(),
				"message":             "Plan pending approval",
				"graph_id":            graph.ID.String(),
			})
			return
		}

		if err := taskStore.CreateGraph(ctx, &graph); err != nil {
			slog.Error("CreateGraph failed", "err", err)
			http.Error(w, `{"error":"create graph failed"}`, http.StatusInternalServerError)
			return
		}

		if orgID.Valid {
			_, err = database.ExecContext(ctx,
				`UPDATE tasks SET org_id = $1 WHERE goal_id = $2 AND org_id IS NULL`,
				orgID.UUID, goalID)
			if err != nil {
				slog.Warn("propagate org_id to tasks failed", "goal_id", goalID, "err", err)
			}
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

	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		stats := map[string]any{}

		orgFilter := r.URL.Query().Get("org_id")
		var orgScoped bool
		var orgUUID uuid.UUID
		if orgFilter != "" {
			parsed, err := uuid.Parse(orgFilter)
			if err != nil {
				http.Error(w, `{"error":"invalid org_id"}`, http.StatusBadRequest)
				return
			}
			orgUUID = parsed
			orgScoped = true
		}

		var totalGoals, activeGoals, completedGoals, failedGoals int
		if orgScoped {
			database.QueryRowContext(ctx, `SELECT count(*) FROM goals WHERE org_id = $1`, orgUUID).Scan(&totalGoals)
			database.QueryRowContext(ctx, `SELECT count(*) FROM goals WHERE org_id = $1 AND status = 'active'`, orgUUID).Scan(&activeGoals)
			database.QueryRowContext(ctx, `SELECT count(*) FROM goals WHERE org_id = $1 AND status = 'completed'`, orgUUID).Scan(&completedGoals)
			database.QueryRowContext(ctx, `SELECT count(*) FROM goals WHERE org_id = $1 AND status = 'failed'`, orgUUID).Scan(&failedGoals)
		} else {
			database.QueryRowContext(ctx, `SELECT count(*) FROM goals`).Scan(&totalGoals)
			database.QueryRowContext(ctx, `SELECT count(*) FROM goals WHERE status = 'active'`).Scan(&activeGoals)
			database.QueryRowContext(ctx, `SELECT count(*) FROM goals WHERE status = 'completed'`).Scan(&completedGoals)
			database.QueryRowContext(ctx, `SELECT count(*) FROM goals WHERE status = 'failed'`).Scan(&failedGoals)
		}
		stats["goals"] = map[string]int{
			"total": totalGoals, "active": activeGoals,
			"completed": completedGoals, "failed": failedGoals,
			"pending": totalGoals - activeGoals - completedGoals - failedGoals,
		}

		var taskRows *sql.Rows
		if orgScoped {
			taskRows, _ = database.QueryContext(ctx, `SELECT status, count(*) FROM tasks WHERE org_id = $1 GROUP BY status`, orgUUID)
		} else {
			taskRows, _ = database.QueryContext(ctx, `SELECT status, count(*) FROM tasks GROUP BY status`)
		}
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
		var recentRows *sql.Rows
		if orgScoped {
			recentRows, _ = database.QueryContext(ctx,
				`SELECT id, agent_id, goal_text, status, created_at::text FROM goals WHERE org_id = $1 ORDER BY created_at DESC LIMIT 10`, orgUUID)
		} else {
			recentRows, _ = database.QueryContext(ctx,
				`SELECT id, agent_id, goal_text, status, created_at::text FROM goals ORDER BY created_at DESC LIMIT 10`)
		}
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
	srv.Shutdown(shutdownCtx)
	slog.Info("goal service stopped")
}
