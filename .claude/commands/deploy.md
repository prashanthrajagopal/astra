# Deploy

Run deployment for Astra.

## Local Deployment

From repo root, run `./scripts/deploy.sh`.

Read `.cursor/skills/devops-deployment/SKILL.md` for the full runbook.

**Behavior:**
1. Postgres/Redis/Memcached — native-first, Docker for missing services
2. Migrations — all `migrations/*.sql` in order
3. Build — `go build` all platform binaries into `bin/`
4. Start — background processes, logs in `logs/*.log`, PIDs in `logs/*.pid`
5. Seed — super-admin user, then `scripts/seed-agents.sh`

**Stop:** `for f in logs/*.pid; do kill $(cat $f) 2>/dev/null; done`

## GCP Deployment

Run `./scripts/gcp-deploy.sh` with appropriate flags:

| Flag | Purpose |
|------|---------|
| `--setup` | First-time: Artifact Registry, GKE, Cloud SQL, Memorystore, GCS bucket |
| `--dev` / `--prod` | Values tier |
| `--build-only` | Build + push images only |
| `--deploy-only` | Skip image build; migrate + Helm |

Typical first run: `./scripts/gcp-deploy.sh --setup --dev`
Iterative: `./scripts/gcp-deploy.sh --dev`

**GCP Stack:** GKE Autopilot, Cloud SQL PostgreSQL, Memorystore Redis + Memcached, Artifact Registry, GCS (no MinIO on GCP).
