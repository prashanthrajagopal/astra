# Phase 4 — Orchestration, Eval, Security

**Status:** Complete  
**Date:** 2026-03-09

## What was built

### WP4.1 — Planner goal->DAG (`internal/planner/`)
- Replaced hardcoded planner path with LLM-aware planning flow.
- Planner attempts JSON DAG generation from LLM response and falls back to deterministic graph when needed.
- New planner signature includes context and agent identity.

### WP4.2 — planner-service (`cmd/planner-service`)
- Added HTTP planner service with `/plan` and `/health`.
- Service runs on `PLANNER_PORT` (default 8087).

### WP4.3 — goal-service (`cmd/goal-service`)
- Added goal lifecycle endpoints (`POST /goals`, `GET /goals`, `GET /goals/{id}`, `POST /goals/{id}/finalize`).
- Persists `goals`, creates/updates `phase_runs`, and appends phase lifecycle events.
- Uses planner + task graph persistence to create executable plans.
- Service runs on `GOAL_SERVICE_PORT` (default 8088).

### WP4.4 — evaluation-service (`cmd/evaluation-service`, `internal/evaluation`)
- Added evaluator endpoint with default pass/fail behavior and criteria checks (regex/substring).
- Service runs on `EVALUATION_PORT` (default 8089).

### WP4.5 — identity (`cmd/identity`)
- Added JWT token issue (`POST /tokens`) and validation (`POST /validate`) endpoints.
- Uses HS256 with env-configured secret `ASTRA_JWT_SECRET`.
- Service runs on `IDENTITY_PORT` (default 8085).

### WP4.6 — access-control (`cmd/access-control`)
- Added policy check endpoint (`POST /check`) and approval workflow endpoints.
- Added pending approval listing and approve/deny endpoints.
- Service runs on `ACCESS_CONTROL_PORT` (default 8086).

### WP4.7 — api-gateway auth enforcement (`cmd/api-gateway`)
- Added authentication/authorization middleware on protected routes.
- Gateway validates JWT via identity service and enforces policy checks via access-control.
- Health endpoint remains open for readiness.

### WP4.8 — Tool approval gates (`cmd/tool-runtime` + migration 0012)
- Added approval gate check before tool execution.
- Dangerous tool actions create `approval_requests` rows and return `pending_approval`.
- Added approval request metadata support (`task_id`, `worker_id`).

### WP4.9 — LLM usage async persistence (`cmd/llm-router`)
- Added async usage publish to Redis stream `astra:usage`.
- Added consumer that writes to `llm_usage` and appends `LLMUsage` events.
- No synchronous DB write on the completion request path.

## Operational updates

- `scripts/deploy.sh` updated to build/start Phase 4 services (identity, access-control, planner-service, goal-service, evaluation-service).
- `scripts/validate.sh` updated with real Phase 4 checks for auth, policy, approval gates, evaluation, and async usage plumbing.
- `docs/PRD.md` updated to mark Phase 3 and Phase 4 complete.

## Verification

- `go build ./...` passes.
- `go test ./... -short` passes.
- Phase 4 validation section now executes concrete checks instead of placeholders.
