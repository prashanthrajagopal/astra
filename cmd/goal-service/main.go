package main

import (
	"context"
	"database/sql"
	"encoding/json"
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

	gsrv := &goalServer{
		db:               database,
		rdb:              rdb,
		docStore:         docStore,
		taskStore:        taskStore,
		eventStore:       eventStore,
		depEngine:        depEngine,
		bus:              bus,
		planner:          p,
		autoApprovePlans: os.Getenv("AUTO_APPROVE_PLANS") == "true",
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /ready", health.ReadyHandler(database, rdb))

	mux.HandleFunc("POST /internal/apply-plan", gsrv.handleApplyPlan)
	mux.HandleFunc("POST /internal/goals", gsrv.handleCreateInternalGoal)
	mux.HandleFunc("POST /goals", gsrv.handleCreateGoal)
	mux.HandleFunc("GET /goals/{id}", gsrv.handleGetGoal)
	mux.HandleFunc("GET /goals/{id}/details", gsrv.handleGetGoalDetails)
	mux.HandleFunc("GET /goals", gsrv.handleListGoals)
	mux.HandleFunc("POST /goals/{id}/finalize", gsrv.handleFinalizeGoal)
	mux.HandleFunc("GET /stats", gsrv.handleStats)

	httpSrv := &http.Server{Addr: ":" + strconv.Itoa(port), Handler: mux}
	go func() {
		slog.Info("goal service listening", "port", port)
		if err := httpx.ListenAndServe(httpSrv, cfg); err != nil && err != http.ErrServerClosed {
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
	_ = httpSrv.Shutdown(shutdownCtx)
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
