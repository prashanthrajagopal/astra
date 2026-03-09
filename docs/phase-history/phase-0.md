# Phase 0 — Prep (completed)

**Phase:** 0  
**Status:** completed  
**Scope:** Repo layout, proto codegen, CI, deploy script, phase/usage/audit design and schema.

---

## Goals

- Establish monorepo layout (cmd/, internal/, pkg/, proto/, migrations/, scripts/, docs/).
- Proto definitions with generated Go stubs; full-tree `go build ./...` and CI (vet, lint, test, build).
- Single local deploy script (native-first infra, migrations, run core services).
- Design and schema for phase/build history, token/LLM usage, and audit logs.

---

## What was done

### Repo and build

- **Proto:** Moved to `proto/kernel/kernel.proto` and `proto/tasks/task.proto`; added `buf.yaml`, `buf.gen.yaml` (remote plugins). Generated Go: `proto/kernel/*.pb.go`, `proto/tasks/*.pb.go`. Script: `scripts/proto-generate.sh`; doc: `docs/codegen.md`.
- **CI:** `.github/workflows/ci.yml` — vet, golangci-lint, test, build on push/PR to main. `.golangci.yml` with errcheck, govet, ineffassign, gofmt.
- **Deploy:** `scripts/deploy.sh` — native-first Postgres/Redis/Memcached; Docker fallback; idempotent migrations; build and start api-gateway + scheduler-service. Docs: `docs/local-deployment.md`, `docs/mac-mini-deployment.md`.

### Phase history, usage, and audit (design + schema)

- **Design:** `docs/phase-history-usage-audit-design.md` — phase/build history (file + DB + pgvector), per-request token/LLM usage (response metadata + async persistence), audit logs in `events`; 10 ms hot-path constraint; implementation order.
- **Migration:** `migrations/0009_phase_history_and_usage.sql` — `phase_runs`, `phase_summaries` (vector 1536), `llm_usage`, indexes; `idx_events_created_at` for audit. Idempotent.

### Development history

- **This file:** `docs/phase-history/phase-0.md` — what was built in Phase 0.
- **Convention:** `docs/phase-history/README.md` explains development-phase history vs runtime `phase_runs` and vector DB.

---

## Decisions

- **Buf remote plugins** — No local protoc/protoc-gen-go required; CI uses committed generated code.
- **Go 1.22 in CI/Dockerfile** — go.mod may say 1.25; 1.22 used for image/runner availability until 1.25 is standard.
- **Audit in `events`** — Single append-only store for phase and LLM usage events; no separate audit DB for MVP.
- **Async usage and phase file writes** — Usage and phase log files written via stream/consumer so API stays under 10 ms.

---

## References

- PRD: `docs/PRD.md` (Phase 0 in §25).
- Phase 0 checklist: `docs/phase0-completion-checklist.md`.
- Audit/usage design: `docs/phase-history-usage-audit-design.md`.
