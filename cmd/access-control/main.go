package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/health"
	"astra/pkg/httpx"
	"astra/pkg/logger"
)

var dangerousSubstrings = []string{"delete", "prod", "kubectl", "terraform"}

type checkReq struct {
	Subject      string  `json:"subject"`
	Action       string  `json:"action"`
	Resource     string  `json:"resource"`
	ToolName     string  `json:"tool_name"`
	IsSuperAdmin bool    `json:"is_super_admin"`
	TrustScore   float64 `json:"trust_score,omitempty"`
}

type checkResp struct {
	Allowed          bool   `json:"allowed"`
	ApprovalRequired bool   `json:"approval_required"`
	Reason           string `json:"reason"`
}

type approveReq struct {
	DecidedBy string `json:"decided_by"`
}

type approvalRequest struct {
	ID            string     `json:"id"`
	RequestType   string     `json:"request_type"`
	TaskID        *string    `json:"task_id,omitempty"`
	WorkerID      *string    `json:"worker_id,omitempty"`
	ToolName      string     `json:"tool_name"`
	ActionSummary string     `json:"action_summary"`
	GoalID        *string    `json:"goal_id,omitempty"`
	GraphID       *string    `json:"graph_id,omitempty"`
	Summary       string     `json:"summary,omitempty"`
	Status        string     `json:"status"`
	RequestedBy   *string    `json:"requested_by,omitempty"`
	AssignedTo    *string    `json:"assigned_to,omitempty"`
	RequestedAt   time.Time  `json:"requested_at"`
	DecidedAt     *time.Time `json:"decided_at,omitempty"`
	DecidedBy     string     `json:"decided_by,omitempty"`
	PlanPayload   []byte     `json:"plan_payload,omitempty"`
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logger.New(cfg.LogLevel))

	dbConn, err := db.Connect(cfg.PostgresDSN())
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer dbConn.Close()

	httpClient, err := httpx.NewClient(cfg, 5*time.Second)
	if err != nil {
		slog.Error("failed to create http client", "err", err)
		os.Exit(1)
	}

	goalServiceAddr := strings.TrimSuffix(cfg.GoalServiceAddr, "/")
	mux := http.NewServeMux()
	srv := &server{db: dbConn, goalServiceAddr: goalServiceAddr, client: httpClient}

	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /ready", health.ReadyHandler(dbConn, nil))
	mux.HandleFunc("POST /check", srv.handleCheck)
	mux.HandleFunc("POST /approvals/{id}/approve", srv.handleApprove)
	mux.HandleFunc("POST /approvals/{id}/deny", srv.handleDeny)
	mux.HandleFunc("POST /approvals/{id}/decide", srv.handleDecide)
	mux.HandleFunc("GET /approvals/pending", srv.handlePending)
	mux.HandleFunc("GET /approvals/{id}", srv.handleGetByID)

	addr := fmt.Sprintf(":%d", cfg.AccessControlPort)
	slog.Info("access control service started", "addr", addr)
	srvHTTP := &http.Server{Addr: addr, Handler: mux}
	if err := httpx.ListenAndServe(srvHTTP, cfg); err != nil {
		slog.Error("access control server failed", "err", err)
		os.Exit(1)
	}
}

