package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"astra/internal/agentdocs"
	"astra/internal/events"
	"astra/internal/goaladmission"
	"astra/internal/goals"
	"astra/internal/messaging"
	"astra/internal/planner"
	"astra/internal/tasks"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type goalServer struct {
	db               *sql.DB
	rdb              *redis.Client
	docStore         *agentdocs.Store
	taskStore        *tasks.Store
	eventStore       *events.Store
	depEngine        *goals.Store
	bus              *messaging.Bus
	planner          *planner.Planner
	autoApprovePlans bool
}

func (s *goalServer) handleCreateGoal(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey != "" {
		if statusCode, body := getCachedGoalResponse(r.Context(), s.rdb, idempotencyKey); statusCode != 0 {
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
	if err := goaladmission.CheckBeforeNewGoal(r.Context(), s.db, s.rdb, agentID); err != nil {
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
		if err := s.db.QueryRowContext(ctx,
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

	initialStatus, err := goalInitialStatus(ctx, s.db, depIDs)
	if err != nil {
		slog.Error("determine initial goal status failed", "err", err)
		http.Error(w, `{"error":"status check failed"}`, http.StatusInternalServerError)
		return
	}

	goalID := uuid.New()
	depArrayLit := uuidSliceToArrayLiteral(depIDs)

	_, err = s.db.ExecContext(ctx,
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
		_ = s.docStore.CreateDocument(ctx, doc)
	}

	// If the goal is blocked, skip planning — the planner runs when the goal is activated.
	if initialStatus == "blocked" {
		resp := map[string]interface{}{
			"goal_id": goalID.String(),
			"status":  initialStatus,
		}
		if idempotencyKey != "" {
			setCachedGoalResponse(r.Context(), s.rdb, idempotencyKey, http.StatusCreated, resp)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
		return
	}

	phaseRunID := uuid.New()
	_, err = s.db.ExecContext(ctx,
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
	_, err = s.eventStore.Append(ctx, "PhaseStarted", phaseRunID.String(), phasePayload)
	if err != nil {
		slog.Warn("append PhaseStarted event failed", "err", err)
	}

	agentCtx, err := s.docStore.AssembleContext(ctx, agentID, &goalID)
	var agentCtxJSON json.RawMessage
	if err == nil && agentCtx != nil {
		agentCtxJSON, _ = agentdocs.SerializeContext(agentCtx)
	}

	planOpts := &planner.PlanOptions{Workspace: req.Workspace, AgentContext: agentCtxJSON}
	graph, err := s.planner.Plan(ctx, goalID, req.GoalText, agentID, planOpts)
	if err != nil {
		slog.Error("plan failed", "err", err)
		http.Error(w, `{"error":"plan failed"}`, http.StatusInternalServerError)
		return
	}

	if !s.autoApprovePlans && !req.AutoApprove {
		planPayload := buildPlanPayload(&graph, goalID, agentID, req.GoalText)
		planPayloadJSON, err := json.Marshal(planPayload)
		if err != nil {
			slog.Error("marshal plan_payload failed", "err", err)
			http.Error(w, `{"error":"serialize plan failed"}`, http.StatusInternalServerError)
			return
		}
		approvalID := uuid.New()
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO approval_requests (id, request_type, goal_id, graph_id, plan_payload, status, requested_by)
		 VALUES ($1, 'plan', $2, $3, $4, 'pending', $5)`,
			approvalID, goalID, graph.ID, planPayloadJSON, userVal)
		if err != nil {
			slog.Error("insert plan approval failed", "err", err)
			http.Error(w, `{"error":"create approval request failed"}`, http.StatusInternalServerError)
			return
		}

		assignApprovalToAdmin(ctx, s.db, approvalID, agentID)

		resp202 := map[string]interface{}{
			"goal_id":             goalID.String(),
			"approval_request_id": approvalID.String(),
			"message":             "Plan pending approval",
			"graph_id":            graph.ID.String(),
		}
		if idempotencyKey != "" {
			setCachedGoalResponse(r.Context(), s.rdb, idempotencyKey, http.StatusAccepted, resp202)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(resp202)
		return
	}

	if err := s.taskStore.CreateGraph(ctx, &graph); err != nil {
		slog.Error("CreateGraph failed", "err", err)
		http.Error(w, `{"error":"create graph failed"}`, http.StatusInternalServerError)
		return
	}

	_, err = s.db.ExecContext(ctx,
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
		setCachedGoalResponse(r.Context(), s.rdb, idempotencyKey, http.StatusCreated, resp)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (s *goalServer) handleGetGoal(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	var goal struct {
		ID               string   `json:"id"`
		AgentID          string   `json:"agent_id"`
		GoalText         string   `json:"goal_text"`
		Priority         int      `json:"priority"`
		Status           string   `json:"status"`
		CreatedAt        string   `json:"created_at"`
		CascadeID        *string  `json:"cascade_id,omitempty"`
		SourceAgentID    *string  `json:"source_agent_id,omitempty"`
		CompletedAt      *string  `json:"completed_at,omitempty"`
		DependsOnGoalIDs []string `json:"depends_on_goal_ids,omitempty"`
	}
	var cascadeID sql.NullString
	var sourceAgentID sql.NullString
	var completedAt sql.NullString
	var depArrayLit sql.NullString

	err = s.db.QueryRowContext(r.Context(),
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
}

func (s *goalServer) handleGetGoalDetails(w http.ResponseWriter, r *http.Request) {
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
	err = s.db.QueryRowContext(ctx,
		`SELECT id, agent_id, goal_text, priority, status, created_at::text FROM goals WHERE id = $1`,
		id).Scan(&goal.ID, &goal.AgentID, &goal.GoalText, &goal.Priority, &goal.Status, &goal.CreatedAt)
	if err != nil {
		slog.Error("get goal details failed", "id", idStr, "err", err)
		http.Error(w, `{"error":"goal not found"}`, http.StatusNotFound)
		return
	}
	taskList, err := s.taskStore.ListTasksByGoalID(ctx, idStr)
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
}

func (s *goalServer) handleListGoals(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		http.Error(w, `{"error":"agent_id query required"}`, http.StatusBadRequest)
		return
	}
	if _, err := uuid.Parse(agentID); err != nil {
		http.Error(w, `{"error":"invalid agent_id"}`, http.StatusBadRequest)
		return
	}
	rows, err := s.db.QueryContext(r.Context(),
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
}

func (s *goalServer) handleFinalizeGoal(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	goalID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	rows, err := s.db.QueryContext(ctx,
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
	err = s.db.QueryRowContext(ctx,
		`SELECT id FROM phase_runs WHERE goal_id = $1 AND status = 'running' ORDER BY started_at DESC LIMIT 1`,
		goalID).Scan(&phaseRunID)
	if err != nil {
		slog.Error("find phase_run failed", "err", err)
		http.Error(w, `{"error":"phase_run not found"}`, http.StatusNotFound)
		return
	}

	_, err = s.db.ExecContext(ctx,
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
	_, err = s.eventStore.Append(ctx, eventType, phaseRunID.String(), payload)
	if err != nil {
		slog.Warn("append event failed", "err", err)
	}

	var cascadeIDStr string
	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(cascade_id::text, '') FROM goals WHERE id = $1`, goalID).Scan(&cascadeIDStr)

	var cascadeUUID uuid.UUID
	if cascadeIDStr != "" {
		cascadeUUID, _ = uuid.Parse(cascadeIDStr)
	}
	if err := s.depEngine.PublishGoalCompleted(ctx, goalID, cascadeUUID, phaseStatus, summary); err != nil {
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
}

func (s *goalServer) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats := map[string]any{}

	var totalGoals, activeGoals, completedGoals, failedGoals int
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM goals`).Scan(&totalGoals)
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM goals WHERE status = 'active'`).Scan(&activeGoals)
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM goals WHERE status = 'completed'`).Scan(&completedGoals)
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM goals WHERE status = 'failed'`).Scan(&failedGoals)
	stats["goals"] = map[string]int{
		"total": totalGoals, "active": activeGoals,
		"completed": completedGoals, "failed": failedGoals,
		"pending": totalGoals - activeGoals - completedGoals - failedGoals,
	}

	taskRows, _ := s.db.QueryContext(ctx, `SELECT status, count(*) FROM tasks GROUP BY status`)
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
	recentRows, _ := s.db.QueryContext(ctx,
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
}

func (s *goalServer) handleApplyPlan(w http.ResponseWriter, r *http.Request) {
	handleApplyPlan(w, r, s.db, s.taskStore)
}

func (s *goalServer) handleCreateInternalGoal(w http.ResponseWriter, r *http.Request) {
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
	count, err := s.rdb.Incr(ctx, rateKey).Result()
	if err != nil {
		slog.Error("rate limit check failed", "source_agent_id", sourceAgentID, "err", err)
		http.Error(w, `{"error":"rate limit check failed"}`, http.StatusInternalServerError)
		return
	}
	if count == 1 {
		s.rdb.Expire(ctx, rateKey, time.Minute)
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
	for _, depStr := range req.DependsOnGoalIDs {
		parsed, err := uuid.Parse(depStr)
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
		if err := s.db.QueryRowContext(ctx,
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

	initialStatus, err := goalInitialStatus(ctx, s.db, depIDs)
	if err != nil {
		slog.Error("determine initial goal status failed", "err", err)
		http.Error(w, `{"error":"status check failed"}`, http.StatusInternalServerError)
		return
	}

	goalID := uuid.New()
	depArrayLit := uuidSliceToArrayLiteral(depIDs)

	_, err = s.db.ExecContext(ctx,
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
		"goal_id": goalID.String(),
		"status":  initialStatus,
	})
}
