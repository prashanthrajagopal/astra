# Astra Platform Dashboard — Implementation Blueprint

**Version:** 1.0  
**Target:** Immediate implementation, no external frontend toolchain  
**Requirement:** Web UI served by Astra with live snapshot visibility

---

## 1. Overview

A single-page dashboard served by the API Gateway, providing platform visibility via a **live snapshot endpoint** and vanilla HTML/CSS/JS UI with auto-refresh. No npm, webpack, or React—pure Go `embed` for static assets.

---

## 2. Architecture Summary

| Component | Responsibility |
|-----------|----------------|
| **api-gateway** | Serves dashboard static assets (`/dashboard`, `/dashboard/*`) and aggregates snapshot data (`GET /api/dashboard/snapshot`) |
| **Snapshot endpoint** | Fan-out to worker-manager, access-control, cost-tracker; read logs/PIDs from filesystem |
| **Static UI** | Vanilla JS, fetches `/api/dashboard/snapshot` every 5s, renders services/workers/approvals/cost/logs |

---

## 3. Service & Port Matrix (Existing)

| Service | Port | Type | Health Endpoint |
|---------|------|------|----------------|
| api-gateway | 8080 | HTTP | GET /health |
| task-service | 9090 | gRPC | (health via TCP) |
| agent-service | 9091 | gRPC | (health via TCP) |
| scheduler-service | — | internal | — |
| worker-manager | 8082 | HTTP | GET /health |
| tool-runtime | 8083 | HTTP | GET /health |
| prompt-manager | 8084 | HTTP | GET /health |
| identity | 8085 | HTTP | GET /health |
| access-control | 8086 | HTTP | GET /health |
| planner-service | 8087 | HTTP | GET /health |
| goal-service | 8088 | HTTP | GET /health |
| evaluation-service | 8089 | HTTP | GET /health |
| cost-tracker | 8090 | HTTP | GET /health |
| memory-service | 9092 | gRPC | (health via TCP) |
| llm-router | 9093 | gRPC | (health via TCP) |

Log paths: `logs/<service>.log`  
PID paths: `logs/<service>.pid`

---

## 4. Exact File Paths to Create/Modify

### 4.1 New Files

| Path | Purpose |
|------|---------|
| `cmd/api-gateway/dashboard/index.html` | Single-page dashboard HTML |
| `cmd/api-gateway/dashboard/static/style.css` | Minimal layout, dark theme |
| `cmd/api-gateway/dashboard/static/app.js` | Fetch snapshot, render, auto-refresh 5s |
| `internal/dashboard/snapshot.go` | Snapshot aggregator (services, workers, approvals, cost, logs, PIDs) |

**Note:** Dashboard files live under `cmd/api-gateway/dashboard/` so `//go:embed` can reference them (Go embed cannot use `..` paths).

### 4.2 Modified Files

| Path | Changes |
|------|---------|
| `cmd/api-gateway/main.go` | Add `embed` for `dashboard`, route `GET /dashboard` and `GET /dashboard/*`, add `GET /api/dashboard/snapshot` handler |
| `pkg/config/config.go` | Add `LogsDir` (default `logs`), `WorkerManagerAddr`, `AccessControlAddr`, `CostTrackerAddr` (or derive from existing IdentityAddr pattern) |
| `scripts/deploy.sh` | No change (api-gateway already built/started) |
| `scripts/validate.sh` | Add Phase Dashboard: `curl GET /api/dashboard/snapshot` returns 200, JSON has `services`, `workers`, `approvals`, `cost`, `logs`, `pids` keys |

---

## 5. Snapshot Endpoint Contract

**Endpoint:** `GET /api/dashboard/snapshot`

**Auth:** None for local dev (dashboard is ops-internal). For production, use same JWT as other protected routes or a dedicated `dashboard` scope.

**Response (200 JSON):**

```json
{
  "services": [
    {
      "name": "api-gateway",
      "port": 8080,
      "type": "http",
      "healthy": true,
      "latency_ms": 2
    },
    {
      "name": "worker-manager",
      "port": 8082,
      "type": "http",
      "healthy": true,
      "latency_ms": 1
    }
  ],
  "workers": [
    {
      "id": "uuid",
      "hostname": "host.local",
      "status": "active",
      "capabilities": ["general"],
      "last_heartbeat": "2026-03-10T12:00:00Z"
    }
  ],
  "approvals": [
    {
      "id": "uuid",
      "tool_name": "terraform plan",
      "action_summary": "Infra change",
      "status": "pending",
      "requested_at": "2026-03-10T11:55:00Z"
    }
  ],
  "cost": {
    "rows": [
      {
        "day": "2026-03-10",
        "agent_id": "uuid",
        "model": "gpt-4",
        "tokens_in": 1000,
        "tokens_out": 200,
        "cost_dollars": 0.02
      }
    ]
  },
  "logs": {
    "api-gateway": ["line1", "line2"],
    "worker-manager": ["line1", "line2"]
  },
  "pids": {
    "api-gateway": 12345,
    "worker-manager": 12346
  }
}
```

---

## 6. Snapshot Aggregation Logic

`internal/dashboard/snapshot.go`:

1. **services** — For each known service (from config/env), issue HTTP `GET /health` or TCP connect (gRPC). Record `healthy` and `latency_ms`. Use `http.Client` with 2s timeout.
2. **workers** — HTTP GET `{WorkerManagerAddr}/workers`. Parse JSON, pass through.
3. **approvals** — HTTP GET `{AccessControlAddr}/approvals/pending`. Parse JSON, pass through.
4. **cost** — HTTP GET `{CostTrackerAddr}/cost/daily?days=7`. Parse JSON, pass through.
5. **logs** — For each service in a fixed list, read last 20 lines from `{LogsDir}/{service}.log`. If file missing, return empty array for that service.
6. **pids** — For each service, read `{LogsDir}/{service}.pid`, parse int. If missing/invalid, omit or use 0.

