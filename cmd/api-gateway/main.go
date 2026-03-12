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
	"time"

	"astra/internal/agentdocs"
	"astra/internal/chat"
	"astra/internal/dashboard"
	"astra/internal/identity"
	"astra/internal/llm"
	"astra/internal/memory"
	"astra/internal/orgs"
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
	registerDashboardRoutes(mux, cfg, dashCollector, dashboardClient, docStore, chatStore, database, llmBackend, memoryStore, auth)
	orgStore := orgs.NewStore(database)
	identityStore := identity.NewStore(database)
	registerMultiTenantRoutes(mux, cfg, orgStore, identityStore, dashboardClient, database, auth)
	mux.HandleFunc("GET /login", handleLoginPage)
	mux.HandleFunc("POST /login", makeLoginProxyHandler(dashboardClient, cfg.IdentityAddr))
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
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusFound)
	})

	handler := orgSlugInterceptor(mux, orgStore, dashboardClient, cfg.IdentityAddr)
	addr := fmt.Sprintf(":%d", cfg.HTTPPort)
	slog.Info("api gateway started", "addr", addr)
	srv := &http.Server{Addr: addr, Handler: handler}
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
			OrgID        string   `json:"org_id"`
			OrgRole      string   `json:"org_role"`
			TeamIDs      []string `json:"team_ids"`
			IsSuperAdmin bool     `json:"is_super_admin"`
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

		r.Header.Set("X-User-Id", valRes.UserID)
		r.Header.Set("X-Org-Id", valRes.OrgID)
		r.Header.Set("X-Org-Role", valRes.OrgRole)
		r.Header.Set("X-Email", valRes.Email)
		if valRes.IsSuperAdmin {
			r.Header.Set("X-Is-Super-Admin", "true")
		}
		if len(valRes.TeamIDs) > 0 {
			r.Header.Set("X-Team-Ids", strings.Join(valRes.TeamIDs, ","))
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
	w.Write([]byte(loginPageHTML))
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
		io.Copy(w, resp.Body)
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

var reservedPaths = map[string]bool{
	"login": true, "logout": true, "health": true, "agents": true, "tasks": true,
	"graphs": true, "superadmin": true, "org": true, "api": true, "dashboard": true,
	"chat": true, "goals": true, "":true,
}

func orgSlugInterceptor(mux http.Handler, orgStore *orgs.Store, client *http.Client, identityAddr string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
		first := parts[0]
		if first == "" || reservedPaths[first] {
			mux.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		org, err := orgStore.GetOrgBySlug(ctx, first)
		if err != nil || org == nil {
			mux.ServeHTTP(w, r)
			return
		}
		rest := ""
		if len(parts) > 1 {
			rest = "/" + parts[1]
		}
		switch {
		case r.Method == "POST" && rest == "/login":
			handleOrgLoginPost(w, r, org, client, identityAddr, ctx)
		case r.Method == "GET" && rest == "/dashboard":
			handleOrgDashboardPage(w, org)
		case r.Method == "GET" && (rest == "" || rest == "/"):
			handleOrgLoginPage(w, org)
		default:
			mux.ServeHTTP(w, r)
		}
	})
}

func handleOrgLoginPage(w http.ResponseWriter, org *orgs.Organization) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := strings.ReplaceAll(orgLoginPageHTML, "{{ORG_NAME}}", org.Name)
	html = strings.ReplaceAll(html, "{{ORG_SLUG}}", org.Slug)
	html = strings.ReplaceAll(html, "{{ORG_ID}}", org.ID.String())
	w.Write([]byte(html))
}

func handleOrgLoginPost(w http.ResponseWriter, r *http.Request, org *orgs.Organization, client *http.Client, identityAddr string, ctx context.Context) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
	var loginReq map[string]interface{}
	json.Unmarshal(body, &loginReq)
	loginReq["org_id"] = org.ID.String()
	enriched, _ := json.Marshal(loginReq)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(identityAddr, "/")+"/users/login", bytes.NewReader(enriched))
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
	io.Copy(w, resp.Body)
}

func handleOrgDashboardPage(w http.ResponseWriter, org *orgs.Organization) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := strings.ReplaceAll(orgDashboardHTML, "{{ORG_NAME}}", org.Name)
	html = strings.ReplaceAll(html, "{{ORG_SLUG}}", org.Slug)
	html = strings.ReplaceAll(html, "{{ORG_ID}}", org.ID.String())
	w.Write([]byte(html))
}

