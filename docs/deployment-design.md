# Astra Deployment Design

Single-script local deployment with native-first infra and DevOps-only execution. Implementers: use this doc to build the script, update the DevOps agent, add the rule/skill/command, and update docs.

---

## 1. One Script Only

- **Single entry point:** `scripts/deploy.sh` (no `dev.sh`, `build-native.sh`, `run-dev.sh`, `migrate.sh`).
- **Scope:** Local development deployment only. **GCP:** use [`scripts/gcp-deploy.sh`](../scripts/gcp-deploy.sh) (GKE Autopilot, Cloud SQL, Memorystore). On GCP, workspace/object storage is **Google Cloud Storage** (bucket on `--setup`), not MinIO.
- **Remove or consolidate:** Delete or replace `scripts/dev.sh` with `scripts/deploy.sh`. Migration execution lives inside `deploy.sh`; no standalone `migrate.sh`.

---

## 2. Native-First, Docker Fallback

- **Principle:** Use existing local services when available; start Docker only for services that are **not** already running.
- **Services checked (in order):** Postgres, Redis, Memcached, MinIO (if required for local dev).
- **Per service:** If available on the expected host:port → use it. Else → start that service via Docker (compose or override).
- **Docker usage:** Start only the missing services (e.g. `docker compose up -d postgres` or use a compose file/override that defines only postgres, redis, memcached, minio so they can be brought up individually).

---

## 3. Service Detection (Exact Steps)

Script must detect availability **before** starting any Docker containers.

### Postgres

- **Method 1:** If `pg_isready` is in PATH: `pg_isready -h "${POSTGRES_HOST:-localhost}" -p "${POSTGRES_PORT:-5432}" -U "${POSTGRES_USER:-astra}"` (or equivalent; success exit = available).
- **Method 2 (fallback):** TCP connect to `POSTGRES_HOST:POSTGRES_PORT` (e.g. `bash -c 'echo >/dev/tcp/$HOST/$PORT'` or equivalent); if connect succeeds, treat as available.
- **Result:** If available → use native Postgres; set a flag so migrations run on host via `psql`. If not → start Postgres via Docker and run migrations via `docker exec`.

### Redis

- **Method 1:** If `redis-cli` in PATH: `redis-cli -h "${REDIS_HOST:-localhost}" -p "${REDIS_PORT:-6379}" ping` (or equivalent); response PONG = available.
- **Method 2 (fallback):** TCP connect to `REDIS_HOST:REDIS_PORT` (e.g. 6379).
- **Result:** If available → use native Redis. If not → start Redis via Docker.

### Memcached

- **Method:** TCP connect to `MEMCACHED_HOST:MEMCACHED_PORT` (default localhost:11211). If connection succeeds, treat as available. Optionally: send a minimal protocol check (e.g. `version\r\n`) and read response.
- **Result:** If available → use native. If not → start Memcached via Docker.

### MinIO (if needed for local dev)

- **Method:** HTTP/HTTPS request to the MinIO endpoint (e.g. health or bucket list) or TCP connect to the configured port (e.g. 9000). If reachable, use native.
- **Result:** If available → use native. If not → start MinIO via Docker.

---

## 4. Docker: Start Only Missing Services

- **Compose:** Use `docker-compose.yml` (or override) so that services can be started individually, e.g. `docker compose up -d postgres`, `docker compose up -d redis`, etc.
- **Order:** After detection, for each of Postgres, Redis, Memcached, MinIO: if not available, run `docker compose up -d <service>` (or equivalent) for that service only.
- **Wait:** After starting a service via Docker, wait until it is ready (e.g. for Postgres: `pg_isready` inside container or from host; for Redis: `redis-cli ping`; for Memcached: TCP connect; for MinIO: HTTP/port check).
- **No full stack required:** Do not run `docker compose up -d` for all services if some are already native; only start the missing ones.

---

## 5. Migrations

- **When Postgres is Docker:** Run migrations inside the Postgres container: `docker compose exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -f -` with each migration file piped in, or `docker compose exec -T postgres psql ... -f /path` if files are mounted. Apply migrations in lexical order (e.g. `migrations/*.sql`).
- **When Postgres is native:** Run migrations on the host using `psql`. Require `psql` in PATH when native Postgres is used; if not found, exit with a clear message (e.g. "Native Postgres detected but psql not found. Install PostgreSQL client or use Docker Postgres.").
- **Idempotency:** Migrations must remain idempotent (IF NOT EXISTS, IF EXISTS) per project standards.

---

## 6. Build

- **Location:** Build outputs to `bin/` (e.g. `bin/api-gateway`, `bin/scheduler-service`).
- **Commands:** `go build -o bin/<name> ./cmd/<name>` for each required binary.
- **Minimum for local dev:** Build at least `api-gateway` and `scheduler-service`. Optionally build `agent-service`, `execution-worker` (document as optional in script or env).
- **Prerequisites:** Script must require Go in PATH; run from repo root; `go mod tidy` (or equivalent) before build. Create `bin/` and `logs/` if missing.

---

## 7. Start (Local Dev)

- **Process management:** Start built binaries in the background.
- **Environment:** Load `.env` (e.g. `set -a; source .env; set +a` or equivalent) before starting processes.
- **Logs:** Redirect stdout/stderr to `logs/<service>.log` (e.g. `logs/api-gateway.log`, `logs/scheduler-service.log`).
- **PIDs:** Write PID to `logs/<service>.pid` for each started process so users can stop with `kill $(cat logs/*.pid)`.
- **Services to start:** At minimum: `api-gateway`, `scheduler-service`. Optional: `agent-service`, `execution-worker` (configurable or documented).
- **Order:** Start after migrations and build; ensure infra (Postgres, Redis, Memcached) is ready before starting services that depend on them.

