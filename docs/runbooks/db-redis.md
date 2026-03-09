# Runbook: Database and Redis Issues

## Detect
- Service startup failures to Postgres or Redis
- Elevated latency and timeout errors

## Triage
- Verify reachability to Postgres/Redis ports
- Validate credentials/env in `.env`
- Check migration drift and DB lock contention

## Contain
- Stop non-critical background jobs
- Keep API health endpoints operational if possible

## Remediate
- Restart infra services (native or Docker fallback)
- Re-run migrations via `scripts/deploy.sh`
- Re-deploy services and validate end-to-end
