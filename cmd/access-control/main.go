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
	"astra/pkg/httpx"
	"astra/pkg/logger"
)

// Policy MVP:
// - health allowed always
// - actions on /health allowed
// - if action is "tool.execute" and tool_name contains delete, prod, kubectl, terraform => approval_required=true, allowed=false
// - otherwise allowed=true

var dangerousSubstrings = []string{"delete", "prod", "kubectl", "terraform"}

type checkReq struct {
	Subject  string `json:"subject"`
	Action   string `json:"action"`
	Resource string `json:"resource"`
	ToolName string `json:"tool_name"`
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
	Summary       string     `json:"summary,omitempty"` // plan goal_text truncated for list
	Status        string     `json:"status"`
	RequestedAt   time.Time  `json:"requested_at"`
	DecidedAt     *time.Time `json:"decided_at,omitempty"`
	DecidedBy     string     `json:"decided_by,omitempty"`
	PlanPayload   []byte     `json:"plan_payload,omitempty"` // only in GET by id
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
	mux.HandleFunc("POST /check", srv.handleCheck)
	mux.HandleFunc("POST /approvals/{id}/approve", srv.handleApprove)
	mux.HandleFunc("POST /approvals/{id}/deny", srv.handleDeny)
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
	db                *sql.DB
	goalServiceAddr   string
	client            *http.Client
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
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
	if action == "tool.execute" && toolName != "" {
		for _, sub := range dangerousSubstrings {
			if strings.Contains(toolName, sub) {
				return checkResp{
					Allowed:          false,
					ApprovalRequired: true,
					Reason:           "dangerous tool requires approval",
				}
			}
		}
	}
	return checkResp{Allowed: true}
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
	json.NewDecoder(r.Body).Decode(&req)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

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

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(request_type, 'risky_task'), task_id, worker_id, tool_name, action_summary,
		 goal_id, graph_id, LEFT(COALESCE(plan_payload->>'goal_text',''), 200), status, requested_at, decided_at, decided_by
		 FROM approval_requests WHERE status='pending' ORDER BY requested_at ASC`)
	if err != nil {
		slog.Error("list pending approvals failed", "err", err)
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var list []approvalRequest
	for rows.Next() {
		var ar approvalRequest
		var taskID, workerID, goalID, graphID sql.NullString
		var decidedAt sql.NullTime
		var decidedBy sql.NullString
		var summary sql.NullString
		err := rows.Scan(&ar.ID, &ar.RequestType, &taskID, &workerID, &ar.ToolName, &ar.ActionSummary,
			&goalID, &graphID, &summary, &ar.Status, &ar.RequestedAt, &decidedAt, &decidedBy)
		if err != nil {
			slog.Error("scan approval row failed", "err", err)
			continue
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
	if err := rows.Err(); err != nil {
		slog.Error("rows iteration failed", "err", err)
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
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
	var decidedAt sql.NullTime
	var decidedBy sql.NullString
	var planPayload []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(request_type, 'risky_task'), task_id, worker_id, tool_name, action_summary,
		 goal_id, graph_id, status, requested_at, decided_at, decided_by, plan_payload
		 FROM approval_requests WHERE id::text = $1`,
		idStr).Scan(&ar.ID, &ar.RequestType, &taskID, &workerID, &ar.ToolName, &ar.ActionSummary,
		&goalID, &graphID, &ar.Status, &ar.RequestedAt, &decidedAt, &decidedBy, &planPayload)
	if err == sql.ErrNoRows {
		http.Error(w, "approval not found", http.StatusNotFound)
		return
	}
	if err != nil {
		slog.Error("get approval by id failed", "id", idStr, "err", err)
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
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
	if decidedAt.Valid {
		ar.DecidedAt = &decidedAt.Time
	}
	if decidedBy.Valid {
		ar.DecidedBy = decidedBy.String
	}
	out := map[string]interface{}{
		"id":              ar.ID,
		"request_type":    ar.RequestType,
		"task_id":         ar.TaskID,
		"worker_id":       ar.WorkerID,
		"tool_name":       ar.ToolName,
		"action_summary":  ar.ActionSummary,
		"goal_id":         ar.GoalID,
		"graph_id":        ar.GraphID,
		"status":          ar.Status,
		"requested_at":    ar.RequestedAt,
		"decided_at":      ar.DecidedAt,
		"decided_by":      ar.DecidedBy,
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