---

## 8. Script Output (Summary)

At the end of a successful run, the script must print:

- **Per infra service:** Whether it used **native** or **Docker** (e.g. "Postgres: native", "Redis: Docker").
- **Log locations:** e.g. "Logs: logs/api-gateway.log, logs/scheduler-service.log".
- **PIDs:** Where PIDs are stored (e.g. `logs/*.pid`).
- **How to stop:** e.g. "Stop: kill $(cat logs/api-gateway.pid) $(cat logs/scheduler-service.pid)" or equivalent.

---

## 9. DevOps Agent Owns All Deployments

### 9.1 Rule (New or Add to Existing)

- **Add a rule** (e.g. in `.cursor/rules/` or extend an existing deployment-related rule) that states:
  - **Only the DevOps agent may run deployment scripts or deployment commands** (e.g. `scripts/deploy.sh`, `scripts/gcp-deploy.sh`, `docker compose` for Astra infra, `helm install`/`helm upgrade` for Astra).
  - **Other agents must not** run the deploy script, docker compose for Astra, or helm install/upgrade for Astra. They must **delegate deployment requests to DevOps** (e.g. via Project Manager → Tech Lead → DevOps, or via a `/deploy` command that routes to DevOps).

### 9.2 DevOps Agent Definition

- **Update** `.cursor/agents/devops-engineer.md`:
  - State that **local and cloud deployment** are explicit responsibilities of the DevOps engineer.
  - **Local:** Run the single script `scripts/deploy.sh`; follow native-first behavior (use existing Postgres/Redis/Memcached when available; only start Docker for missing services).
  - **Cloud:** Use Helm/K8s per the DevOps skill (e.g. `helm upgrade --install` with `deployments/helm/astra`, env-specific values).
  - Reference the DevOps deployment skill for step-by-step runbooks.

### 9.3 DevOps Deployment Skill

- **Create a skill** for the DevOps agent (e.g. `.cursor/skills/devops-deployment/SKILL.md` or similar) that covers:
  - **Local deployment:**
    - Single script: `scripts/deploy.sh` (run from repo root).
    - What the script does step-by-step: detect Postgres/Redis/Memcached (and MinIO if needed), start only missing services via Docker, run migrations (in-container vs host psql), build Go binaries to `bin/`, start services in background with logs and PIDs, print summary.
    - When to use native vs Docker: use native when the service is already available at the configured host:port; otherwise start via Docker.
  - **Cloud deployment:**
    - Use Helm chart at `deployments/helm/astra`.
    - Commands: e.g. `helm upgrade --install astra ./deployments/helm/astra -f values-<env>.yaml -n <namespace>` (document exact pattern and required values files).
    - What to run for staging vs production (values, namespace, rollback procedure).
- **No code in the skill** — procedural steps and references only.

### 9.4 Deploy Command

- **Add a custom command** (e.g. `/deploy` or document in the skill that "deploy" routes to DevOps).
  - If using Cursor commands: add `.cursor/commands/deploy.md` (or equivalent) that instructs: "Route to DevOps agent to run deployment. Local: run scripts/deploy.sh. Cloud: follow DevOps deployment skill."
  - So that users and other agents can request deployment by invoking the command, which routes to DevOps.

---

## 10. Cloud Deployment (No Script Change)

- The single script **does not** handle cloud. For cloud (K8s, Helm):
  - DevOps follows the **DevOps deployment skill** (runbook).
  - Use `helm upgrade --install` with `deployments/helm/astra` and env-specific values.
  - Document in the skill: target env, namespace, values files, and how to roll back.

---

## 11. Documentation Updates

- **docs/local-deployment.md:**
  - Point to the **single deploy script** `scripts/deploy.sh` as the one entry point for local dev.
  - State that **only the DevOps agent** runs the deploy script; others should request deployment by delegating to DevOps (or using `/deploy`).
  - Describe **native-first behavior:** if Postgres/Redis/Memcached (and MinIO if applicable) are already running locally, the script uses them and does not start Docker for those; Docker is used only for missing services.
  - Remove or replace references to `dev.sh`, and any separate migrate/build/run scripts.
- **docs/mac-mini-deployment.md:**
  - Same as above: single script `scripts/deploy.sh`, DevOps-only execution, native-first. Optionally note that for Mac Mini, running Astra services and Ollama natively while using the script for infra (native or Docker) is the recommended approach.

---

## 12. Implementation Checklist (for Implementer)

- [ ] Create `scripts/deploy.sh` implementing sections 2–8 (detection, Docker fallback, migrations, build, start, output).
- [ ] Remove or replace `scripts/dev.sh` (logic consolidated into `scripts/deploy.sh`); ensure no remaining references to `dev.sh`, `migrate.sh`, `build-native.sh`, `run-dev.sh` in docs or scripts.
- [ ] Add rule: only DevOps may run deploy script / docker compose for Astra / helm for Astra; others delegate to DevOps.
- [ ] Update `.cursor/agents/devops-engineer.md`: local and cloud deployment ownership; reference `scripts/deploy.sh` and native-first; reference DevOps deployment skill.
- [ ] Create DevOps deployment skill: local (script steps, native vs Docker) and cloud (Helm, commands, envs).
- [ ] Add `/deploy` (or equivalent) command that routes to DevOps for deployment.
- [ ] Update `docs/local-deployment.md` and `docs/mac-mini-deployment.md` to point to `scripts/deploy.sh`, DevOps-only, and native-first behavior.

---

*End of design document.*
