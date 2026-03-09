package main

import (
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
	TaskID        *string    `json:"task_id,omitempty"`
	WorkerID      *string    `json:"worker_id,omitempty"`
	ToolName      string     `json:"tool_name"`
	ActionSummary string     `json:"action_summary"`
	Status        string     `json:"status"`
	RequestedAt   time.Time  `json:"requested_at"`
	DecidedAt     *time.Time `json:"decided_at,omitempty"`
	DecidedBy     string     `json:"decided_by,omitempty"`
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

	mux := http.NewServeMux()
	srv := &server{db: dbConn}

	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("POST /check", srv.handleCheck)
	mux.HandleFunc("POST /approvals/{id}/approve", srv.handleApprove)
	mux.HandleFunc("POST /approvals/{id}/deny", srv.handleDeny)
	mux.HandleFunc("GET /approvals/pending", srv.handlePending)

	addr := fmt.Sprintf(":%d", cfg.AccessControlPort)
	slog.Info("access control service started", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("access control server failed", "err", err)
		os.Exit(1)
	}
}

type server struct {
	db *sql.DB
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *server) handlePending(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, task_id, worker_id, tool_name, action_summary, status, requested_at, decided_at, decided_by
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
		var taskID, workerID sql.NullString
		var decidedAt sql.NullTime
		var decidedBy sql.NullString
		err := rows.Scan(&ar.ID, &taskID, &workerID, &ar.ToolName, &ar.ActionSummary, &ar.Status, &ar.RequestedAt, &decidedAt, &decidedBy)
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
