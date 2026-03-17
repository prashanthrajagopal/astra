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
	"strconv"
	"strings"
	"sync"
	"time"

	"astra/internal/agentdocs"
	"astra/internal/chat"
	"astra/internal/dashboard"
	"astra/internal/llm"
	"astra/internal/memory"
	"astra/internal/slack"
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
	agentConn   *gogrpc.ClientConn
	taskConn    *gogrpc.ClientConn
	agentClient kernel_pb.KernelServiceClient
	taskClient  tasks_pb.TaskServiceClient
	docStore    *agentdocs.Store
)

const (
	headerContentType = "Content-Type"
	contentTypeJSON   = "application/json"
)

// writeJSONError sets Content-Type and writes a JSON body {"error": msg} with the given status code.
func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

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
	registerDashboardRoutes(mux, cfg, dashCollector, dashboardClient, docStore, chatStore, database, llmBackend, memoryStore, auth)
	mux.HandleFunc("GET /login", handleLoginPage)
	mux.HandleFunc("POST /login", makeLoginProxyHandler(dashboardClient, cfg.IdentityAddr))
	if cfg.ChatEnabled {
		wsHandler := chat.NewWebSocketHandler(chatStore, database, llmBackend, &chat.HandlerConfig{
			MaxMsgLength: cfg.ChatMaxMsgLength,
			RateLimit:    cfg.ChatRateLimit,
			TokenCap:     cfg.ChatTokenCap,
			MemoryStore:  memoryStore,
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
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusFound)
	})

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
			Valid        bool     `json:"valid"`
			Subject      string   `json:"subject"`
			Scopes       []string `json:"scopes"`
			UserID       string   `json:"user_id"`
			Email        string   `json:"email"`
			IsSuperAdmin bool     `json:"is_super_admin"`
		}
		if err := json.NewDecoder(valResp.Body).Decode(&valRes); err != nil || !valRes.Valid {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		action := r.Method + " " + r.URL.Path
		checkBody, _ := json.Marshal(map[string]interface{}{
			"subject":        valRes.Subject,
			"action":         action,
			"resource":       r.URL.Path,
			"tool_name":      "",
			"is_super_admin": valRes.IsSuperAdmin,
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
		if err := json.NewDecoder(checkResp.Body).Decode(&checkRes); err != nil {
			http.Error(w, "access control response invalid", http.StatusBadGateway)
			return
		}
		if !checkRes.Allowed {
			http.Error(w, "forbidden: "+checkRes.Reason, http.StatusForbidden)
			return
		}

		r.Header.Set("X-User-Id", valRes.UserID)
		r.Header.Set("X-Email", valRes.Email)
		if valRes.IsSuperAdmin {
			r.Header.Set("X-Is-Super-Admin", "true")
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

func handleLoginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(loginPageHTML))
}

func makeLoginProxyHandler(client *http.Client, identityAddr string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(identityAddr, "/")+"/users/login", bytes.NewReader(body))
		if err != nil {
			http.Error(w, `{"error":"request build failed"}`, http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, `{"error":"identity service unavailable"}`, http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}
}

const loginPageHTML = `<!doctype html>
<html lang="en" data-theme="dark">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>Astra — Sign In</title>
<link rel="preconnect" href="https://fonts.googleapis.com"/>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet"/>
<style>
*{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg:#0f1117;--surface:#1a1d27;--surface2:#242736;--border:#2e3348;
  --text:#e4e6f0;--text2:#9399b2;--primary:#6c8aff;--primary-hover:#8ba3ff;
  --error:#f87171;--success:#34d399;
}
body{background:var(--bg);color:var(--text);font-family:'Inter',system-ui,sans-serif;
  min-height:100vh;display:flex;align-items:center;justify-content:center}
.login-card{background:var(--surface);border:1px solid var(--border);border-radius:16px;
  padding:48px 40px;width:100%;max-width:420px;box-shadow:0 8px 32px rgba(0,0,0,.4)}
.logo{font-size:1.75rem;font-weight:700;text-align:center;margin-bottom:8px;letter-spacing:-.02em}
.logo span{color:var(--primary)}
.subtitle{text-align:center;color:var(--text2);font-size:.875rem;margin-bottom:32px}
.field{margin-bottom:20px}
.field label{display:block;font-size:.8125rem;font-weight:500;color:var(--text2);margin-bottom:6px}
.field input{width:100%;padding:10px 14px;background:var(--surface2);border:1px solid var(--border);
  border-radius:8px;color:var(--text);font-size:.9375rem;outline:none;transition:border .15s}
.field input:focus{border-color:var(--primary)}
.field input::placeholder{color:var(--text2);opacity:.6}
.btn{width:100%;padding:12px;background:var(--primary);color:#fff;border:none;border-radius:8px;
  font-size:.9375rem;font-weight:600;cursor:pointer;transition:background .15s;margin-top:4px}
.btn:hover{background:var(--primary-hover)}
.btn:disabled{opacity:.5;cursor:not-allowed}
.msg{margin-top:16px;padding:10px 14px;border-radius:8px;font-size:.8125rem;display:none}
.msg.error{display:block;background:rgba(248,113,113,.1);border:1px solid var(--error);color:var(--error)}
.msg.success{display:block;background:rgba(52,211,153,.1);border:1px solid var(--success);color:var(--success)}
.links{margin-top:24px;text-align:center;font-size:.8125rem;color:var(--text2)}
.links a{color:var(--primary);text-decoration:none}
.links a:hover{text-decoration:underline}
</style>
</head>
<body>
<div class="login-card">
  <div class="logo"><span>Astra</span></div>
  <div class="subtitle">Sign in to the Autonomous Agent Platform</div>
  <form id="login-form" autocomplete="on">
    <div class="field">
      <label for="email">Email</label>
      <input id="email" name="email" type="email" placeholder="you@company.com" required autofocus/>
    </div>
    <div class="field">
      <label for="password">Password</label>
      <input id="password" name="password" type="password" placeholder="Password" required/>
    </div>
    <button class="btn" type="submit" id="btn-login">Sign in</button>
  </form>
  <div id="msg" class="msg"></div>
  <div class="links">
    <a href="/superadmin/dashboard/">Super-Admin Dashboard</a>
  </div>
</div>
<script>
(function(){
  var form=document.getElementById('login-form');
  var msg=document.getElementById('msg');
  var btn=document.getElementById('btn-login');
  form.addEventListener('submit',function(e){
    e.preventDefault();
    btn.disabled=true;
    btn.textContent='Signing in...';
    msg.className='msg';msg.style.display='none';
    var body=JSON.stringify({email:document.getElementById('email').value,password:document.getElementById('password').value});
    fetch('/login',{method:'POST',headers:{'Content-Type':'application/json'},body:body})
      .then(function(r){return r.json().then(function(d){return{ok:r.ok,data:d}})})
      .then(function(res){
        if(!res.ok){
          msg.className='msg error';msg.textContent=res.data.error||'Login failed';msg.style.display='block';
          btn.disabled=false;btn.textContent='Sign in';
          return;
        }
        var d=res.data;
        localStorage.setItem('astra_token',d.token||'');
        localStorage.setItem('astra_user',JSON.stringify(d.user||{}));
        if(d.org) localStorage.setItem('astra_org',JSON.stringify(d.org));
        msg.className='msg success';msg.textContent='Welcome, '+(d.user&&d.user.name||'User')+'!';msg.style.display='block';
        var u=d.user||{};
        setTimeout(function(){
          if(u.is_super_admin) window.location.href='/superadmin/dashboard/';
          else window.location.href='/superadmin/dashboard/';
        },600);
      })
      .catch(function(err){
        msg.className='msg error';msg.textContent='Network error: '+err.message;msg.style.display='block';
        btn.disabled=false;btn.textContent='Sign in';
      });
  });
})();
</script>
</body>
</html>`

func handleHealth(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("ok"))
}

func registerDashboardRoutes(mux *http.ServeMux, cfg *config.Config, collector *dashboard.Collector, client *http.Client, store *agentdocs.Store, chatStore *chat.Store, database *sql.DB, llmBackend *llm.EndpointBackend, memStore *memory.Store, auth *authMiddleware) {
	sub, err := fs.Sub(dashboardFS, "dashboard")
	if err != nil {
		slog.Error("dashboard embed setup failed", "err", err)
		return
	}
	fileServer := http.FileServer(http.FS(sub))
	mux.Handle("GET /superadmin/dashboard/", http.StripPrefix("/superadmin/dashboard/", fileServer))
	mux.Handle("GET /superadmin/dashboard", http.RedirectHandler("/superadmin/dashboard/", http.StatusMovedPermanently))
	mux.Handle("GET /dashboard/", http.RedirectHandler("/superadmin/dashboard/", http.StatusMovedPermanently))
	mux.Handle("GET /dashboard", http.RedirectHandler("/superadmin/dashboard/", http.StatusMovedPermanently))
	mux.Handle("GET /superadmin/api/dashboard/snapshot", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
		defer cancel()
		snap := collector.Collect(ctx)
		// Enrich with agents from agent-service (scoped by org/super-admin via gRPC metadata)
		if agentClient != nil {
			kctx := kernelCtxWithAuth(r, ctx)
			resp, err := agentClient.QueryState(kctx, &kernel_pb.QueryStateRequest{EntityType: "agents"})
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
	})))
	mux.Handle("GET /superadmin/api/dashboard/approvals/{id}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleGetApprovalProxy(w, r, client, strings.TrimSuffix(cfg.AccessControlAddr, "/"))
	})))
	mux.Handle("POST /superadmin/api/dashboard/approvals/{id}/approve", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleApprovalActionProxy(w, r, client, strings.TrimSuffix(cfg.AccessControlAddr, "/"), "approve")
	})))
	mux.Handle("POST /superadmin/api/dashboard/approvals/{id}/reject", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleApprovalActionProxy(w, r, client, strings.TrimSuffix(cfg.AccessControlAddr, "/"), "deny")
	})))
	mux.Handle("GET /superadmin/api/dashboard/settings", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		autoApprove := os.Getenv("AUTO_APPROVE_PLANS") == "true"
		w.Header().Set(headerContentType, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(map[string]bool{"auto_approve_plans": autoApprove})
	})))
	slackStore := slack.NewStore(database)
	mux.Handle("GET /superadmin/api/slack/config", auth.protect(handleSlackConfigGet(slackStore)))
	mux.Handle("PUT /superadmin/api/slack/config", auth.protect(handleSlackConfigPut(slackStore)))
	goalServiceBase := fmt.Sprintf("http://localhost:%d", cfg.GoalServicePort)
	if cfg.GoalServicePort == 0 {
		goalServiceBase = "http://localhost:8088"
	}
	mux.Handle("GET /superadmin/api/dashboard/goals/{id}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			_, _ = io.Copy(w, resp.Body)
			return
		}
		_, _ = io.Copy(w, resp.Body)
	})))
	mux.Handle("POST /superadmin/api/dashboard/goals/{id}/cancel", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if strings.TrimSpace(id) == "" {
			http.Error(w, "goal id required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		_, _ = database.ExecContext(ctx,
			`UPDATE tasks SET status = 'failed', result = '{"cancelled":true}'::jsonb, updated_at = now()
			 WHERE goal_id = $1::uuid AND status NOT IN ('completed', 'failed')`, id)

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
		_, _ = database.ExecContext(ctx,
			`INSERT INTO events (event_type, actor_id, payload, created_at) VALUES ('GoalCancelled', $1::uuid, jsonb_build_object('goal_id', $1::uuid, 'cancelled', true), now())`, id)

		w.Header().Set(headerContentType, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
	})))
	mux.Handle("POST /superadmin/api/dashboard/tasks/{id}/cancel", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	})))
	if store != nil {
		mux.Handle("PATCH /superadmin/api/dashboard/agents/{id}/status", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handleDashboardAgentStatus(w, r, store)
		})))
		mux.Handle("DELETE /superadmin/api/dashboard/agents/{id}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handleDashboardAgentDelete(w, r, store)
		})))
	}
	if chatStore != nil {
		mux.Handle("GET /superadmin/api/dashboard/chat/agents", auth.protect(handleDashboardChatAgents(chatStore)))
		mux.Handle("GET /superadmin/api/dashboard/chat/sessions", auth.protect(handleDashboardChatListSessions(chatStore)))
		mux.Handle("POST /superadmin/api/dashboard/chat/sessions", auth.protect(handleDashboardChatCreateSession(chatStore)))
		mux.Handle("GET /superadmin/api/dashboard/chat/sessions/{id}", auth.protect(handleDashboardChatGetSession(chatStore)))
		mux.Handle("GET /superadmin/api/dashboard/chat/sessions/{id}/messages", auth.protect(handleDashboardChatGetMessages(chatStore)))
		goalServiceAddr := strings.TrimSuffix(cfg.GoalServiceAddr, "/")
		mux.Handle("POST /superadmin/api/dashboard/chat/sessions/{id}/messages", auth.protect(handleDashboardChatAppendMessage(chatStore, goalServiceAddr, database, llmBackend, memStore)))
		// Internal endpoint for Slack worker: append message and get assistant reply (auth via X-Slack-Internal-Secret).
		mux.Handle("POST /internal/slack/chat/message", handleInternalSlackChatMessage(chatStore, goalServiceAddr, database, llmBackend, memStore))
		mux.Handle("POST /internal/slack/post", handleInternalSlackPost(slackStore))
	}
	if agentClient != nil {
		mux.Handle("POST /internal/ingest/event", handleInternalIngestEvent())
	}
	mux.Handle("GET /internal/ingest/bindings", handleInternalIngestBindings())
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