**Config additions:**
- `LogsDir` = `getEnv("ASTRA_LOGS_DIR", "logs")` (relative to CWD or absolute)
- `WorkerManagerAddr` = `getEnv("WORKER_MANAGER_ADDR", "http://localhost:8082")`
- `AccessControlAddr` = already exists
- `CostTrackerAddr` = `getEnv("COST_TRACKER_ADDR", "http://localhost:8090")`

---

## 7. Minimum UI Components

| Component | Data Source | Display |
|-----------|-------------|---------|
| Services | `snapshot.services` | Table: name, port, type, healthy (green/red), latency |
| Workers | `snapshot.workers` | Table: id, hostname, status, capabilities, last_heartbeat |
| Approvals | `snapshot.approvals` | Table: id, tool_name, status, requested_at; link to approve/deny (future) |
| Cost | `snapshot.cost.rows` | Table: day, agent_id, model, tokens, cost_dollars |
| Log tails | `snapshot.logs` | Collapsible sections per service, last N lines, monospace |
| PIDs | `snapshot.pids` | Inline with service name or small badge |

**Auto-refresh:** `setInterval(fetchSnapshot, 5000)` with visible "Last updated" timestamp.

---

## 8. Static Asset Embedding (Go)

```go
//go:embed dashboard
var dashboardFS embed.FS

sub, _ := fs.Sub(dashboardFS, "dashboard")
mux.Handle("GET /dashboard/", http.StripPrefix("/dashboard/", http.FileServer(http.FS(sub))))
mux.Handle("GET /dashboard", http.RedirectHandler("/dashboard/", http.StatusMovedPermanently))
```

**Embed path:** Dashboard files in `cmd/api-gateway/dashboard/`; `//go:embed dashboard` embeds that directory (paths: `dashboard/index.html`, `dashboard/static/...`). Use `fs.Sub(dashboardFS, "dashboard")` so FileServer serves `index.html` at `/dashboard/`.

File layout under `web/dashboard/`:
```
cmd/api-gateway/dashboard/
  index.html          -> served at /dashboard/ or /dashboard/index.html
  static/
    style.css
    app.js
```

---

## 9. Deploy & Validate Wiring

### 9.1 Deploy

- **No new binary.** Dashboard is part of api-gateway.
- `deploy.sh` already builds and starts api-gateway; no changes.
- Ensure `web/dashboard/` is present so `embed` succeeds at build time.

### 9.2 Validate

Add to `scripts/validate.sh` (after Phase 7 block, before SUMMARY):

```bash
# ═══════════════════════════════════════════════
# DASHBOARD — Platform visibility
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ DASHBOARD ═══')"

echo "Dashboard snapshot endpoint:"
SNAPSHOT=$(curl -sf "$API/api/dashboard/snapshot" 2>/dev/null || echo "{}")
assert_not_empty "GET /api/dashboard/snapshot returns data" "$SNAPSHOT"

SNAPSHOT_KEYS=$(echo "$SNAPSHOT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(','.join(sorted(d.keys())))" 2>/dev/null || echo "")
assert_contains "snapshot has services key" "services" "$SNAPSHOT_KEYS"
assert_contains "snapshot has workers key" "workers" "$SNAPSHOT_KEYS"
assert_contains "snapshot has approvals key" "approvals" "$SNAPSHOT_KEYS"
assert_contains "snapshot has cost key" "cost" "$SNAPSHOT_KEYS"

echo "Dashboard UI:"
DASH_HTML=$(curl -sf "$API/dashboard/" 2>/dev/null || curl -sf "$API/dashboard/index.html" 2>/dev/null || echo "")
assert_contains "dashboard HTML served" "dashboard" "$DASH_HTML"
```

---

## 10. Implementation Order

| Step | Task | Owner |
|------|------|-------|
| 1 | Add `LogsDir`, `WorkerManagerAddr`, `CostTrackerAddr` to `pkg/config` | Tech Lead → Go Engineer |
| 2 | Create `internal/dashboard/snapshot.go` (aggregator) | Tech Lead → Go Engineer |
| 3 | Create `web/dashboard/index.html`, `static/style.css`, `static/app.js` | Tech Lead → Go Engineer |
| 4 | Extend `cmd/api-gateway/main.go`: embed, routes, snapshot handler | Tech Lead → Go Engineer |
| 5 | Add validation block to `scripts/validate.sh` | Tech Lead → QA Engineer |
| 6 | Update `docs/PRD.md` roadmap (Phase 8 or post-7 item) | Tech Lead |
| 7 | Run `./scripts/validate.sh` to verify | DevOps / QA |

---

## 11. Security Notes

- **S2 (JWT):** Snapshot endpoint can remain unauthenticated for local/dev; for production, protect with JWT and `dashboard` or `admin` scope.
- **S5 (Secrets):** Log tails may contain sensitive data. Restrict dashboard to ops network or require strong auth. Consider truncating or redacting log content.
- **Log path:** `LogsDir` must not be user-controlled to avoid path traversal; validate or use fixed relative path.

---

## 12. Optional Future Enhancements

- Deploy button: POST to trigger `scripts/deploy.sh` (or delegate to DevOps agent per delegation rules)
- Validate button: GET `/api/dashboard/validate` that runs `validate.sh` and streams output (or returns last run result)
- Approve/Deny actions: POST to access-control from dashboard (already have approval IDs)

---

*Blueprint complete. Ready for Tech Lead to task Go Engineer and QA Engineer.*
