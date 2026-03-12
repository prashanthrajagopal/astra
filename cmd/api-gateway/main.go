package main

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"astra/internal/agentdocs"
	"astra/internal/chat"
	"astra/internal/dashboard"
	"astra/internal/llm"
	"astra/internal/memory"
	"astra/pkg/config"
	"astra/pkg/db"
	astraGrpc "astra/pkg/grpc"
	"astra/pkg/httpx"
	"astra/pkg/logger"

	kernel_pb "astra/proto/kernel"
	tasks_pb "astra/proto/tasks"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	gogrpc "google.golang.org/grpc"
)

var (
	agentConn    *gogrpc.ClientConn
	taskConn     *gogrpc.ClientConn
	agentClient  kernel_pb.KernelServiceClient
	taskClient   tasks_pb.TaskServiceClient
	docStore     *agentdocs.Store
)

const (
	headerContentType = "Content-Type"
	contentTypeJSON   = "application/json"
)

//go:embed dashboard
var dashboardFS embed.FS

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

	agentConn, err = astraGrpc.Dial(context.Background(), agentAddr, cfg)
	if err != nil {
		slog.Error("failed to connect to agent service", "addr", agentAddr, "err", err)
		os.Exit(1)
	}
	defer agentConn.Close()

	taskConn, err = astraGrpc.Dial(context.Background(), taskAddr, cfg)
	if err != nil {
		slog.Error("failed to connect to task service", "addr", taskAddr, "err", err)
		os.Exit(1)
	}
	defer taskConn.Close()

	agentClient = kernel_pb.NewKernelServiceClient(agentConn)
	taskClient = tasks_pb.NewTaskServiceClient(taskConn)

	database, err := db.Connect(cfg.PostgresDSN())
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer rdb.Close()

	docStore = agentdocs.NewStore(database, rdb)
	chatStore := chat.NewStore(database)
	llmBackend := llm.NewEndpointBackendFromEnv()

	auth, err := newAuthMiddleware(cfg, cfg.IdentityAddr, cfg.AccessControlAddr)
	if err != nil {
		slog.Error("failed to initialize auth middleware client", "err", err)
		os.Exit(1)
	}
	dashCollector, err := dashboard.NewCollector(cfg)
	if err != nil {
		slog.Error("failed to initialize dashboard collector", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	dashboardClient, err := httpx.NewClient(cfg, 500*time.Millisecond)
	if err != nil {
		slog.Error("failed to initialize dashboard action client", "err", err)
		os.Exit(1)
	}
	var memoryStore *memory.Store
	if cfg.ChatEnabled {
		memoryStore, err = initMemoryStore(database)
		if err != nil {
			slog.Warn("memory store init failed, chat will run without memory context", "err", err)
			memoryStore = nil
		}
	}
	registerDashboardRoutes(mux, cfg, dashCollector, dashboardClient, docStore, chatStore, database, llmBackend, memoryStore)
	if cfg.ChatEnabled {
		wsHandler := chat.NewWebSocketHandler(chatStore, database, llmBackend, &chat.HandlerConfig{
			MaxMsgLength: cfg.ChatMaxMsgLength,
			RateLimit:    cfg.ChatRateLimit,
			TokenCap:    cfg.ChatTokenCap,
			MemoryStore: memoryStore,
		})
		mux.HandleFunc("GET /chat/ws", wsHandler)
	}
	mux.Handle("GET /agents", auth.protect(http.HandlerFunc(handleListAgents)))
	mux.Handle("POST /agents", auth.protect(http.HandlerFunc(handleAgents)))
	mux.Handle("PATCH /agents/{id}", auth.protect(http.HandlerFunc(handleUpdateAgent)))
	mux.Handle("DELETE /agents/{id}", auth.protect(http.HandlerFunc(handleDeleteAgent)))
	mux.Handle("GET /agents/{id}/profile", auth.protect(http.HandlerFunc(handleGetProfile)))
	mux.Handle("POST /agents/{id}/documents", auth.protect(http.HandlerFunc(handleCreateDocument)))
	mux.Handle("GET /agents/{id}/documents", auth.protect(http.HandlerFunc(handleListDocuments)))
	mux.Handle("DELETE /agents/{id}/documents/{doc_id}", auth.protect(http.HandlerFunc(handleDeleteDocument)))
	mux.Handle("POST /agents/{id}/goals", auth.protect(http.HandlerFunc(handleAgentGoals)))
	mux.Handle("/tasks/{rest...}", auth.protect(http.HandlerFunc(handleTasks)))
	mux.Handle("/graphs/{rest...}", auth.protect(http.HandlerFunc(handleGraphs)))

	addr := fmt.Sprintf(":%d", cfg.HTTPPort)
	slog.Info("api gateway started", "addr", addr)
	srv := &http.Server{Addr: addr, Handler: mux}
	if err := httpx.ListenAndServe(srv, cfg); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func initMemoryStore(db *sql.DB) (*memory.Store, error) {
	// Use nil embedder: Search with nil returns recent memories by created_at.
	// For semantic search, memory-service with embedder would be needed.
	store := memory.NewStore(db, nil)
	slog.Info("chat memory store initialized (no embedder, using recent memories for context)")
	return store, nil
}

type authMiddleware struct {

	identityAddr      string
	accessControlAddr string
	client            *http.Client
}

func newAuthMiddleware(cfg *config.Config, identityAddr, accessControlAddr string) (*authMiddleware, error) {
	client, err := httpx.NewClient(cfg, 100*time.Millisecond)
	if err != nil {
		return nil, err
	}
	return &authMiddleware{
		identityAddr:      strings.TrimSuffix(identityAddr, "/"),
		accessControlAddr: strings.TrimSuffix(accessControlAddr, "/"),
		client:            client,
	}, nil
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
		valReq.Header.Set(headerContentType, contentTypeJSON)
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
		checkReq.Header.Set(headerContentType, contentTypeJSON)
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

func registerDashboardRoutes(mux *http.ServeMux, cfg *config.Config, collector *dashboard.Collector, client *http.Client, store *agentdocs.Store, chatStore *chat.Store, database *sql.DB, llmBackend *llm.EndpointBackend, memStore *memory.Store) {
	sub, err := fs.Sub(dashboardFS, "dashboard")
	if err != nil {
		slog.Error("dashboard embed setup failed", "err", err)
		return
	}
	fileServer := http.FileServer(http.FS(sub))
	mux.Handle("GET /dashboard/", http.StripPrefix("/dashboard/", fileServer))
	mux.Handle("GET /dashboard", http.RedirectHandler("/dashboard/", http.StatusMovedPermanently))
	mux.HandleFunc("GET /api/dashboard/snapshot", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
		defer cancel()
		snap := collector.Collect(ctx)
		// Enrich with agents from agent-service (dashboard only, no auth)
		if agentClient != nil {
			resp, err := agentClient.QueryState(ctx, &kernel_pb.QueryStateRequest{EntityType: "agents"})
			if err == nil {
				for _, b := range resp.Results {
					var row struct {
						ID     string `json:"id"`
						Name   string `json:"name"`
						Status string `json:"status"`
					}
					if err := json.Unmarshal(b, &row); err != nil {
						continue
					}
					snap.Agents = append(snap.Agents, map[string]any{
						"id": row.ID, "name": row.Name, "actor_type": row.Name, "status": row.Status,
					})
				}
				snap.AgentCount = len(snap.Agents)
			}
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(snap)
	})
	mux.HandleFunc("GET /api/dashboard/approvals/{id}", func(w http.ResponseWriter, r *http.Request) {
		handleGetApprovalProxy(w, r, client, strings.TrimSuffix(cfg.AccessControlAddr, "/"))
	})
	mux.HandleFunc("POST /api/dashboard/approvals/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
		handleApprovalActionProxy(w, r, client, strings.TrimSuffix(cfg.AccessControlAddr, "/"), "approve")
	})
	mux.HandleFunc("POST /api/dashboard/approvals/{id}/reject", func(w http.ResponseWriter, r *http.Request) {
		handleApprovalActionProxy(w, r, client, strings.TrimSuffix(cfg.AccessControlAddr, "/"), "deny")
	})
	mux.HandleFunc("GET /api/dashboard/settings", func(w http.ResponseWriter, r *http.Request) {
		autoApprove := os.Getenv("AUTO_APPROVE_PLANS") == "true"
		w.Header().Set(headerContentType, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(map[string]bool{"auto_approve_plans": autoApprove})
	})
	goalServiceBase := fmt.Sprintf("http://localhost:%d", cfg.GoalServicePort)
	if cfg.GoalServicePort == 0 {
		goalServiceBase = "http://localhost:8088"
	}
	mux.HandleFunc("GET /api/dashboard/goals/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if strings.TrimSpace(id) == "" {
			http.Error(w, "goal id required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(goalServiceBase, "/")+"/goals/"+id+"/details", nil)
		if err != nil {
			http.Error(w, "request build failed", http.StatusInternalServerError)
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "goal service unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set(headerContentType, contentTypeJSON)
		if resp.StatusCode != http.StatusOK {
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
			return
		}
		io.Copy(w, resp.Body)
	})
	mux.HandleFunc("POST /api/dashboard/goals/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if strings.TrimSpace(id) == "" {
			http.Error(w, "goal id required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// Cancel all non-terminal tasks for this goal
		_, _ = database.ExecContext(ctx,
			`UPDATE tasks SET status = 'failed', result = '{"cancelled":true}'::jsonb, updated_at = now()
			 WHERE goal_id = $1::uuid AND status NOT IN ('completed', 'failed')`, id)

		// Cancel the goal itself
		result, err := database.ExecContext(ctx,
			`UPDATE goals SET status = 'failed' WHERE id = $1::uuid AND status NOT IN ('completed', 'failed')`, id)
		if err != nil {
			slog.Error("cancel goal failed", "err", err, "goal_id", id)
			http.Error(w, "cancel failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			http.Error(w, "goal not found or already terminal", http.StatusConflict)
			return
		}
		// Event for audit
		_, _ = database.ExecContext(ctx,
			`INSERT INTO events (event_type, actor_id, payload, created_at) VALUES ('GoalCancelled', $1::uuid, jsonb_build_object('goal_id', $1::uuid, 'cancelled', true), now())`, id)

		w.Header().Set(headerContentType, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
	})
	mux.HandleFunc("POST /api/dashboard/tasks/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "task id required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		result, err := database.ExecContext(ctx,
			`UPDATE tasks SET status = 'failed', result = '{"cancelled":true}'::jsonb, updated_at = now()
			 WHERE id = $1::uuid AND status NOT IN ('completed', 'failed')`, id)
		if err != nil {
			slog.Error("cancel task failed", "err", err, "task_id", id)
			http.Error(w, "cancel failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			http.Error(w, "task not found or already terminal", http.StatusConflict)
			return
		}
		_, _ = database.ExecContext(ctx,
			`INSERT INTO events (event_type, actor_id, payload, created_at) VALUES ('TaskCancelled', $1::uuid, '{"cancelled":true}'::jsonb, now())`, id)
		w.Header().Set(headerContentType, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
	})
	if store != nil {
		mux.HandleFunc("PATCH /api/dashboard/agents/{id}/status", func(w http.ResponseWriter, r *http.Request) {
			handleDashboardAgentStatus(w, r, store)
		})
		mux.HandleFunc("DELETE /api/dashboard/agents/{id}", func(w http.ResponseWriter, r *http.Request) {
			handleDashboardAgentDelete(w, r, store)
		})
	}
	if chatStore != nil {
		mux.HandleFunc("GET /api/dashboard/chat/agents", handleDashboardChatAgents(chatStore))
		mux.HandleFunc("GET /api/dashboard/chat/sessions", handleDashboardChatListSessions(chatStore))
		mux.HandleFunc("POST /api/dashboard/chat/sessions", handleDashboardChatCreateSession(chatStore))
		mux.HandleFunc("GET /api/dashboard/chat/sessions/{id}", handleDashboardChatGetSession(chatStore))
		mux.HandleFunc("GET /api/dashboard/chat/sessions/{id}/messages", handleDashboardChatGetMessages(chatStore))
		goalServiceAddr := strings.TrimSuffix(cfg.GoalServiceAddr, "/")
		mux.HandleFunc("POST /api/dashboard/chat/sessions/{id}/messages", handleDashboardChatAppendMessage(chatStore, goalServiceAddr, database, llmBackend, memStore))
	}
}

func handleDashboardAgentStatus(w http.ResponseWriter, r *http.Request, store *agentdocs.Store) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}
	agentID, err := uuid.Parse(id)
	if err != nil {
		http.Error(w, "invalid agent id", http.StatusBadRequest)
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Status == "" {
		http.Error(w, "body must be JSON with status", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := store.UpdateAgentStatus(ctx, agentID, req.Status); err != nil {
		slog.Error("dashboard UpdateAgentStatus failed", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleDashboardAgentDelete(w http.ResponseWriter, r *http.Request, store *agentdocs.Store) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}
	agentID, err := uuid.Parse(id)
	if err != nil {
		http.Error(w, "invalid agent id", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := store.DeleteAgent(ctx, agentID); err != nil {
		slog.Error("dashboard DeleteAgent failed", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleDashboardChatAgents(chatStore *chat.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		agents, err := chatStore.ListChatCapableAgents(ctx)
		if err != nil {
			slog.Error("chat ListChatCapableAgents failed", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := make([]map[string]any, len(agents))
		for i, a := range agents {
			items[i] = map[string]any{"id": a.ID.String(), "name": a.Name}
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]any{"agents": items})
	}
}

func handleDashboardChatListSessions(chatStore *chat.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		agentIDStr := r.URL.Query().Get("agent_id")
		var agentID *uuid.UUID
		if agentIDStr != "" {
			id, err := uuid.Parse(agentIDStr)
			if err != nil {
				http.Error(w, "invalid agent_id", http.StatusBadRequest)
				return
			}
			agentID = &id
		}
		sessions, err := chatStore.ListSessions(ctx, chat.DashboardUserID, agentID)
		if err != nil {
			slog.Error("chat ListSessions failed", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := make([]map[string]any, len(sessions))
		for i, s := range sessions {
			items[i] = map[string]any{
				"id": s.ID.String(), "user_id": s.UserID, "agent_id": s.AgentID.String(),
				"title": s.Title, "status": s.Status, "created_at": s.CreatedAt, "updated_at": s.UpdatedAt,
			}
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]any{"sessions": items})
	}
}

func handleDashboardChatCreateSession(chatStore *chat.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AgentID string `json:"agent_id"`
			Title   string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		agentID, err := uuid.Parse(req.AgentID)
		if err != nil {
			http.Error(w, "invalid agent_id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		se, err := chatStore.CreateSession(ctx, chat.DashboardUserID, agentID, req.Title)
		if err != nil {
			slog.Error("chat CreateSession failed", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		slog.Info("chat session created", "session_id", se.ID, "agent_id", se.AgentID, "user_id", chat.DashboardUserID)
		proto := "ws"
		if r.TLS != nil {
			proto = "wss"
		}
		wsPath := fmt.Sprintf("%s://%s/chat/ws?session_id=%s", proto, r.Host, se.ID.String())
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id": se.ID.String(), "agent_id": se.AgentID.String(), "title": se.Title, "status": se.Status,
			"created_at": se.CreatedAt, "updated_at": se.UpdatedAt,
			"websocket_url": wsPath,
		})
	}
}

func handleDashboardChatGetSession(chatStore *chat.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "session id required", http.StatusBadRequest)
			return
		}
		sessionID, err := uuid.Parse(id)
		if err != nil {
			http.Error(w, "invalid session id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		se, err := chatStore.GetSession(ctx, sessionID, chat.DashboardUserID)
		if err != nil {
			slog.Error("chat GetSession failed", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if se == nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		proto := "ws"
		if r.TLS != nil {
			proto = "wss"
		}
		wsPath := fmt.Sprintf("%s://%s/chat/ws?session_id=%s", proto, r.Host, se.ID.String())
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]any{
			"id": se.ID.String(), "agent_id": se.AgentID.String(), "title": se.Title, "status": se.Status,
			"created_at": se.CreatedAt, "updated_at": se.UpdatedAt,
			"websocket_url": wsPath,
		})
	}
}

func handleDashboardChatGetMessages(chatStore *chat.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "session id required", http.StatusBadRequest)
			return
		}
		sessionID, err := uuid.Parse(id)
		if err != nil {
			http.Error(w, "invalid session id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		msgs, err := chatStore.GetMessages(ctx, sessionID, chat.DashboardUserID)
		if err != nil {
			slog.Error("chat GetMessages failed", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if msgs == nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		items := make([]map[string]any, len(msgs))
		for i, m := range msgs {
			items[i] = map[string]any{
				"id": m.ID.String(), "session_id": m.SessionID.String(), "role": m.Role, "content": m.Content,
				"tokens_in": m.TokensIn, "tokens_out": m.TokensOut, "created_at": m.CreatedAt,
			}
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]any{"messages": items})
	}
}

func handleDashboardChatAppendMessage(chatStore *chat.Store, goalServiceAddr string, db *sql.DB, llmBackend *llm.EndpointBackend, memStore *memory.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "session id required", http.StatusBadRequest)
			return
		}
		sessionID, err := uuid.Parse(id)
		if err != nil {
			http.Error(w, "invalid session id", http.StatusBadRequest)
			return
		}
		var req struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if req.Content == "" {
			http.Error(w, "content required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
		defer cancel()

		m, err := chatStore.AppendMessage(ctx, sessionID, chat.DashboardUserID, "user", req.Content, nil, nil, 0, 0)
		if err != nil {
			slog.Error("chat AppendMessage failed", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if m == nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		se, err := chatStore.GetSession(ctx, sessionID, chat.DashboardUserID)
		if err != nil || se == nil {
			slog.Error("chat GetSession failed", "err", err, "session_id", sessionID)
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		var assistantMsg *chat.Message
		if chat.NeedsGoalWorkflow(req.Content) {
			slog.Info("chat routing to goal workflow", "session_id", sessionID, "content_preview", req.Content[:min(len(req.Content), 50)])
			assistantMsg, err = chat.ProcessGoalMessage(ctx, chatStore, goalServiceAddr, se.AgentID, sessionID, chat.DashboardUserID, req.Content)
		} else {
			slog.Info("chat routing to direct LLM", "session_id", sessionID, "content_preview", req.Content[:min(len(req.Content), 50)])
			directCtx, directCancel := context.WithTimeout(ctx, 60*time.Second)
			defer directCancel()
			assistantMsg, err = chat.ProcessRESTMessage(directCtx, chatStore, db, llmBackend, sessionID, chat.DashboardUserID, memStore)
		}
		if err != nil {
			slog.Error("chat ProcessGoalMessage failed", "err", err, "session_id", sessionID)
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{
				"id": m.ID.String(), "role": "user", "content": m.Content,
				"assistant_error": err.Error(),
			})
			return
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id": m.ID.String(), "role": "user", "content": m.Content,
			"assistant": map[string]any{
				"id": assistantMsg.ID.String(), "role": "assistant", "content": assistantMsg.Content,
				"tokens_in": assistantMsg.TokensIn, "tokens_out": assistantMsg.TokensOut,
			},
		})
	}
}

func handleGetApprovalProxy(w http.ResponseWriter, r *http.Request, client *http.Client, accessControlAddr string) {
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		http.Error(w, "approval id required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, accessControlAddr+"/approvals/"+id, nil)
	if err != nil {
		http.Error(w, "request build failed", http.StatusInternalServerError)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "access-control unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set(headerContentType, contentTypeJSON)
	if resp.StatusCode == http.StatusNotFound {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "approval not found"})
		return
	}
	if resp.StatusCode >= 300 {
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		return
	}
	io.Copy(w, resp.Body)
}

func handleApprovalActionProxy(w http.ResponseWriter, r *http.Request, client *http.Client, accessControlAddr, action string) {
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		http.Error(w, "approval id required", http.StatusBadRequest)
		return
	}
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		payload = map[string]any{"decided_by": "dashboard-ui"}
	}
	body, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(r.Context(), 800*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, accessControlAddr+"/approvals/"+id+"/"+action, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "request build failed", http.StatusInternalServerError)
		return
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "access-control unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set(headerContentType, contentTypeJSON)
	if resp.StatusCode >= 300 {
		w.WriteHeader(resp.StatusCode)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "error",
			"code":   resp.StatusCode,
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      "ok",
		"approval_id": id,
		"action":      action,
	})
}

func handleListAgents(w http.ResponseWriter, r *http.Request) {
	// Route is registered as "GET /agents" so only GET reaches this handler.
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	resp, err := agentClient.QueryState(ctx, &kernel_pb.QueryStateRequest{EntityType: "agents"})
	if err != nil {
		slog.Error("QueryState agents failed", "err", err)
		http.Error(w, "list agents failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	type agentRow struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		ActorType string `json:"actor_type"`
		Status    string `json:"status"`
	}
	var agents []map[string]interface{}
	for _, b := range resp.Results {
		var row agentRow
		if err := json.Unmarshal(b, &row); err != nil {
			continue
		}
		agents = append(agents, map[string]interface{}{
			"id":          row.ID,
			"actor_type":  row.ActorType,
			"name":        row.Name,
			"status":      row.Status,
		})
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]interface{}{"agents": agents})
}

func handleAgents(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ActorType    string `json:"actor_type"`
		Config       string `json:"config"`
		Name         string `json:"name"`
		SystemPrompt string `json:"system_prompt"`
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
	if req.SystemPrompt != "" {
		agentID, err := uuid.Parse(resp.ActorId)
		if err == nil && docStore != nil {
			_ = docStore.UpdateProfile(ctx, agentID, &req.SystemPrompt, nil)
		}
	}
	if req.Name != "" {
		agentID, err := uuid.Parse(resp.ActorId)
		if err == nil && docStore != nil {
			_ = docStore.UpdateAgentName(ctx, agentID, req.Name)
		}
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]string{"actor_id": resp.ActorId})
}

func handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}
	var req struct {
		SystemPrompt *string          `json:"system_prompt"`
		Config       *json.RawMessage `json:"config"`
		Status       *string          `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	id, err := uuid.Parse(agentID)
	if err != nil {
		http.Error(w, "invalid agent id", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if req.Status != nil {
		if err := docStore.UpdateAgentStatus(ctx, id, *req.Status); err != nil {
			slog.Error("UpdateAgentStatus failed", "err", err)
			http.Error(w, "update status failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := docStore.UpdateProfile(ctx, id, req.SystemPrompt, req.Config); err != nil {
		slog.Error("UpdateProfile failed", "err", err)
		http.Error(w, "update failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}
	id, err := uuid.Parse(agentID)
	if err != nil {
		http.Error(w, "invalid agent id", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := docStore.DeleteAgent(ctx, id); err != nil {
		slog.Error("DeleteAgent failed", "err", err)
		http.Error(w, "delete failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleGetProfile(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}
	id, err := uuid.Parse(agentID)
	if err != nil {
		http.Error(w, "invalid agent id", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	profile, err := docStore.GetProfile(ctx, id)
	if err != nil {
		slog.Error("GetProfile failed", "err", err)
		http.Error(w, "get profile failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if profile == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":            profile.ID.String(),
		"name":          profile.Name,
		"system_prompt": profile.SystemPrompt,
		"config":        profile.Config,
	})
}

func handleCreateDocument(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}
	agentUUID, err := uuid.Parse(agentID)
	if err != nil {
		http.Error(w, "invalid agent id", http.StatusBadRequest)
		return
	}
	var req struct {
		DocType  string          `json:"doc_type"`
		Name     string          `json:"name"`
		Content  *string         `json:"content,omitempty"`
		URI      *string         `json:"uri,omitempty"`
		Metadata json.RawMessage `json:"metadata,omitempty"`
		Priority int             `json:"priority"`
		GoalID   *string         `json:"goal_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.DocType == "" || req.Name == "" {
		http.Error(w, "doc_type and name required", http.StatusBadRequest)
		return
	}
	dt := agentdocs.DocType(req.DocType)
	if dt != agentdocs.DocTypeRule && dt != agentdocs.DocTypeSkill && dt != agentdocs.DocTypeContextDoc && dt != agentdocs.DocTypeReference {
		http.Error(w, "invalid doc_type", http.StatusBadRequest)
		return
	}
	if req.Content == nil && req.URI == nil {
		http.Error(w, "content or uri required", http.StatusBadRequest)
		return
	}
	if req.Priority == 0 {
		req.Priority = 100
	}
	doc := &agentdocs.Document{
		AgentID:   agentUUID,
		DocType:   dt,
		Name:      req.Name,
		Content:   req.Content,
		URI:       req.URI,
		Metadata:  req.Metadata,
		Priority:  req.Priority,
	}
	if req.GoalID != nil {
		g, err := uuid.Parse(*req.GoalID)
		if err == nil {
			doc.GoalID = &g
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := docStore.CreateDocument(ctx, doc); err != nil {
		slog.Error("CreateDocument failed", "err", err)
		http.Error(w, "create document failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": doc.ID.String()})
}

func handleListDocuments(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}
	agentUUID, err := uuid.Parse(agentID)
	if err != nil {
		http.Error(w, "invalid agent id", http.StatusBadRequest)
		return
	}
	opts := agentdocs.ListOptions{}
	if dt := r.URL.Query().Get("doc_type"); dt != "" {
		d := agentdocs.DocType(dt)
		opts.DocType = &d
	}
	if gID := r.URL.Query().Get("goal_id"); gID != "" {
		g, err := uuid.Parse(gID)
		if err == nil {
			opts.GoalID = &g
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	docs, err := docStore.ListDocuments(ctx, agentUUID, opts)
	if err != nil {
		slog.Error("ListDocuments failed", "err", err)
		http.Error(w, "list documents failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]interface{}, len(docs))
	for i, d := range docs {
		items[i] = map[string]interface{}{
			"id":         d.ID.String(),
			"agent_id":   d.AgentID.String(),
			"doc_type":   string(d.DocType),
			"name":       d.Name,
			"content":    d.Content,
			"uri":        d.URI,
			"metadata":   d.Metadata,
			"priority":   d.Priority,
			"created_at": d.CreatedAt,
			"updated_at": d.UpdatedAt,
		}
		if d.GoalID != nil {
			items[i]["goal_id"] = d.GoalID.String()
		}
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]interface{}{"documents": items})
}

func handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	docID := r.PathValue("doc_id")
	if agentID == "" || docID == "" {
		http.Error(w, "agent id and doc_id required", http.StatusBadRequest)
		return
	}
	docUUID, err := uuid.Parse(docID)
	if err != nil {
		http.Error(w, "invalid doc_id", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := docStore.DeleteDocument(ctx, docUUID); err != nil {
		slog.Error("DeleteDocument failed", "err", err)
		http.Error(w, "delete failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
	w.Header().Set(headerContentType, contentTypeJSON)
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
		w.Header().Set(headerContentType, contentTypeJSON)
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
		w.Header().Set(headerContentType, contentTypeJSON)
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
	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tasks":        resp.Tasks,
		"dependencies": resp.Dependencies,
	})
}
