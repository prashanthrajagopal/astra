# Phase 2 — Workers & Tool Runtime

**Status:** Complete  
**Date:** 2026-03-09

## What was built

### WP2.1 — Worker registration and heartbeat (`internal/workers/`)
- **`registry.go`** — DB-backed `Registry` with `Register`, `UpdateHeartbeat`, `MarkOffline`, `ListActive`, `FindStaleWorkers`. Capabilities stored as JSONB. Uses `ON CONFLICT DO UPDATE` for idempotent registration.
- **`worker.go`** — Extended with `NewWithDB` constructor. `StartHeartbeat` now calls `Registry.Register` on first tick and `Registry.UpdateHeartbeat` on subsequent ticks when DB is present.
- **`registry_test.go`** — Tests for register, list active, stale detection, mark offline.

### WP2.2 — Execution worker (`cmd/execution-worker/`)
- Full task execution loop: consume from `astra:tasks:shard:0` → transition queued→scheduled→running → set `worker_id` on task → read task payload → build `ToolRequest` → call `Runtime.Execute` → `CompleteTask` or `FailTask`.
- Configurable tool runtime via `TOOL_RUNTIME` env (default: noop, option: docker).
- Worker registers in DB on startup; marks offline on shutdown.
- Dual heartbeat: bus publish (Redis stream) and registry update (DB).

### WP2.3 — Tool runtime and sandbox (`internal/tools/`, `cmd/tool-runtime/`)
- **`DockerRuntime`** — Real `docker run` via `os/exec`:
  - `--memory`, `--cpus` resource limits from ToolRequest.
  - `--network none` for isolation, `--read-only` for security.
  - Input via stdin, output from stdout/stderr.
  - Context-based timeout; returns exit code, duration, output.
  - Configurable image (default: `alpine:3.20`).
- **`NoopRuntime`** — Phase 2 placeholder returning JSON for general tasks, HTML for browser tasks.
- **`cmd/tool-runtime`** — HTTP service on port 8083 with `POST /execute` (base64 I/O) and `GET /health`.
- **`runtime_test.go`** — Unit tests for NoopRuntime and DockerRuntime (short-mode skippable).

### WP2.4 — Worker manager (`cmd/worker-manager/`)
- HTTP service on port 8082.
- Background loop every 15s: `FindStaleWorkers(30s)` → `MarkOffline`.
- Re-queue logic: `FindOrphanedRunningTasks` → `RequeueTask` → republish to stream.
- `GET /workers` returns JSON array of active workers; `GET /health` returns "ok".

### WP2.5 — Re-queue on worker failure
- Added `worker_id` column to `tasks` table via `migrations/0010_worker_task_tracking.sql`.
- `tasks.Store.SetWorkerID` — sets worker_id when execution-worker claims a task.
- `tasks.Store.FindOrphanedRunningTasks` — finds running tasks whose worker is offline.
- `tasks.Store.RequeueTask` — transitions running→queued, clears worker_id, emits TaskRequeued event.
- Worker-manager runs re-queue scan in same loop as stale worker detection.

### WP2.6 — Browser worker (`cmd/browser-worker/`)
- Consumes from `astra:tasks:browser` stream.
- Registers with `["browser"]` capabilities.
- Uses NoopRuntime (returns placeholder HTML for browser tasks; real Playwright deferred).
- Same registration, heartbeat, consume, complete/fail pattern as execution-worker.

## Key decisions
- **Docker-first sandbox:** WASM/Firecracker deferred to later phases. Docker gives immediate isolation with resource limits.
- **NoopRuntime default:** Phase 2 ships with noop as the default runtime to avoid Docker daemon dependency for development. Set `TOOL_RUNTIME=docker` to use real Docker sandbox.
- **worker_id on tasks:** Enables precise orphan detection. `ON DELETE SET NULL` ensures task data survives worker cleanup.
- **Re-queue in worker-manager:** Chosen over scheduler to keep the scheduler focused on dispatch. Worker-manager owns worker lifecycle and recovery.

## Files changed
- `internal/workers/worker.go` — Extended with NewWithDB, registry integration
- `internal/workers/registry.go` — New: DB-backed worker registry
- `internal/workers/registry_test.go` — New: Registry tests
- `internal/tools/runtime.go` — Real DockerRuntime, NoopRuntime
- `internal/tools/runtime_test.go` — New: Runtime tests
- `internal/tasks/store.go` — Added SetWorkerID, FindOrphanedRunningTasks, RequeueTask
- `cmd/execution-worker/main.go` — Full execution loop with tool runtime
- `cmd/worker-manager/main.go` — Health tracking, stale detection, re-queue
- `cmd/tool-runtime/main.go` — HTTP tool execution service
- `cmd/browser-worker/main.go` — Browser task worker
- `migrations/0010_worker_task_tracking.sql` — worker_id column on tasks
- `scripts/deploy.sh` — Updated to build/start Phase 2 services
- `scripts/validate.sh` — Phase 2 validation tests (34 pass, 0 fail)
- `docs/PRD.md` — Phase 2 marked complete