type server struct {
	db              *sql.DB
	goalServiceAddr string
	client          *http.Client
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *server) handleCheck(w http.ResponseWriter, r *http.Request) {
	var req checkReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	resp := evaluatePolicy(req)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func evaluatePolicy(req checkReq) checkResp {
	action := strings.TrimSpace(strings.ToLower(req.Action))
	resource := strings.TrimSpace(strings.ToLower(req.Resource))
	toolName := strings.TrimSpace(strings.ToLower(req.ToolName))

	if resource == "/health" || resource == "health" || action == "health" {
		return checkResp{Allowed: true}
	}

	if req.IsSuperAdmin {
		if isExecutionDetailResource(resource) {
			return checkResp{Allowed: false, Reason: "super-admin cannot access execution details"}
		}
		return checkResp{Allowed: true}
	}

	// If trust score is provided and below threshold, require approval.
	if req.TrustScore > 0 && req.TrustScore < 0.3 {
		return checkResp{Allowed: false, ApprovalRequired: true, Reason: "low trust score requires approval"}
	}

	// Any authenticated user (subject set) can access; dangerous tools may require approval.
	if action == "tool.execute" && toolName != "" {
		for _, sub := range dangerousSubstrings {
			if strings.Contains(toolName, sub) {
				return checkResp{Allowed: false, ApprovalRequired: true, Reason: "dangerous tool requires approval"}
			}
		}
	}
	return checkResp{Allowed: true}
}

func isExecutionDetailResource(resource string) bool {
	executionPaths := []string{"/tasks/", "/graphs/", "/goals/"}
	for _, p := range executionPaths {
		if strings.Contains(resource, p) && (strings.Contains(resource, "/payload") || strings.Contains(resource, "/result") || strings.Contains(resource, "/details")) {
			return true
		}
	}
	return false
}

func (s *server) handleApprove(w http.ResponseWriter, r *http.Request) {
	s.handleApprovalAction(w, r, "approved")
}

func (s *server) handleDeny(w http.ResponseWriter, r *http.Request) {
	s.handleApprovalAction(w, r, "denied")
}

func (s *server) handleApprovalAction(w http.ResponseWriter, r *http.Request, status string) {
	idStr := r.PathValue("id")
	if _, err := uuid.Parse(idStr); err != nil {
		http.Error(w, "invalid approval id", http.StatusBadRequest)
		return
	}

	var req approveReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}

	callerUserID := r.Header.Get("X-User-Id")
	if callerUserID != "" {
		req.DecidedBy = callerUserID
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if callerUserID != "" && r.Header.Get("X-Is-Super-Admin") != "true" {
		var assignedTo sql.NullString
		err := s.db.QueryRowContext(ctx,
			`SELECT assigned_to FROM approval_requests WHERE id::text = $1`,
			idStr).Scan(&assignedTo)
		if err == sql.ErrNoRows {
			http.Error(w, "approval not found", http.StatusNotFound)
			return
		}
		if err != nil {
			slog.Error("load approval for auth check failed", "id", idStr, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !assignedTo.Valid || assignedTo.String != callerUserID {
			http.Error(w, "forbidden: not authorized to act on this approval", http.StatusForbidden)
			return
		}
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE approval_requests SET status=$1, decided_at=now(), decided_by=COALESCE(NULLIF($2,''), decided_by), updated_at=now()
		 WHERE id::text=$3 AND status='pending'`,
		status, req.DecidedBy, idStr)
	if err != nil {
		slog.Error("update approval failed", "id", idStr, "err", err)
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		http.Error(w, "approval not found or not pending", http.StatusNotFound)
		return
	}
	if status == "approved" && s.goalServiceAddr != "" {
		var requestType string
		err := s.db.QueryRowContext(ctx, `SELECT COALESCE(request_type, 'risky_task') FROM approval_requests WHERE id::text = $1`, idStr).Scan(&requestType)
		if err == nil && requestType == "plan" {
			applyBody, _ := json.Marshal(map[string]string{"approval_id": idStr})
			applyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.goalServiceAddr+"/internal/apply-plan", bytes.NewReader(applyBody))
			if err == nil {
				applyReq.Header.Set("Content-Type", "application/json")
				applyResp, err := s.client.Do(applyReq)
				if err != nil {
					slog.Error("apply-plan call failed", "err", err)
				} else {
					applyResp.Body.Close()
					if applyResp.StatusCode >= 400 {
						slog.Error("apply-plan returned error", "status", applyResp.StatusCode)
					}
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *server) handlePending(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	callerUserID := r.Header.Get("X-User-Id")

	baseCols := `SELECT id, COALESCE(request_type, 'risky_task'), task_id, worker_id, tool_name, action_summary,
		 goal_id, graph_id, LEFT(COALESCE(plan_payload->>'goal_text',''), 200), status,
		 requested_by, assigned_to, requested_at, decided_at, decided_by
		 FROM approval_requests`

	var rows *sql.Rows
	var err error
	if callerUserID != "" {
		rows, err = s.db.QueryContext(ctx,
			baseCols+` WHERE status='pending' AND (assigned_to::text = $1 OR assigned_to IS NULL)
			 ORDER BY requested_at ASC`,
			callerUserID)
	} else {
		rows, err = s.db.QueryContext(ctx,
			baseCols+` WHERE status='pending' ORDER BY requested_at ASC`)
	}
	if err != nil {
		slog.Error("list pending approvals failed", "err", err)
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := scanApprovalRows(rows)
	if err := rows.Err(); err != nil {
		slog.Error("rows iteration failed", "err", err)
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func scanApprovalRows(rows *sql.Rows) []approvalRequest {
	var list []approvalRequest
	for rows.Next() {
		var ar approvalRequest
		var taskID, workerID, goalID, graphID sql.NullString
		var toolName, actionSummary sql.NullString
		var requestedBy, assignedTo sql.NullString
		var decidedAt sql.NullTime
		var decidedBy sql.NullString
		var summary sql.NullString
		err := rows.Scan(&ar.ID, &ar.RequestType, &taskID, &workerID, &toolName, &actionSummary,
			&goalID, &graphID, &summary, &ar.Status,
			&requestedBy, &assignedTo, &ar.RequestedAt, &decidedAt, &decidedBy)
		if err != nil {
			slog.Error("scan approval row failed", "err", err)
			continue
		}
		if toolName.Valid {
			ar.ToolName = toolName.String
		}
		if actionSummary.Valid {
			ar.ActionSummary = actionSummary.String
		}
		if taskID.Valid {
			ar.TaskID = &taskID.String
		}
		if workerID.Valid {
			ar.WorkerID = &workerID.String
		}
		if goalID.Valid {
			ar.GoalID = &goalID.String
		}
		if graphID.Valid {
			ar.GraphID = &graphID.String
		}
		if requestedBy.Valid {
			ar.RequestedBy = &requestedBy.String
		}
		if assignedTo.Valid {
			ar.AssignedTo = &assignedTo.String
		}
		if summary.Valid {
			ar.Summary = summary.String
		}
		if decidedAt.Valid {
			ar.DecidedAt = &decidedAt.Time
		}
		if decidedBy.Valid {
			ar.DecidedBy = decidedBy.String
		}
		list = append(list, ar)
	}
	return list
}

func (s *server) handleDecide(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if _, err := uuid.Parse(idStr); err != nil {
		http.Error(w, "invalid approval id", http.StatusBadRequest)
		return
	}

	var req struct {
		Decision string `json:"decision"`
		UserID   string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Decision != "approved" && req.Decision != "denied" {
		http.Error(w, `{"error":"decision must be 'approved' or 'denied'"}`, http.StatusBadRequest)
		return
	}

	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		userID = req.UserID
	}
	if userID == "" {
		http.Error(w, `{"error":"user_id required"}`, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var status string
	var requiredApprovals int
	var approvalsJSON []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT status, COALESCE(required_approvals, 1), COALESCE(approvals, '[]'::jsonb)
		 FROM approval_requests WHERE id::text = $1`, idStr).Scan(&status, &requiredApprovals, &approvalsJSON)
	if err == sql.ErrNoRows {
		http.Error(w, "approval not found", http.StatusNotFound)
		return
	}
	if err != nil {
		slog.Error("load approval failed", "id", idStr, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if status != "pending" {
		http.Error(w, `{"error":"approval already decided"}`, http.StatusConflict)
		return
	}

	var existingApprovals []map[string]interface{}
	_ = json.Unmarshal(approvalsJSON, &existingApprovals)

	for _, a := range existingApprovals {
		if uid, ok := a["user_id"].(string); ok && uid == userID {
			http.Error(w, `{"error":"user already voted"}`, http.StatusConflict)
			return
		}
	}

	newApproval := map[string]interface{}{
		"user_id":   userID,
		"decision":  req.Decision,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	existingApprovals = append(existingApprovals, newApproval)
	updatedJSON, _ := json.Marshal(existingApprovals)

	newStatus := "pending"
	approvedCount := 0
	for _, a := range existingApprovals {
		if d, ok := a["decision"].(string); ok {
			if d == "denied" {
				newStatus = "denied"
				break
			}
			if d == "approved" {
				approvedCount++
			}
		}
	}
	if newStatus != "denied" && approvedCount >= requiredApprovals {
		newStatus = "approved"
	}

	if newStatus != "pending" {
		_, err = s.db.ExecContext(ctx,
			`UPDATE approval_requests SET approvals = $1::jsonb, status = $2, decided_at = now(), decided_by = $3, updated_at = now()
			 WHERE id::text = $4`,
			updatedJSON, newStatus, userID, idStr)
	} else {
		_, err = s.db.ExecContext(ctx,
			`UPDATE approval_requests SET approvals = $1::jsonb, updated_at = now()
			 WHERE id::text = $2`,
			updatedJSON, idStr)
	}
	if err != nil {
		slog.Error("update approval failed", "id", idStr, "err", err)
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}

	if newStatus == "approved" && s.goalServiceAddr != "" {
		var requestType string
		_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(request_type, 'risky_task') FROM approval_requests WHERE id::text = $1`, idStr).Scan(&requestType)
		if requestType == "plan" {
			applyBody, _ := json.Marshal(map[string]string{"approval_id": idStr})
			applyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.goalServiceAddr+"/internal/apply-plan", bytes.NewReader(applyBody))
			if err == nil {
				applyReq.Header.Set("Content-Type", "application/json")
				resp, err := s.client.Do(applyReq)
				if err != nil {
					slog.Error("apply-plan call failed", "err", err)
				} else {
					resp.Body.Close()
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":             newStatus,
		"approvals_received": len(existingApprovals),
		"approvals_required": requiredApprovals,
	})
}

func (s *server) handleGetByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if _, err := uuid.Parse(idStr); err != nil {
		http.Error(w, "invalid approval id", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var ar approvalRequest
	var taskID, workerID, goalID, graphID sql.NullString
	var toolName, actionSummary sql.NullString
	var requestedBy, assignedTo sql.NullString
	var decidedAt sql.NullTime
	var decidedBy sql.NullString
	var planPayload []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(request_type, 'risky_task'), task_id, worker_id, tool_name, action_summary,
		 goal_id, graph_id, status, requested_by, assigned_to, requested_at, decided_at, decided_by, plan_payload
		 FROM approval_requests WHERE id::text = $1`,
		idStr).Scan(&ar.ID, &ar.RequestType, &taskID, &workerID, &toolName, &actionSummary,
		&goalID, &graphID, &ar.Status, &requestedBy, &assignedTo, &ar.RequestedAt, &decidedAt, &decidedBy, &planPayload)
	if err == sql.ErrNoRows {
		http.Error(w, "approval not found", http.StatusNotFound)
		return
	}
	if err != nil {
		slog.Error("get approval by id failed", "id", idStr, "err", err)
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	if toolName.Valid {
		ar.ToolName = toolName.String
	}
	if actionSummary.Valid {
		ar.ActionSummary = actionSummary.String
	}
	if taskID.Valid {
		ar.TaskID = &taskID.String
	}
	if workerID.Valid {
		ar.WorkerID = &workerID.String
	}
	if goalID.Valid {
		ar.GoalID = &goalID.String
	}
	if graphID.Valid {
		ar.GraphID = &graphID.String
	}
	if requestedBy.Valid {
		ar.RequestedBy = &requestedBy.String
	}
	if assignedTo.Valid {
		ar.AssignedTo = &assignedTo.String
	}
	if decidedAt.Valid {
		ar.DecidedAt = &decidedAt.Time
	}
	if decidedBy.Valid {
		ar.DecidedBy = decidedBy.String
	}
	out := map[string]interface{}{
		"id":             ar.ID,
		"request_type":   ar.RequestType,
		"task_id":        ar.TaskID,
		"worker_id":      ar.WorkerID,
		"tool_name":      ar.ToolName,
		"action_summary": ar.ActionSummary,
		"goal_id":        ar.GoalID,
		"graph_id":       ar.GraphID,
		"status":         ar.Status,
		"requested_by":   ar.RequestedBy,
		"assigned_to":    ar.AssignedTo,
		"requested_at":   ar.RequestedAt,
		"decided_at":     ar.DecidedAt,
		"decided_by":     ar.DecidedBy,
	}
	if len(planPayload) > 0 {
		var payload interface{}
		if json.Unmarshal(planPayload, &payload) == nil {
			out["plan_payload"] = payload
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
