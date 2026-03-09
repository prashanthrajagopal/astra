package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"astra/pkg/config"
	"astra/pkg/logger"

	kernel_pb "astra/proto/kernel"
	tasks_pb "astra/proto/tasks"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	agentConn   *grpc.ClientConn
	taskConn    *grpc.ClientConn
	agentClient kernel_pb.KernelServiceClient
	taskClient  tasks_pb.TaskServiceClient
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	log := logger.New(cfg.LogLevel)
	slog.SetDefault(log)

	agentAddr := fmt.Sprintf("localhost:%d", cfg.AgentGRPCPort)
	taskAddr := fmt.Sprintf("localhost:%d", cfg.GRPCPort)

	agentConn, err = grpc.NewClient(agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("failed to connect to agent service", "addr", agentAddr, "err", err)
		os.Exit(1)
	}
	defer agentConn.Close()

	taskConn, err = grpc.NewClient(taskAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("failed to connect to task service", "addr", taskAddr, "err", err)
		os.Exit(1)
	}
	defer taskConn.Close()

	agentClient = kernel_pb.NewKernelServiceClient(agentConn)
	taskClient = tasks_pb.NewTaskServiceClient(taskConn)

	auth := newAuthMiddleware(cfg.IdentityAddr, cfg.AccessControlAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.Handle("POST /agents", auth.protect(http.HandlerFunc(handleAgents)))
	mux.Handle("POST /agents/{id}/goals", auth.protect(http.HandlerFunc(handleAgentGoals)))
	mux.Handle("/tasks/{rest...}", auth.protect(http.HandlerFunc(handleTasks)))
	mux.Handle("/graphs/{rest...}", auth.protect(http.HandlerFunc(handleGraphs)))

	addr := fmt.Sprintf(":%d", cfg.HTTPPort)
	slog.Info("api gateway started", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}

type authMiddleware struct {
	identityAddr      string
	accessControlAddr string
	client            *http.Client
}

func newAuthMiddleware(identityAddr, accessControlAddr string) *authMiddleware {
	return &authMiddleware{
		identityAddr:      strings.TrimSuffix(identityAddr, "/"),
		accessControlAddr: strings.TrimSuffix(accessControlAddr, "/"),
		client:            &http.Client{Timeout: 100 * time.Millisecond},
	}
}

func (a *authMiddleware) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := extractBearer(r)
		if tok == "" {
			http.Error(w, "missing or invalid authorization", http.StatusUnauthorized)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 150*time.Millisecond)
		defer cancel()

		// Call identity /validate
		valBody, _ := json.Marshal(map[string]string{"token": tok})
		valReq, _ := http.NewRequestWithContext(ctx, "POST", a.identityAddr+"/validate", bytes.NewReader(valBody))
		valReq.Header.Set("Content-Type", "application/json")
		valResp, err := a.client.Do(valReq)
		if err != nil {
			slog.Warn("identity validate failed", "err", err)
			http.Error(w, "authentication failed", http.StatusUnauthorized)
			return
		}
		defer valResp.Body.Close()
		if valResp.StatusCode == http.StatusUnauthorized {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}
		if valResp.StatusCode != http.StatusOK {
			http.Error(w, "authentication failed", http.StatusUnauthorized)
			return
		}
		var valRes struct {
			Valid   bool     `json:"valid"`
			Subject string   `json:"subject"`
			Scopes  []string `json:"scopes"`
		}
		if err := json.NewDecoder(valResp.Body).Decode(&valRes); err != nil || !valRes.Valid {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		action := r.Method + " " + r.URL.Path
		checkBody, _ := json.Marshal(map[string]interface{}{
			"subject":   valRes.Subject,
			"action":    action,
			"resource":  r.URL.Path,
			"tool_name": "",
		})
		checkReq, _ := http.NewRequestWithContext(ctx, "POST", a.accessControlAddr+"/check", bytes.NewReader(checkBody))
		checkReq.Header.Set("Content-Type", "application/json")
		checkResp, err := a.client.Do(checkReq)
		if err != nil {
			slog.Warn("access-control check failed", "err", err)
			http.Error(w, "authorization check failed", http.StatusInternalServerError)
			return
		}
		defer checkResp.Body.Close()
		var checkRes struct {
			Allowed          bool   `json:"allowed"`
			ApprovalRequired bool   `json:"approval_required"`
			Reason           string `json:"reason"`
		}
		json.NewDecoder(checkResp.Body).Decode(&checkRes)
		if !checkRes.Allowed {
			http.Error(w, "forbidden: "+checkRes.Reason, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func extractBearer(r *http.Request) string {
	ah := r.Header.Get("Authorization")
	if ah == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(ah, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(ah, prefix))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

func handleAgents(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ActorType string `json:"actor_type"`
		Config    string `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	configBytes := []byte(req.Config)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	resp, err := agentClient.SpawnActor(ctx, &kernel_pb.SpawnActorRequest{
		ActorType: req.ActorType,
		Config:    configBytes,
	})
	if err != nil {
		slog.Error("SpawnActor failed", "err", err)
		http.Error(w, "spawn failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"actor_id": resp.ActorId})
}

func handleAgentGoals(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}
	var req struct {
		GoalText string `json:"goal_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	payload, _ := json.Marshal(map[string]string{"goal_text": req.GoalText})
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	_, err := agentClient.SendMessage(ctx, &kernel_pb.SendMessageRequest{
		TargetActorId: agentID,
		MessageType:   "CreateGoal",
		Source:        "api-gateway",
		Payload:       payload,
	})
	if err != nil {
		slog.Error("SendMessage CreateGoal failed", "agent_id", agentID, "err", err)
		http.Error(w, "create goal failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleTasks(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	path := "/tasks/" + rest
	taskID := strings.TrimSuffix(rest, "/complete")
	taskID = strings.TrimSuffix(taskID, "/")
	isComplete := strings.HasSuffix(path, "/complete")

	if taskID == "" {
		http.Error(w, "task id required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if isComplete && r.Method == http.MethodPost {
		var req struct {
			Result []byte `json:"result"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		_, err := taskClient.CompleteTask(ctx, &tasks_pb.CompleteTaskRequest{
			TaskId: taskID,
			Result: req.Result,
		})
		if err != nil {
			slog.Error("CompleteTask failed", "task_id", taskID, "err", err)
			http.Error(w, "complete failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	if r.Method == http.MethodGet {
		resp, err := taskClient.GetTask(ctx, &tasks_pb.GetTaskRequest{TaskId: taskID})
		if err != nil {
			slog.Error("GetTask failed", "task_id", taskID, "err", err)
			http.Error(w, "get task failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         resp.Id,
			"graph_id":   resp.GraphId,
			"agent_id":   resp.AgentId,
			"type":       resp.Type,
			"status":     resp.Status,
			"payload":    resp.Payload,
			"result":     resp.Result,
			"priority":   resp.Priority,
			"retries":    resp.Retries,
			"created_at": resp.CreatedAt,
			"updated_at": resp.UpdatedAt,
		})
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func handleGraphs(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	graphID := strings.TrimSuffix(rest, "/")
	if graphID == "" {
		http.Error(w, "graph id required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	resp, err := taskClient.GetGraph(ctx, &tasks_pb.GetGraphRequest{GraphId: graphID})
	if err != nil {
		slog.Error("GetGraph failed", "graph_id", graphID, "err", err)
		http.Error(w, "get graph failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tasks":        resp.Tasks,
		"dependencies": resp.Dependencies,
	})
}