func makeOrgDashboardHandler(orgStore *orgs.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		org, err := orgStore.GetOrgBySlug(ctx, slug)
		if err != nil || org == nil {
			http.Error(w, "Organization not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := strings.ReplaceAll(orgDashboardHTML, "{{ORG_NAME}}", org.Name)
		html = strings.ReplaceAll(html, "{{ORG_SLUG}}", org.Slug)
		html = strings.ReplaceAll(html, "{{ORG_ID}}", org.ID.String())
		w.Write([]byte(html))
	}
}

const orgDashboardHTML = `<!doctype html>
<html lang="en" data-theme="dark">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>{{ORG_NAME}} — Dashboard</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet"/>
<style>
*{box-sizing:border-box;margin:0;padding:0}
:root{--bg:#0f1117;--surface:#1a1d27;--surface2:#242736;--border:#2e3348;
  --text:#e4e6f0;--text2:#9399b2;--primary:#6c8aff;--primary-hover:#8ba3ff;
  --error:#f87171;--success:#34d399;--warn:#fbbf24}
body{background:var(--bg);color:var(--text);font-family:'Inter',system-ui,sans-serif;min-height:100vh}
.hdr{display:flex;justify-content:space-between;align-items:center;padding:16px 24px;background:var(--surface);border-bottom:1px solid var(--border)}
.hdr h1{font-size:1.25rem;font-weight:700}.hdr h1 span{color:var(--primary)}
.badge{display:inline-block;background:rgba(108,138,255,.15);border:1px solid rgba(108,138,255,.3);border-radius:6px;padding:2px 10px;font-size:.75rem;color:var(--primary);margin-left:8px;vertical-align:middle}
.btn{padding:8px 16px;background:var(--primary);color:#fff;border:none;border-radius:8px;font-size:.8125rem;font-weight:600;cursor:pointer;transition:background .15s}
.btn:hover{background:var(--primary-hover)}.btn-sm{padding:6px 12px;font-size:.75rem}
.btn-ghost{background:transparent;border:1px solid var(--border);color:var(--text2)}.btn-ghost:hover{background:var(--surface2);color:var(--text)}
.nav{display:flex;gap:0;padding:0 24px;background:var(--surface);border-bottom:1px solid var(--border)}
.nav button{padding:12px 20px;background:none;border:none;color:var(--text2);font-size:.875rem;font-weight:500;cursor:pointer;border-bottom:2px solid transparent;transition:all .15s}
.nav button:hover{color:var(--text)}.nav button.active{color:var(--primary);border-bottom-color:var(--primary)}
.content{padding:24px;max-width:1200px;margin:0 auto}
.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:16px;margin-bottom:24px}
.card{background:var(--surface);border:1px solid var(--border);border-radius:12px;padding:20px}
.card .label{font-size:.75rem;color:var(--text2);text-transform:uppercase;letter-spacing:.05em;margin-bottom:4px}
.card .value{font-size:1.75rem;font-weight:700}
.panel{background:var(--surface);border:1px solid var(--border);border-radius:12px;padding:20px;margin-bottom:16px}
.panel-hdr{display:flex;justify-content:space-between;align-items:center;margin-bottom:16px}
.panel-hdr h2{font-size:1rem;font-weight:600}
table{width:100%;border-collapse:collapse;font-size:.8125rem}
th{text-align:left;color:var(--text2);font-weight:500;padding:8px 12px;border-bottom:1px solid var(--border)}
td{padding:10px 12px;border-bottom:1px solid rgba(46,51,72,.5)}
tr:hover td{background:rgba(108,138,255,.03)}
.st{display:inline-block;padding:2px 8px;border-radius:4px;font-size:.6875rem;font-weight:600;text-transform:uppercase}
.st-active,.st-running{background:rgba(52,211,153,.15);color:var(--success)}
.st-idle,.st-stopped,.st-unknown{background:rgba(147,153,178,.15);color:var(--text2)}
.st-error{background:rgba(248,113,113,.15);color:var(--error)}
.empty{text-align:center;padding:40px;color:var(--text2);font-size:.875rem}
.mo{display:none;position:fixed;inset:0;background:rgba(0,0,0,.6);z-index:100;align-items:center;justify-content:center}
.mo.open{display:flex}
.modal{background:var(--surface);border:1px solid var(--border);border-radius:16px;padding:32px;width:100%;max-width:440px}
.modal h3{font-size:1.125rem;font-weight:600;margin-bottom:20px}
.field{margin-bottom:16px}
.field label{display:block;font-size:.8125rem;font-weight:500;color:var(--text2);margin-bottom:6px}
.field input,.field select{width:100%;padding:10px 14px;background:var(--surface2);border:1px solid var(--border);border-radius:8px;color:var(--text);font-size:.875rem;outline:none}
.field input:focus,.field select:focus{border-color:var(--primary)}
.mact{display:flex;gap:8px;justify-content:flex-end;margin-top:20px}
.tab{display:none}.tab.active{display:block}
.pag{display:flex;gap:8px;justify-content:center;margin-top:16px}
code{background:var(--surface2);padding:2px 6px;border-radius:4px;font-size:.75rem}
</style>
</head>
<body>
<div class="hdr">
  <div><h1><span>{{ORG_NAME}}</span> Dashboard</h1><span class="badge">{{ORG_SLUG}}</span></div>
  <button class="btn btn-ghost" onclick="localStorage.clear();location.href='/{{ORG_SLUG}}'">Sign out</button>
</div>
<div class="nav" id="nav">
  <button class="active" data-tab="overview">Overview</button>
  <button data-tab="agents">Agents</button>
  <button data-tab="goals">Goals</button>
  <button data-tab="teams">Teams</button>
  <button data-tab="members">Members</button>
</div>
<div class="content">
  <div class="tab active" id="tab-overview">
    <div class="cards">
      <div class="card"><div class="label">Agents</div><div class="value" id="s-ag">&#8212;</div></div>
      <div class="card"><div class="label">Teams</div><div class="value" id="s-tm">&#8212;</div></div>
      <div class="card"><div class="label">Members</div><div class="value" id="s-mb">&#8212;</div></div>
      <div class="card"><div class="label">Goals</div><div class="value" id="s-gl">&#8212;</div></div>
    </div>
  </div>
  <div class="tab" id="tab-agents">
    <div class="panel"><div class="panel-hdr"><h2>Agents</h2></div>
    <table><thead><tr><th>Name</th><th>Type</th><th>Status</th><th>Visibility</th><th>ID</th></tr></thead>
    <tbody id="ag-tb"></tbody></table>
    <div id="ag-e" class="empty" style="display:none">No agents found</div>
    <div class="pag" id="ag-pg"></div></div>
  </div>
  <div class="tab" id="tab-goals">
    <div class="panel"><div class="panel-hdr"><h2>Goals</h2></div>
    <div class="empty">Org-scoped goal listing coming soon.<br/>Use the <a href="/superadmin/dashboard/" style="color:var(--primary)">super-admin dashboard</a> for goal management.</div></div>
  </div>
  <div class="tab" id="tab-teams">
    <div class="panel"><div class="panel-hdr"><h2>Teams</h2><button class="btn btn-sm" onclick="openM('tm-mo')">Create Team</button></div>
    <table><thead><tr><th>Name</th><th>Slug</th><th>Description</th><th>Created</th></tr></thead>
    <tbody id="tm-tb"></tbody></table>
    <div id="tm-e" class="empty" style="display:none">No teams yet</div></div>
  </div>
  <div class="tab" id="tab-members">
    <div class="panel"><div class="panel-hdr"><h2>Members</h2><button class="btn btn-sm" onclick="openM('mb-mo')">Invite Member</button></div>
    <table><thead><tr><th>Name</th><th>Email</th><th>Role</th><th>Joined</th></tr></thead>
    <tbody id="mb-tb"></tbody></table>
    <div id="mb-e" class="empty" style="display:none">No members yet</div></div>
  </div>
</div>
<div class="mo" id="tm-mo"><div class="modal"><h3>Create Team</h3>
  <div class="field"><label>Name</label><input id="tf-n" placeholder="Engineering"/></div>
  <div class="field"><label>Slug</label><input id="tf-s" placeholder="engineering"/></div>
  <div class="mact"><button class="btn btn-ghost" onclick="closeM('tm-mo')">Cancel</button><button class="btn" onclick="mkTeam()">Create</button></div>
</div></div>
<div class="mo" id="mb-mo"><div class="modal"><h3>Invite Member</h3>
  <div class="field"><label>Name</label><input id="mf-n" placeholder="Jane Doe"/></div>
  <div class="field"><label>Email</label><input id="mf-e" type="email" placeholder="jane@company.com"/></div>
  <div class="field"><label>Password</label><input id="mf-p" type="password" placeholder="Temporary password"/></div>
  <div class="field"><label>Role</label><select id="mf-r"><option value="member">Member</option><option value="admin">Admin</option></select></div>
  <div class="mact"><button class="btn btn-ghost" onclick="closeM('mb-mo')">Cancel</button><button class="btn" onclick="mkMember()">Invite</button></div>
</div></div>
<script>
(function(){
var OID='{{ORG_ID}}',SLG='{{ORG_SLUG}}',tok=localStorage.getItem('astra_token');
if(!tok){location.href='/'+SLG;return}
function authFetch(u,o){o=o||{};o.headers=Object.assign({'Authorization':'Bearer '+tok,'Content-Type':'application/json'},o.headers||{});
  return fetch(u,o).then(function(r){if(r.status===401){localStorage.clear();location.href='/'+SLG;throw new Error('unauthorized')}return r.json()})}
function $(id){return document.getElementById(id)}
function esc(s){var d=document.createElement('div');d.textContent=s;return d.innerHTML}
document.getElementById('nav').onclick=function(e){if(e.target.tagName!=='BUTTON')return;
  document.querySelectorAll('.nav button').forEach(function(b){b.classList.remove('active')});
  document.querySelectorAll('.tab').forEach(function(t){t.classList.remove('active')});
  e.target.classList.add('active');var t=$('tab-'+e.target.dataset.tab);if(t)t.classList.add('active');
  var d=e.target.dataset.tab;if(d==='agents')ldAg();if(d==='teams')ldTm();if(d==='members')ldMb()};
window.openM=function(id){$(id).classList.add('open')};
window.closeM=function(id){$(id).classList.remove('open')};
function ldOv(){
  authFetch('/agents').then(function(d){$('s-ag').textContent=(d.agents||[]).length}).catch(function(){$('s-ag').textContent='?'});
  authFetch('/org/api/teams?org_id='+OID).then(function(d){$('s-tm').textContent=(d.teams||[]).length}).catch(function(){$('s-tm').textContent='?'});
  authFetch('/org/api/members?org_id='+OID).then(function(d){$('s-mb').textContent=(d.members||[]).length}).catch(function(){$('s-mb').textContent='?'});
  $('s-gl').textContent='\u2014';
}ldOv();
var agPg=0,agSz=20;
window.ldAg=function(){authFetch('/agents').then(function(d){
  var rw=d.agents||[],tb=$('ag-tb'),em=$('ag-e');tb.innerHTML='';
  if(!rw.length){em.style.display='block';return}em.style.display='none';
  var st=agPg*agSz,pg=rw.slice(st,st+agSz);
  pg.forEach(function(a){var tr=document.createElement('tr');
    tr.innerHTML='<td>'+esc(a.name||'\u2014')+'</td><td>'+esc(a.actor_type||'\u2014')+'</td><td><span class="st st-'+(a.status||'unknown')+'">'+esc(a.status||'unknown')+'</span></td><td>'+esc(a.visibility||'\u2014')+'</td><td style="font-family:monospace;font-size:.75rem;color:var(--text2)">'+esc((a.id||'').substring(0,8))+'</td>';
    tb.appendChild(tr)});
  var p=$('ag-pg'),pages=Math.ceil(rw.length/agSz);p.innerHTML='';
  for(var i=0;i<pages;i++){var b=document.createElement('button');b.className='btn btn-sm'+(i===agPg?'':' btn-ghost');b.textContent=i+1;b.dataset.p=i;
    b.onclick=function(){agPg=+this.dataset.p;ldAg()};p.appendChild(b)}
}).catch(function(){})};
window.ldTm=function(){authFetch('/org/api/teams?org_id='+OID).then(function(d){
  var rw=d.teams||[],tb=$('tm-tb'),em=$('tm-e');tb.innerHTML='';
  if(!rw.length){em.style.display='block';return}em.style.display='none';
  rw.forEach(function(t){var tr=document.createElement('tr');
    tr.innerHTML='<td>'+esc(t.name)+'</td><td><code>'+esc(t.slug)+'</code></td><td>'+esc(t.description||'\u2014')+'</td><td>'+new Date(t.created_at).toLocaleDateString()+'</td>';
    tb.appendChild(tr)});
}).catch(function(){})};
window.mkTeam=function(){var n=$('tf-n').value,s=$('tf-s').value;
  if(!n||!s)return alert('Name and slug required');
  authFetch('/org/api/teams?org_id='+OID,{method:'POST',body:JSON.stringify({name:n,slug:s})}).then(function(){
    closeM('tm-mo');$('tf-n').value='';$('tf-s').value='';ldTm();ldOv()}).catch(function(e){alert('Error: '+e.message)})};
window.ldMb=function(){authFetch('/org/api/members?org_id='+OID).then(function(d){
  var rw=d.members||[],tb=$('mb-tb'),em=$('mb-e');tb.innerHTML='';
  if(!rw.length){em.style.display='block';return}em.style.display='none';
  rw.forEach(function(m){var tr=document.createElement('tr');
    tr.innerHTML='<td>'+esc(m.name)+'</td><td>'+esc(m.email)+'</td><td><span class="badge">'+esc(m.role)+'</span></td><td>'+new Date(m.created_at).toLocaleDateString()+'</td>';
    tb.appendChild(tr)});
}).catch(function(){})};
window.mkMember=function(){var n=$('mf-n').value,e=$('mf-e').value,p=$('mf-p').value,r=$('mf-r').value;
  if(!n||!e||!p)return alert('All fields required');
  authFetch('/org/api/members?org_id='+OID,{method:'POST',body:JSON.stringify({name:n,email:e,password:p,role:r})}).then(function(){
    closeM('mb-mo');$('mf-n').value='';$('mf-e').value='';$('mf-p').value='';ldMb();ldOv()}).catch(function(er){alert('Error: '+er.message)})};
})();
</script>
</body>
</html>`

const orgLoginPageHTML = `<!doctype html>
<html lang="en" data-theme="dark">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>{{ORG_NAME}} — Astra</title>
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
.logo{font-size:1.75rem;font-weight:700;text-align:center;margin-bottom:4px;letter-spacing:-.02em}
.logo span{color:var(--primary)}
.org-name{text-align:center;font-size:1.1rem;font-weight:600;color:var(--text);margin-bottom:4px}
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
.org-badge-top{display:inline-block;background:rgba(108,138,255,.15);border:1px solid rgba(108,138,255,.3);
  border-radius:6px;padding:2px 10px;font-size:.75rem;color:var(--primary);margin-bottom:16px}
</style>
</head>
<body>
<div class="login-card">
  <div class="logo"><span>Astra</span></div>
  <div class="org-name">{{ORG_NAME}}</div>
  <div class="subtitle">Sign in to your organization</div>
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
    <a href="/login">Platform login</a>
  </div>
</div>
<script>
(function(){
  var form=document.getElementById('login-form');
  var msg=document.getElementById('msg');
  var btn=document.getElementById('btn-login');
  var orgSlug='{{ORG_SLUG}}';
  form.addEventListener('submit',function(e){
    e.preventDefault();
    btn.disabled=true;btn.textContent='Signing in...';
    msg.className='msg';msg.style.display='none';
    var body=JSON.stringify({email:document.getElementById('email').value,password:document.getElementById('password').value});
    fetch('/'+orgSlug+'/login',{method:'POST',headers:{'Content-Type':'application/json'},body:body})
      .then(function(r){return r.json().then(function(d){return{ok:r.ok,data:d}})})
      .then(function(res){
        if(!res.ok){
          msg.className='msg error';msg.textContent=res.data.error||'Login failed';msg.style.display='block';
          btn.disabled=false;btn.textContent='Sign in';return;
        }
        var d=res.data;
        localStorage.setItem('astra_token',d.token||'');
        localStorage.setItem('astra_user',JSON.stringify(d.user||{}));
        if(d.org) localStorage.setItem('astra_org',JSON.stringify(d.org));
        msg.className='msg success';msg.textContent='Welcome, '+(d.user&&d.user.name||'User')+'!';msg.style.display='block';
        setTimeout(function(){ window.location.href='/'+orgSlug+'/dashboard'; },600);
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
	w.Write([]byte("ok"))
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
			io.Copy(w, resp.Body)
			return
		}
		io.Copy(w, resp.Body)
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

// ---------------------------------------------------------------------------
// Multi-Tenant Routes
// ---------------------------------------------------------------------------

func registerMultiTenantRoutes(mux *http.ServeMux, cfg *config.Config, orgStore *orgs.Store, identityStore *identity.Store, client *http.Client, database *sql.DB, auth *authMiddleware) {
	identityAddr := strings.TrimSuffix(cfg.IdentityAddr, "/")

	// --- Super-Admin Org Routes ---

	mux.Handle("GET /superadmin/api/orgs", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, offset := parsePagination(r)
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		list, total, err := orgStore.ListOrgs(ctx, limit, offset)
		if err != nil {
			slog.Error("ListOrgs failed", "err", err)
			http.Error(w, "list orgs failed", http.StatusInternalServerError)
			return
		}
		items := make([]map[string]any, len(list))
		for i, o := range list {
			items[i] = map[string]any{
				"id": o.ID.String(), "name": o.Name, "slug": o.Slug,
				"status": o.Status, "config": json.RawMessage(o.Config),
				"created_at": o.CreatedAt, "updated_at": o.UpdatedAt,
			}
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]any{"orgs": items, "total": total})
	})))

	mux.Handle("POST /superadmin/api/orgs", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Slug == "" {
			http.Error(w, "name and slug required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		org, err := orgStore.CreateOrg(ctx, req.Name, req.Slug)
		if err != nil {
			slog.Error("CreateOrg failed", "err", err)
			http.Error(w, "create org failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id": org.ID.String(), "name": org.Name, "slug": org.Slug,
			"status": org.Status, "created_at": org.CreatedAt, "updated_at": org.UpdatedAt,
		})
	})))

	mux.Handle("GET /superadmin/api/orgs/{id}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid org id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		org, err := orgStore.GetOrg(ctx, id)
		if err != nil {
			slog.Error("GetOrg failed", "err", err)
			http.Error(w, "get org failed", http.StatusInternalServerError)
			return
		}
		if org == nil {
			http.Error(w, "org not found", http.StatusNotFound)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]any{
			"id": org.ID.String(), "name": org.Name, "slug": org.Slug,
			"status": org.Status, "config": json.RawMessage(org.Config),
			"created_at": org.CreatedAt, "updated_at": org.UpdatedAt,
		})
	})))

	mux.Handle("PATCH /superadmin/api/orgs/{id}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid org id", http.StatusBadRequest)
			return
		}
		var req struct {
			Name   *string `json:"name"`
			Slug   *string `json:"slug"`
			Status *string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := orgStore.UpdateOrg(ctx, id, req.Name, req.Slug, req.Status); err != nil {
			slog.Error("UpdateOrg failed", "err", err)
			http.Error(w, "update org failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	mux.Handle("DELETE /superadmin/api/orgs/{id}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid org id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := orgStore.DeleteOrg(ctx, id); err != nil {
			slog.Error("DeleteOrg failed", "err", err)
			http.Error(w, "delete org failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	mux.Handle("POST /superadmin/api/orgs/{id}/admins", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid org id", http.StatusBadRequest)
			return
		}
		var req struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
			http.Error(w, "user_id required", http.StatusBadRequest)
			return
		}
		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			http.Error(w, "invalid user_id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := orgStore.AddOrgMember(ctx, orgID, userID, "admin"); err != nil {
			slog.Error("AddOrgMember admin failed", "err", err)
			http.Error(w, "add admin failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	mux.Handle("DELETE /superadmin/api/orgs/{id}/admins/{uid}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid org id", http.StatusBadRequest)
			return
		}
		userID, err := uuid.Parse(r.PathValue("uid"))
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := orgStore.RemoveOrgMember(ctx, orgID, userID); err != nil {
			slog.Error("RemoveOrgMember failed", "err", err)
			http.Error(w, "remove admin failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	// --- Super-Admin User Routes (proxy to identity service) ---

	mux.Handle("GET /superadmin/api/users", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyToIdentity(w, r, client, identityAddr, http.MethodGet, "/users")
	})))

	mux.Handle("POST /superadmin/api/users", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyToIdentity(w, r, client, identityAddr, http.MethodPost, "/users")
	})))

	mux.Handle("GET /superadmin/api/users/{id}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyToIdentity(w, r, client, identityAddr, http.MethodGet, "/users/"+r.PathValue("id"))
	})))

	mux.Handle("PATCH /superadmin/api/users/{id}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyToIdentity(w, r, client, identityAddr, http.MethodPatch, "/users/"+r.PathValue("id"))
	})))

	mux.Handle("POST /superadmin/api/users/{id}/reset-password", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyToIdentity(w, r, client, identityAddr, http.MethodPost, "/users/"+r.PathValue("id")+"/reset-password")
	})))

	// --- Super-Admin User-Org Management ---

	mux.Handle("GET /superadmin/api/users/{id}/orgs", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		memberships, err := identityStore.GetOrgMemberships(ctx, userID)
		if err != nil {
			slog.Error("GetOrgMemberships failed", "err", err)
			http.Error(w, "get memberships failed", http.StatusInternalServerError)
			return
		}
		items := make([]map[string]any, len(memberships))
		for i, m := range memberships {
			items[i] = map[string]any{
				"org_id": m.OrgID.String(), "org_name": m.OrgName,
				"org_slug": m.OrgSlug, "role": m.Role,
			}
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]any{"memberships": items})
	})))

	mux.Handle("POST /superadmin/api/users/{id}/orgs", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		var req struct {
			OrgID string `json:"org_id"`
			Role  string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OrgID == "" || req.Role == "" {
			http.Error(w, "org_id and role required", http.StatusBadRequest)
			return
		}
		orgID, err := uuid.Parse(req.OrgID)
		if err != nil {
			http.Error(w, "invalid org_id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := orgStore.AddOrgMember(ctx, orgID, userID, req.Role); err != nil {
			slog.Error("AddOrgMember failed", "err", err)
			http.Error(w, "add membership failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	mux.Handle("PATCH /superadmin/api/users/{id}/orgs/{oid}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		orgID, err := uuid.Parse(r.PathValue("oid"))
		if err != nil {
			http.Error(w, "invalid org id", http.StatusBadRequest)
			return
		}
		var req struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Role == "" {
			http.Error(w, "role required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := orgStore.UpdateOrgMemberRole(ctx, orgID, userID, req.Role); err != nil {
			slog.Error("UpdateOrgMemberRole failed", "err", err)
			http.Error(w, "update role failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	mux.Handle("DELETE /superadmin/api/users/{id}/orgs/{oid}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		orgID, err := uuid.Parse(r.PathValue("oid"))
		if err != nil {
			http.Error(w, "invalid org id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := orgStore.RemoveOrgMember(ctx, orgID, userID); err != nil {
			slog.Error("RemoveOrgMember failed", "err", err)
			http.Error(w, "remove membership failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	mux.Handle("POST /superadmin/api/users/{id}/suspend", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]string{"status": "suspended"})
		proxyToIdentityWithBody(w, client, identityAddr, http.MethodPatch, "/users/"+r.PathValue("id"), body, r.Context())
	})))

	mux.Handle("POST /superadmin/api/users/{id}/activate", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]string{"status": "active"})
		proxyToIdentityWithBody(w, client, identityAddr, http.MethodPatch, "/users/"+r.PathValue("id"), body, r.Context())
	})))

	// --- Org-Level Routes ---

	mux.Handle("GET /org/api/teams", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID, err := extractOrgID(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		teams, err := orgStore.ListTeams(ctx, orgID)
		if err != nil {
			slog.Error("ListTeams failed", "err", err)
			http.Error(w, "list teams failed", http.StatusInternalServerError)
			return
		}
		items := make([]map[string]any, len(teams))
		for i, t := range teams {
			items[i] = map[string]any{
				"id": t.ID.String(), "org_id": t.OrgID.String(),
				"name": t.Name, "slug": t.Slug,
				"description": t.Description, "created_at": t.CreatedAt,
			}
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]any{"teams": items})
	})))

	mux.Handle("POST /org/api/teams", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID, err := extractOrgID(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var req struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Slug == "" {
			http.Error(w, "name and slug required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		team, err := orgStore.CreateTeam(ctx, orgID, req.Name, req.Slug)
		if err != nil {
			slog.Error("CreateTeam failed", "err", err)
			http.Error(w, "create team failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id": team.ID.String(), "org_id": team.OrgID.String(),
			"name": team.Name, "slug": team.Slug,
			"description": team.Description, "created_at": team.CreatedAt,
		})
	})))

	mux.Handle("PATCH /org/api/teams/{id}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid team id", http.StatusBadRequest)
			return
		}
		var req struct {
			Name        *string `json:"name"`
			Slug        *string `json:"slug"`
			Description *string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := orgStore.UpdateTeam(ctx, id, req.Name, req.Slug, req.Description); err != nil {
			slog.Error("UpdateTeam failed", "err", err)
			http.Error(w, "update team failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	mux.Handle("DELETE /org/api/teams/{id}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid team id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := orgStore.DeleteTeam(ctx, id); err != nil {
			slog.Error("DeleteTeam failed", "err", err)
			http.Error(w, "delete team failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	mux.Handle("POST /org/api/teams/{id}/members", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		teamID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid team id", http.StatusBadRequest)
			return
		}
		var req struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
			http.Error(w, "user_id required", http.StatusBadRequest)
			return
		}
		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			http.Error(w, "invalid user_id", http.StatusBadRequest)
			return
		}
		if req.Role == "" {
			req.Role = "member"
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := orgStore.AddTeamMember(ctx, teamID, userID, req.Role); err != nil {
			slog.Error("AddTeamMember failed", "err", err)
			http.Error(w, "add team member failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	mux.Handle("DELETE /org/api/teams/{id}/members/{uid}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		teamID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid team id", http.StatusBadRequest)
			return
		}
		userID, err := uuid.Parse(r.PathValue("uid"))
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := orgStore.RemoveTeamMember(ctx, teamID, userID); err != nil {
			slog.Error("RemoveTeamMember failed", "err", err)
			http.Error(w, "remove team member failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	mux.Handle("GET /org/api/members", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID, err := extractOrgID(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		members, err := orgStore.ListOrgMembers(ctx, orgID)
		if err != nil {
			slog.Error("ListOrgMembers failed", "err", err)
			http.Error(w, "list members failed", http.StatusInternalServerError)
			return
		}
		items := make([]map[string]any, len(members))
		for i, m := range members {
			items[i] = map[string]any{
				"user_id": m.UserID.String(), "email": m.Email,
				"name": m.Name, "role": m.Role,
				"created_at": m.CreatedAt,
			}
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]any{"members": items})
	})))

	mux.Handle("POST /org/api/members", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID, err := extractOrgID(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var req struct {
			Email    string `json:"email"`
			Name     string `json:"name"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Name == "" || req.Password == "" {
			http.Error(w, "email, name, and password required", http.StatusBadRequest)
			return
		}
		if req.Role == "" {
			req.Role = "member"
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		user, err := identityStore.CreateUser(ctx, req.Email, req.Name, req.Password, false)
		if err != nil {
			slog.Error("CreateUser for org member failed", "err", err)
			http.Error(w, "create user failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := orgStore.AddOrgMember(ctx, orgID, user.ID, req.Role); err != nil {
			slog.Error("AddOrgMember failed", "err", err)
			http.Error(w, "add member failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"user_id": user.ID.String(), "email": user.Email,
			"name": user.Name, "role": req.Role,
		})
	})))

	mux.Handle("PATCH /org/api/members/{uid}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID, err := extractOrgID(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		userID, err := uuid.Parse(r.PathValue("uid"))
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		var req struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Role == "" {
			http.Error(w, "role required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := orgStore.UpdateOrgMemberRole(ctx, orgID, userID, req.Role); err != nil {
			slog.Error("UpdateOrgMemberRole failed", "err", err)
			http.Error(w, "update role failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	mux.Handle("DELETE /org/api/members/{uid}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID, err := extractOrgID(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		userID, err := uuid.Parse(r.PathValue("uid"))
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := orgStore.RemoveOrgMember(ctx, orgID, userID); err != nil {
			slog.Error("RemoveOrgMember failed", "err", err)
			http.Error(w, "remove member failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	// --- Org-Level Agent Creation ---

	mux.Handle("POST /org/api/agents", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID, err := extractOrgID(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		userID := r.Header.Get("X-User-Id")
		if userID == "" {
			userID = r.URL.Query().Get("user_id")
		}
		if userID == "" {
			http.Error(w, "user_id required (X-User-Id header or user_id query param)", http.StatusBadRequest)
			return
		}
		ownerUUID, err := uuid.Parse(userID)
		if err != nil {
			http.Error(w, "invalid user_id", http.StatusBadRequest)
			return
		}

		var req struct {
			Name         string `json:"name"`
			Visibility   string `json:"visibility"`
			TeamID       string `json:"team_id"`
			SystemPrompt string `json:"system_prompt"`
			Config       string `json:"config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		configBytes := []byte(req.Config)
		resp, err := agentClient.SpawnActor(ctx, &kernel_pb.SpawnActorRequest{
			ActorType: "agent",
			Config:    configBytes,
		})
		if err != nil {
			slog.Error("SpawnActor for org agent failed", "err", err)
			http.Error(w, "spawn failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		agentUUID, err := uuid.Parse(resp.ActorId)
		if err != nil {
			http.Error(w, "invalid actor id from spawn", http.StatusInternalServerError)
			return
		}

		if req.SystemPrompt != "" && docStore != nil {
			_ = docStore.UpdateProfile(ctx, agentUUID, &req.SystemPrompt, nil)
		}
		if req.Name != "" && docStore != nil {
			_ = docStore.UpdateAgentName(ctx, agentUUID, req.Name)
		}

		visibility := req.Visibility
		if visibility == "" {
			visibility = "private"
		}

		var teamID *uuid.UUID
		if req.TeamID != "" {
			t, err := uuid.Parse(req.TeamID)
			if err != nil {
				http.Error(w, "invalid team_id", http.StatusBadRequest)
				return
			}
			teamID = &t
		}

		_, err = database.ExecContext(ctx,
			`UPDATE agents SET org_id = $1, owner_id = $2, visibility = $3, team_id = $4 WHERE id = $5`,
			orgID, ownerUUID, visibility, teamID, agentUUID)
		if err != nil {
			slog.Error("update agent org metadata failed", "err", err, "agent_id", agentUUID)
			http.Error(w, "agent created but metadata update failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"actor_id":   resp.ActorId,
			"org_id":     orgID.String(),
			"owner_id":   ownerUUID.String(),
			"visibility": visibility,
		})
	})))

	// --- Agent Collaborator Routes ---

	mux.Handle("POST /org/api/agents/{id}/collaborators", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid agent id", http.StatusBadRequest)
			return
		}
		var req struct {
			CollaboratorType string `json:"collaborator_type"`
			CollaboratorID   string `json:"collaborator_id"`
			Permission       string `json:"permission"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.CollaboratorType != "user" && req.CollaboratorType != "team" {
			http.Error(w, "collaborator_type must be 'user' or 'team'", http.StatusBadRequest)
			return
		}
		collabID, err := uuid.Parse(req.CollaboratorID)
		if err != nil {
			http.Error(w, "invalid collaborator_id", http.StatusBadRequest)
			return
		}
		perm := req.Permission
		if perm == "" {
			perm = "use"
		}
		if perm != "use" && perm != "edit" && perm != "admin" {
			http.Error(w, "permission must be 'use', 'edit', or 'admin'", http.StatusBadRequest)
			return
		}

		newID := uuid.New()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		_, err = database.ExecContext(ctx,
			`INSERT INTO agent_collaborators (id, agent_id, collaborator_type, collaborator_id, permission) VALUES ($1, $2, $3, $4, $5)`,
			newID, agentID, req.CollaboratorType, collabID, perm)
		if err != nil {
			slog.Error("insert agent_collaborator failed", "err", err)
			http.Error(w, "add collaborator failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id": newID.String(), "agent_id": agentID.String(),
			"collaborator_type": req.CollaboratorType, "collaborator_id": collabID.String(),
			"permission": perm,
		})
	})))

	mux.Handle("GET /org/api/agents/{id}/collaborators", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid agent id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, err := database.QueryContext(ctx,
			`SELECT ac.id, ac.collaborator_type, ac.collaborator_id, ac.permission, ac.created_at FROM agent_collaborators ac WHERE ac.agent_id = $1`,
			agentID)
		if err != nil {
			slog.Error("query agent_collaborators failed", "err", err)
			http.Error(w, "list collaborators failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var items []map[string]any
		for rows.Next() {
			var id, collabType, collabID, perm string
			var createdAt time.Time
			if err := rows.Scan(&id, &collabType, &collabID, &perm, &createdAt); err != nil {
				slog.Error("scan agent_collaborator row failed", "err", err)
				continue
			}
			items = append(items, map[string]any{
				"id": id, "collaborator_type": collabType, "collaborator_id": collabID,
				"permission": perm, "created_at": createdAt,
			})
		}
		if err := rows.Err(); err != nil {
			slog.Error("iterate agent_collaborators failed", "err", err)
			http.Error(w, "list collaborators failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []map[string]any{}
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]any{"collaborators": items})
	})))

	mux.Handle("DELETE /org/api/agents/{id}/collaborators/{cid}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid agent id", http.StatusBadRequest)
			return
		}
		collabRowID, err := uuid.Parse(r.PathValue("cid"))
		if err != nil {
			http.Error(w, "invalid collaborator id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		result, err := database.ExecContext(ctx,
			`DELETE FROM agent_collaborators WHERE id = $1 AND agent_id = $2`,
			collabRowID, agentID)
		if err != nil {
			slog.Error("delete agent_collaborator failed", "err", err)
			http.Error(w, "delete collaborator failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			http.Error(w, "collaborator not found", http.StatusNotFound)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))

	// --- Agent Admin Routes ---

	mux.Handle("POST /org/api/agents/{id}/admins", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid agent id", http.StatusBadRequest)
			return
		}
		var req struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
			http.Error(w, "user_id required", http.StatusBadRequest)
			return
		}
		adminUserID, err := uuid.Parse(req.UserID)
		if err != nil {
			http.Error(w, "invalid user_id", http.StatusBadRequest)
			return
		}
		newID := uuid.New()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		_, err = database.ExecContext(ctx,
			`INSERT INTO agent_admins (id, agent_id, user_id) VALUES ($1, $2, $3)`,
			newID, agentID, adminUserID)
		if err != nil {
			slog.Error("insert agent_admin failed", "err", err)
			http.Error(w, "add agent admin failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id": newID.String(), "agent_id": agentID.String(), "user_id": adminUserID.String(),
		})
	})))

	mux.Handle("DELETE /org/api/agents/{id}/admins/{uid}", auth.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid agent id", http.StatusBadRequest)
			return
		}
		adminUserID, err := uuid.Parse(r.PathValue("uid"))
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		result, err := database.ExecContext(ctx,
			`DELETE FROM agent_admins WHERE agent_id = $1 AND user_id = $2`,
			agentID, adminUserID)
		if err != nil {
			slog.Error("delete agent_admin failed", "err", err)
			http.Error(w, "delete agent admin failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			http.Error(w, "agent admin not found", http.StatusNotFound)
			return
		}
		w.Header().Set(headerContentType, contentTypeJSON)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})))
}