// handleInternalSlackChatMessage is called by the Slack worker. It accepts org_id, agent_id, user_id, optional session_id, and message;
// appends the user message, runs chat (REST or goal), and returns assistant content. Auth: X-Slack-Internal-Secret header.
func handleInternalSlackChatMessage(chatStore *chat.Store, goalServiceAddr string, db *sql.DB, llmBackend *llm.EndpointBackend, memStore *memory.Store) http.HandlerFunc {
	secret := os.Getenv("ASTRA_SLACK_INTERNAL_SECRET")
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if secret != "" && r.Header.Get("X-Slack-Internal-Secret") != secret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			OrgID     string `json:"org_id"`
			AgentID   string `json:"agent_id"`
			UserID    string `json:"user_id"`
			SessionID string `json:"session_id,omitempty"`
			Message   string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.UserID == "" || req.AgentID == "" || req.Message == "" {
			http.Error(w, "user_id, agent_id, and message required", http.StatusBadRequest)
			return
		}
		agentID, err := uuid.Parse(req.AgentID)
		if err != nil {
			http.Error(w, "invalid agent_id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()

		var sessionID uuid.UUID
		if req.SessionID != "" {
			sessionID, err = uuid.Parse(req.SessionID)
			if err != nil {
				http.Error(w, "invalid session_id", http.StatusBadRequest)
				return
			}
			se, err := chatStore.GetSession(ctx, sessionID, req.UserID)
			if err != nil || se == nil {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
		} else {
			se, err := chatStore.CreateSession(ctx, req.UserID, agentID, "")
			if err != nil {
				slog.Error("internal slack chat CreateSession failed", "err", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			sessionID = se.ID
		}

		_, err = chatStore.AppendMessage(ctx, sessionID, req.UserID, "user", req.Message, nil, nil, 0, 0)
		if err != nil {
			slog.Error("internal slack chat AppendMessage failed", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var assistantMsg *chat.Message
		if chat.NeedsGoalWorkflow(req.Message) {
			assistantMsg, err = chat.ProcessGoalMessage(ctx, chatStore, goalServiceAddr, agentID, sessionID, req.UserID, req.Message)
		} else {
			directCtx, directCancel := context.WithTimeout(ctx, 60*time.Second)
			defer directCancel()
			assistantMsg, err = chat.ProcessRESTMessage(directCtx, chatStore, db, llmBackend, sessionID, req.UserID, memStore)
		}
		if err != nil {
			slog.Error("internal slack chat process failed", "err", err)
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"session_id":        sessionID.String(),
				"assistant_content": "Sorry, I couldn't process that: " + err.Error(),
			})
			return
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]any{
			"session_id":        sessionID.String(),
			"assistant_content": assistantMsg.Content,
		})
	}
}

// handleInternalIngestBindings returns GET /internal/ingest/bindings: list of agents with their ingest source (for adapters). Auth: X-Ingest-Secret.
func handleInternalIngestBindings() http.HandlerFunc {
	secret := os.Getenv("ASTRA_INGEST_SECRET")
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
			return
		}
		if secret != "" && r.Header.Get("X-Ingest-Secret") != secret {
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		bindings, err := docStore.GetIngestBindings(ctx)
		if err != nil {
			slog.Error("ingest bindings failed", "err", err)
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if bindings == nil {
			bindings = []agentdocs.IngestBinding{}
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]interface{}{"bindings": bindings})
	}
}

// ingestRateLimiter limits POST /internal/ingest/event by agent_id (default 100/min per agent). Configurable via ASTRA_INGEST_RATE_LIMIT (e.g. "100").
type ingestRateLimiter struct {
	mu       sync.Mutex
	perAgent map[string][]time.Time
	limit    int
	window   time.Duration
}

func newIngestRateLimiter() *ingestRateLimiter {
	limit := 100
	if s := os.Getenv("ASTRA_INGEST_RATE_LIMIT"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	return &ingestRateLimiter{perAgent: make(map[string][]time.Time), limit: limit, window: time.Minute}
}

func (rl *ingestRateLimiter) allow(agentID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-rl.window)
	var kept []time.Time
	for _, t := range rl.perAgent[agentID] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rl.limit {
		return false
	}
	kept = append(kept, now)
	rl.perAgent[agentID] = kept
	return true
}

var ingestRL = newIngestRateLimiter()

// handleInternalIngestEvent handles POST /internal/ingest/event for external event ingest (GCP Pub/Sub, external Redis, WebSocket adapters). Auth: X-Ingest-Secret.
func handleInternalIngestEvent() http.HandlerFunc {
	secret := os.Getenv("ASTRA_INGEST_SECRET")
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
			return
		}
		if secret != "" && r.Header.Get("X-Ingest-Secret") != secret {
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		var req struct {
			AgentID     string          `json:"agent_id"`
			MessageType string          `json:"message_type,omitempty"`
			Payload     json.RawMessage `json:"payload,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
			return
		}
		if req.AgentID == "" {
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "agent_id required"})
			return
		}
		if !ingestRL.allow(req.AgentID) {
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded for agent"})
			return
		}
		agentID := req.AgentID
		messageType := req.MessageType
		if messageType == "" {
			messageType = "ExternalEvent"
		}
		payload := req.Payload
		if payload == nil {
			payload = []byte("{}")
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		_, err := agentClient.SendMessage(ctx, &kernel_pb.SendMessageRequest{
			TargetActorId: agentID,
			MessageType:   messageType,
			Source:        "ingest",
			Payload:       payload,
		})
		if err != nil {
			slog.Error("ingest SendMessage failed", "agent_id", agentID, "err", err)
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

// handleInternalSlackPost handles POST /internal/slack/post for proactive Slack messages. Auth: X-Slack-Internal-Secret. Single-platform: uses default workspace.
func handleInternalSlackPost(slackStore *slack.Store) http.HandlerFunc {
	secret := os.Getenv("ASTRA_SLACK_INTERNAL_SECRET")
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if secret != "" && r.Header.Get("X-Slack-Internal-Secret") != secret {
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		var req struct {
			ChannelID string `json:"channel_id,omitempty"`
			Text      string `json:"text"`
			ThreadTs  string `json:"thread_ts,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Text == "" {
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "text required"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if err := slack.PostMessage(ctx, slackStore, &http.Client{}, req.ChannelID, req.Text, req.ThreadTs); err != nil {
			slog.Error("internal slack post failed", "err", err)
			w.Header().Set(headerContentType, contentTypeJSON)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

func handleSlackConfigGet(store *slack.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		out := map[string]string{
			"signing_secret":     "",
			"client_id":          "",
			"client_secret":      "",
			"oauth_redirect_url": "",
		}
		for k := range out {
			val, _ := store.GetConfig(ctx, k)
			if val != "" && (k == "client_secret" || k == "signing_secret") {
				val = "********"
			}
			out[k] = val
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(out)
	}
}

func handleSlackConfigPut(store *slack.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		keys := []string{slack.ConfigKeySigningSecret, slack.ConfigKeyClientID, slack.ConfigKeyClientSecret, slack.ConfigKeyOAuthRedirectURL}
		for _, k := range keys {
			if v, ok := body[k]; ok && v != "" {
				if err := store.SetConfig(ctx, k, v); err != nil {
					slog.Error("slack config set failed", "key", k, "err", err)
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
		_, _ = io.Copy(w, resp.Body)
		return
	}
	_, _ = io.Copy(w, resp.Body)
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
	if v := r.Header.Get("X-User-Id"); v != "" {
		req.Header.Set("X-User-Id", v)
	}
	if r.Header.Get("X-Is-Super-Admin") == "true" {
		req.Header.Set("X-Is-Super-Admin", "true")
	}
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

// kernelCtxWithAuth returns context for kernel calls (single-platform: no org scoping).
func kernelCtxWithAuth(r *http.Request, ctx context.Context) context.Context {
	return ctx
}

func handleListAgents(w http.ResponseWriter, r *http.Request) {
	// Route is registered as "GET /agents" so only GET reaches this handler.
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	ctx = kernelCtxWithAuth(r, ctx)
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
			"id":         row.ID,
			"actor_type": row.ActorType,
			"name":       row.Name,
			"status":     row.Status,
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
		ChatCapable  bool   `json:"chat_capable"`
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
	agentID, parseErr := uuid.Parse(resp.ActorId)
	if parseErr != nil {
		http.Error(w, "invalid agent id from spawn", http.StatusInternalServerError)
		return
	}

	if req.SystemPrompt != "" && docStore != nil {
		_ = docStore.UpdateProfile(ctx, agentID, &req.SystemPrompt, nil)
		docStore.SetAgentPromptCache(ctx, agentID, req.SystemPrompt)
	}
	if req.Name != "" && docStore != nil {
		_ = docStore.UpdateAgentName(ctx, agentID, req.Name)
	}
	if docStore != nil && req.ChatCapable {
		_ = docStore.UpdateAgentMeta(ctx, agentID, "", req.ChatCapable, nil, nil, nil)
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]string{"actor_id": resp.ActorId})
}

func handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		writeJSONError(w, http.StatusBadRequest, "agent id required")
		return
	}
	var req struct {
		Name                      *string          `json:"name"`
		SystemPrompt              *string          `json:"system_prompt"`
		Config                    *json.RawMessage `json:"config"`
		Status                    *string          `json:"status"`
		ChatCapable               *bool            `json:"chat_capable"`
		IngestSourceType          *string          `json:"ingest_source_type"`
		IngestSourceConfig        *json.RawMessage `json:"ingest_source_config"`
		SlackNotificationsEnabled *bool            `json:"slack_notifications_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	id, err := uuid.Parse(agentID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid agent id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if req.Name != nil {
		if err := docStore.UpdateAgentName(ctx, id, *req.Name); err != nil {
			slog.Error("UpdateAgentName failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "update name failed: "+err.Error())
			return
		}
	}
	if req.ChatCapable != nil {
		chatVal := *req.ChatCapable
		if err := docStore.UpdateAgentMeta(ctx, id, "", chatVal, nil, nil, nil); err != nil {
			slog.Error("UpdateAgentMeta failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "update chat_capable failed: "+err.Error())
			return
		}
	}
	if req.Status != nil {
		if err := docStore.UpdateAgentStatus(ctx, id, *req.Status); err != nil {
			slog.Error("UpdateAgentStatus failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "update status failed: "+err.Error())
			return
		}
	}
	if req.IngestSourceType != nil {
		sourceType := strings.TrimSpace(*req.IngestSourceType)
		var cfg json.RawMessage
		if req.IngestSourceConfig != nil {
			cfg = *req.IngestSourceConfig
		}
		if err := docStore.UpdateAgentIngestSource(ctx, id, sourceType, cfg); err != nil {
			slog.Error("UpdateAgentIngestSource failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "update ingest source failed: "+err.Error())
			return
		}
	}
	if req.SlackNotificationsEnabled != nil {
		if err := docStore.UpdateAgentSlackNotifications(ctx, id, *req.SlackNotificationsEnabled); err != nil {
			slog.Error("UpdateAgentSlackNotifications failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "update slack notifications failed: "+err.Error())
			return
		}
	}
	if err := docStore.UpdateProfile(ctx, id, req.SystemPrompt, req.Config); err != nil {
		slog.Error("UpdateProfile failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "update failed: "+err.Error())
		return
	}
	if req.SystemPrompt != nil {
		docStore.SetAgentPromptCache(ctx, id, *req.SystemPrompt)
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
	out := map[string]interface{}{
		"id":                          profile.ID.String(),
		"name":                        profile.Name,
		"system_prompt":               profile.SystemPrompt,
		"config":                      profile.Config,
		"chat_capable":                profile.ChatCapable,
		"slack_notifications_enabled": profile.SlackNotificationsEnabled,
	}
	if profile.ActorType != "" {
		out["actor_type"] = profile.ActorType
	}
	if profile.IngestSourceType != "" {
		out["ingest_source_type"] = profile.IngestSourceType
	}
	if len(profile.IngestSourceConfig) > 0 {
		out["ingest_source_config"] = profile.IngestSourceConfig
	}
	json.NewEncoder(w).Encode(out)
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
		AgentID:  agentUUID,
		DocType:  dt,
		Name:     req.Name,
		Content:  req.Content,
		URI:      req.URI,
		Metadata: req.Metadata,
		Priority: req.Priority,
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
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
