package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"astra/internal/tools"
	"astra/pkg/config"
	"astra/pkg/db"
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

	dbConn, err := db.Connect(cfg.PostgresDSN())
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer dbConn.Close()

	runtime := newToolRuntime()
	httpClient, err := httpx.NewClient(cfg, 100*time.Millisecond)
	if err != nil {
		slog.Error("failed to initialize HTTP client", "err", err)
		os.Exit(1)
	}
	gate := &approvalGate{
		accessControlAddr: strings.TrimSuffix(cfg.AccessControlAddr, "/"),
		db:                dbConn,
		client:            httpClient,
	}

	port := 8083
	if p := os.Getenv("TOOL_RUNTIME_PORT"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil {
			port = parsed
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /execute", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name           string  `json:"name"`
			Input          string  `json:"input"`
			TimeoutSeconds int     `json:"timeout_seconds"`
			MemoryLimit    int64   `json:"memory_limit"`
			CPULimit       float64 `json:"cpu_limit"`
			TaskID         string  `json:"task_id"`
			WorkerID       string  `json:"worker_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		var input []byte
		if req.Input != "" {
			var err error
			input, err = base64.StdEncoding.DecodeString(req.Input)
			if err != nil {
				http.Error(w, "invalid base64 input", http.StatusBadRequest)
				return
			}
		}
		timeout := 30 * time.Second
		if req.TimeoutSeconds > 0 {
			timeout = time.Duration(req.TimeoutSeconds) * time.Second
		}
		memLimit := int64(268435456)
		if req.MemoryLimit > 0 {
			memLimit = req.MemoryLimit
		}
		cpuLimit := 1.0
		if req.CPULimit > 0 {
			cpuLimit = req.CPULimit
		}

		toolReq := tools.ToolRequest{
			Name:        req.Name,
			Input:       input,
			Timeout:     timeout,
			MemoryLimit: memLimit,
			CPULimit:    cpuLimit,
		}

		// Approval gate: check before execute
		checkResp, err := gate.check(r.Context(), req.Name, req.TaskID, req.WorkerID)
		if err != nil {
			slog.Warn("approval gate check failed", "err", err)
			http.Error(w, "approval check failed", http.StatusInternalServerError)
			return
		}
		if checkResp.ApprovalRequired {
			arID, err := gate.insertPending(r.Context(), req.Name, req.TaskID, req.WorkerID)
			if err != nil {
				slog.Error("insert approval request failed", "err", err)
				http.Error(w, "failed to create approval request", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":              "pending_approval",
				"approval_request_id": arID,
			})
			return
		}
		if !checkResp.Allowed {
			http.Error(w, "forbidden: "+checkResp.Reason, http.StatusForbidden)
			return
		}

		toolResult, err := runtime.Execute(r.Context(), toolReq)
		if err != nil {
			slog.Error("execute failed", "name", req.Name, "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		outputB64 := base64.StdEncoding.EncodeToString(toolResult.Output)
		resp := map[string]interface{}{
			"output":      outputB64,
			"exit_code":   toolResult.ExitCode,
			"duration_ms": toolResult.Duration.Milliseconds(),
			"artifacts":   toolResult.Artifacts,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	srv := &http.Server{Addr: ":" + strconv.Itoa(port), Handler: mux}
	go func() {
		slog.Info("tool runtime listening", "port", port)
		if err := httpx.ListenAndServe(srv, cfg); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "err", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down tool runtime")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}

func newToolRuntime() tools.Runtime {
	rt := strings.ToLower(strings.TrimSpace(os.Getenv("TOOL_RUNTIME")))
	if rt == "docker" {
		return tools.NewDockerRuntime()
	}
	return tools.NewNoopRuntime()
}

type checkResult struct {
	Allowed          bool   `json:"allowed"`
	ApprovalRequired bool   `json:"approval_required"`
	Reason           string `json:"reason"`
}

type approvalGate struct {
	accessControlAddr string
	db                *sql.DB
	client            *http.Client
}

func (g *approvalGate) check(ctx context.Context, toolName, taskID, workerID string) (*checkResult, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"subject":   "tool-runtime",
		"action":    "tool.execute",
		"resource":  "tool:" + toolName,
		"tool_name": toolName,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", g.accessControlAddr+"/check", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var res checkResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (g *approvalGate) insertPending(ctx context.Context, toolName, taskID, workerID string) (string, error) {
	id := uuid.New().String()
	actionSummary := fmt.Sprintf("tool.execute:%s", toolName)

	var tID, wID interface{}
	if taskID != "" {
		tID = taskID
	} else {
		tID = nil
	}
	if workerID != "" {
		wID = workerID
	} else {
		wID = nil
	}

	_, err := g.db.ExecContext(ctx,
		`INSERT INTO approval_requests (id, task_id, worker_id, tool_name, action_summary, status)
		 VALUES ($1, $2, $3, $4, $5, 'pending')`,
		id, tID, wID, toolName, actionSummary)
	if err != nil {
		return "", fmt.Errorf("insert approval_requests: %w", err)
	}
	return id, nil
}