func proxyToIdentity(w http.ResponseWriter, r *http.Request, client *http.Client, identityAddr, method, path string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var bodyReader io.Reader
	if r.Body != nil && method != http.MethodGet {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, identityAddr+path, bodyReader)
	if err != nil {
		http.Error(w, "request build failed", http.StatusInternalServerError)
		return
	}
	if bodyReader != nil {
		req.Header.Set(headerContentType, contentTypeJSON)
	}
	if method == http.MethodGet {
		req.URL.RawQuery = r.URL.RawQuery
	}

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("identity proxy failed", "method", method, "path", path, "err", err)
		http.Error(w, "identity service unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func proxyToIdentityWithBody(w http.ResponseWriter, client *http.Client, identityAddr, method, path string, body []byte, parentCtx context.Context) {
	ctx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, identityAddr+path, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "request build failed", http.StatusInternalServerError)
		return
	}
	req.Header.Set(headerContentType, contentTypeJSON)

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("identity proxy failed", "method", method, "path", path, "err", err)
		http.Error(w, "identity service unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func extractOrgID(r *http.Request) (uuid.UUID, error) {
	id := r.Header.Get("X-Org-Id")
	if id == "" {
		id = r.URL.Query().Get("org_id")
	}
	if id == "" {
		return uuid.Nil, fmt.Errorf("org_id required (X-Org-Id header or org_id query param)")
	}
	return uuid.Parse(id)
}

func parsePagination(r *http.Request) (limit, offset int) {
	limit = 50
	offset = 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return
}
