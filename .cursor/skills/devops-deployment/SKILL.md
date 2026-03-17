# DevOps Deployment Skill

Use when performing **local** or **GCP** deployment for Astra. **Only the DevOps agent** runs these scripts.

---

## Local deployment

| Item | Detail |
|------|--------|
| **Script** | `scripts/deploy.sh` (repo root) |
| **Example** | `./scripts/deploy.sh` |
| **Env** | `.env` (from `.env.example` if missing) |

### Behavior (order)

1. **Postgres / Redis / Memcached** — Native-first (detect via `pg_isready`, `redis-cli ping`, TCP). Start **only missing** services with `docker compose` (`postgres`, `redis`, `memcached`). No MinIO required for core local run unless features need it (`docker compose` includes optional MinIO).
2. **Migrations** — All `migrations/*.sql` in order (`ON_ERROR_STOP`).
3. **Build** — `go build` all platform binaries into `bin/` (api-gateway, identity, access-control, task-service, agent-service, scheduler-service, execution-worker, worker-manager, tool-runtime, browser-worker, memory-service, llm-router, prompt-manager, planner-service, goal-service, evaluation-service, cost-tracker).
4. **Start** — Background processes, logs in `logs/*.log`, PIDs in `logs/*.pid`.
5. **Seed** — Super-admin user (idempotent), then `scripts/seed-agents.sh` when gateway is up.

### Stop

`for f in logs/*.pid; do kill $(cat $f) 2>/dev/null; done`

**Full detail:** [`docs/deployment-design.md`](docs/deployment-design.md), [`README.md`](README.md) Quick Start.

---

## GCP deployment (GKE + managed data)

| Item | Detail |
|------|--------|
| **Script** | `scripts/gcp-deploy.sh` (repo root) |
| **Config** | Optional `.env.gcp` in repo root; template [`scripts/.env.gcp.example`](scripts/.env.gcp.example) |
| **Docs** | [`README.md`](README.md) section “GCP (GKE Autopilot)”, [`deployments/helm/astra/README.md`](deployments/helm/astra/README.md) |

### Flags

| Flag | Purpose |
|------|---------|
| `--setup` | First-time: Artifact Registry, GKE Autopilot, Cloud SQL (Postgres 15), Memorystore Redis, Memorystore Memcached, **GCS bucket** for workspace objects |
| `--dev` / `--prod` | Values tier: `values-gke-dev.yaml` vs `values-gke-prod.yaml` (SQL/Redis sizing) |
| `--build-only` | Build + push images; skip Helm/migrations |
| `--deploy-only` | Skip image build; migrate + Helm |

Typical first run: `./scripts/gcp-deploy.sh --setup --dev`  
Iterative: `./scripts/gcp-deploy.sh --dev`

### GCP stack (native managed)

- **Compute:** GKE Autopilot  
- **DB:** Cloud SQL PostgreSQL  
- **Cache / streams:** Memorystore Redis  
- **LLM cache:** Memorystore Memcached  
- **Images:** Artifact Registry  
- **Object storage:** **Google Cloud Storage** — bucket `gs://${GCP_PROJECT}-astra-workspace` (override with `GCS_WORKSPACE_BUCKET` in `.env.gcp`). **Do not deploy MinIO on GCP** for this path; MinIO remains local/docker-compose only.

### Helm

Script runs **per-service** `helm upgrade --install astra-<service> deployments/helm/astra` with `--set service.name=...` and image from Artifact Registry. Not a single umbrella release name `astra`.

### Secrets

Production: prefer **Secret Manager** + Workload Identity; do not commit DB passwords. Values files in repo may contain placeholders for dev tiers only.

---

## Summary

| Environment | Entry |
|-------------|--------|
| Local | `./scripts/deploy.sh` |
| GCP | `./scripts/gcp-deploy.sh` |

Other agents **delegate** deployment to DevOps (or user runs scripts themselves).
