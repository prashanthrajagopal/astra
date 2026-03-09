# DevOps Deployment Skill

Use this skill when performing **local** or **cloud** deployment for Astra. Only the DevOps agent runs deployments.

---

## Local deployment

### Entry point

- **Single script:** `scripts/deploy.sh`
- **Run from:** Repo root. Example: `./scripts/deploy.sh`
- **Who runs it:** Only the DevOps agent (or the user, if they run it themselves). No other agent may run it.

### What the script does (in order)

1. **Load env** — Sources `.env` if present; uses `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER`, `PGPASSWORD` (or `POSTGRES_PASSWORD`), `REDIS_ADDR`, `MEMCACHED_ADDR`.
2. **Detect infra (native-first)**  
   - **Postgres:** `pg_isready` or TCP to `POSTGRES_HOST:POSTGRES_PORT`. If available → use native. If not → `docker compose up -d postgres`, wait until ready.  
   - **Redis:** `redis-cli ping` or TCP to Redis host:port. If available → native. If not → `docker compose up -d redis`, wait.  
   - **Memcached:** TCP to host:11211. If available → native. If not → `docker compose up -d memcached`, wait.
3. **Migrations** — If Postgres is Docker: run each `migrations/*.sql` via `docker compose exec -T postgres psql ...`. If Postgres is native: run with host `psql` (script requires `psql` in PATH when using native Postgres).
4. **Build** — `go mod tidy`, then `go build -o bin/api-gateway ./cmd/api-gateway`, `go build -o bin/scheduler-service ./cmd/scheduler-service`. Requires Go in PATH.
5. **Start** — Run `bin/api-gateway` and `bin/scheduler-service` in background with `.env` loaded; stdout/stderr to `logs/api-gateway.log`, `logs/scheduler-service.log`; PIDs to `logs/*.pid`.
6. **Summary** — Print for each of Postgres, Redis, Memcached whether **native** or **Docker**; log paths; PIDs; how to stop (e.g. `kill $(cat logs/api-gateway.pid) $(cat logs/scheduler-service.pid)`).

### When to use native vs Docker

- Use **native** when the service is already reachable at the configured host:port (e.g. user has Postgres/Redis/Memcached installed and running).
- Use **Docker** only for services that are **not** available. Do not start all infra in Docker if some are already running.

### Prerequisites

- **Go** in PATH (for build).
- **psql** in PATH if Postgres is native (for migrations).
- **Docker** (and daemon running) only if any service is missing and will be started via Docker.

---

## Cloud deployment

### Entry point

- **Helm chart:** `deployments/helm/astra`
- **Who runs it:** Only the DevOps agent.

### Commands (runbook)

- **Install or upgrade:**  
  `helm upgrade --install astra ./deployments/helm/astra -f deployments/helm/astra/values.yaml -n <namespace> --create-namespace`
- **Namespace:** Use PRD namespaces (e.g. `control-plane`, `kernel`, `workers`, `infrastructure`, `observability`) or a single namespace for a minimal cloud deploy.
- **Values:** Use env-specific values files (e.g. `values-staging.yaml`, `values-production.yaml`) when they exist; override image tag, replicas, and secrets as needed.
- **Secrets:** Never put connection strings or secrets in values; use Vault, external secrets, or k8s Secrets and reference them in the chart.

### Rollback

- `helm rollback astra <revision> -n <namespace>`
- Identify revision with `helm history astra -n <namespace>`.

### Staging vs production

- **Staging:** Deploy with staging values; run integration/smoke tests after deploy.
- **Production:** Use production values and namespace; canary or phased rollout per PRD; monitor before full rollout.

---

## Summary

| Environment | Action | Entry |
|-------------|--------|--------|
| Local (dev) | Run single script | `./scripts/deploy.sh` (from repo root) |
| Cloud (K8s) | Helm upgrade/install | `helm upgrade --install astra ./deployments/helm/astra ...` |

Only the **DevOps agent** performs these actions. Other agents delegate deployment requests to DevOps (or the user runs the script/Helm themselves).
