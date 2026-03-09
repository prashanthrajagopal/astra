# Phase 3 & Phase 4 — Execution Memo

**Audience:** Tech Lead (for work assignment to Go Engineer, DevOps Engineer, QA Engineer, DB Architect).  
**Purpose:** Single handoff for implementing Phase 3 (Memory & LLM Routing) and Phase 4 (Orchestration, Eval, Security).  
**Source plans:** `docs/implementation-plans/phase-3.md`, `docs/implementation-plans/phase-4.md`.

---

## 1. Database migrations — Acknowledgment

**Migrations 0011 (prompts) and 0012 (approval_requests) are present; 0009 covers llm_usage and phase_runs. No additional DB migrations required for Phase 3/4 implementation.**

- **0009_phase_history_and_usage.sql:** Provides `llm_usage` (request_id, agent_id, task_id, model, tokens_in, tokens_out, latency_ms, cost_dollars, created_at), `phase_runs`, `phase_summaries`, and `idx_events_created_at`. Sufficient for WP4.9 (LLM usage persistence) and goal-service phase lifecycle.
- **0011_prompts.sql:** Provides `prompts` table (id, name, version, body, variables_schema, created_at, updated_at) and `idx_prompts_name`. Sufficient for WP3.6 (prompt-manager).
- **0012_approval_requests.sql:** Provides `approval_requests` table for tool execution approval gates (WP4.8). Sufficient for S6 approval workflow.

See `docs/db-phase3-phase4-notes.md` for verification. Ensure 0011 and 0012 are run before prompt-manager and approval-gate code.

---

## 2. Phase summaries

**Phase 3 — Memory & LLM Routing**  
Deliver memory-service with pgvector (write, search by embedding), embedding pipeline with Memcached cache, LLM router with model selection and response caching, and prompt-manager using the prompts table. Bring hot-path API reads into compliance with 10ms SLA: memory search and LLM completions served from Redis/Memcached; cache-aside for task and agent state (task-service, agent-service). No Postgres on read path for these endpoints.

**Phase 4 — Orchestration and Security**  
Replace planner stub with LLM-driven goal→DAG planning (planner-service), add goal-service (phase_runs lifecycle, PhaseStarted/PhaseCompleted) and evaluation-service. Implement identity (JWT issue/validate), access-control (OPA), and api-gateway JWT + OPA middleware. Add tool execution approval gates using the approval_requests table (S6). Persist LLM usage asynchronously (astra:usage stream → llm_usage table and events); hot path unchanged.

---

## 3. Assumptions

- **Phase 2 complete:** Execution-worker and tool-runtime operational; worker-manager tracks health.
- **Infra:** Memcached and Redis available (e.g. docker-compose); migrations 0003 (memories), 0007 (indexes), 0009, 0011, 0012 applied.
- **Migrations 0011 and 0012** are run before prompt-manager (WP3.6) and approval-gate (WP4.8) implementation.
- **llm_usage and phase_runs** from migration 0009 are used for WP4.9 and goal-service; no further schema changes for Phase 3/4.

---

## 4. Work packages in execution order

Order respects: (1) Phase 3 foundation (memory store, embedding, LLM router) before services that consume them; (2) prompt-manager after prompts table (0011 done); (3) Phase 4 planner/goal/eval and security in dependency order; (4) WP4.9 after llm-router stable, using llm_usage from 0009.

| ID | One-line description | Primary owner | Key deliverable paths |
|----|----------------------|---------------|------------------------|
| **WP3.1** | Memory store and pgvector: write, search by embedding, GetByID (internal/memory). | Go Engineer | `internal/memory/memory.go`, tests |
| **WP3.2** | Embedding pipeline: Embedder interface, backend, Memcached cache (embed:{hash}), wire into memory Write. | Go Engineer | `internal/memory/embedding.go` or `internal/llm`, Memcached client, tests |
| **WP3.3** | LLM router: Router interface, model selection, Memcached response cache (llm:resp:{model}:{hash}), usage in-memory only. | Go Engineer | `internal/llm/router.go`, `pkg/metrics`, tests |
| **WP3.4** | Memory service API: gRPC WriteMemory, SearchMemories, GetMemory; cache-aside for 10ms read path. | Go Engineer | `proto/` (memory), `cmd/memory-service/main.go`, tests |
| **WP3.5** | LLM router service: gRPC Complete; cmd/llm-router calling internal/llm. | Go Engineer | `proto/` (llm), `cmd/llm-router/main.go`, tests |
| **WP3.6** | Prompt manager: GetPrompt/SavePrompt, CRUD using prompts table; cache get path for 10ms. | Go Engineer | `internal/prompt` or `internal/llm`, `cmd/prompt-manager/main.go`, tests (migration 0011 already in place) |
| **WP3.7** | Redis cache-aside for task and agent state: GetTask/GetGraph, GetAgent from Redis; write path update/invalidate. | Go Engineer | `cmd/task-service`, `cmd/agent-service`, internal/tasks; QA load test (p99 ≤10ms) |
| **WP4.1** | Planner: goal→DAG with LLM (internal/planner), prompt-manager for template, acyclic validation. | Go Engineer | `internal/planner/planner.go`, prompt template, tests |
| **WP4.2** | Planner service API: gRPC Plan(PlanRequest)→PlanResponse; caller creates tasks via task-service. | Go Engineer | `proto/planner`, `cmd/planner-service/main.go`, tests |
| **WP4.3** | Goal service: CreateGoal, GetGoal, ListGoals; phase_runs lifecycle, PhaseStarted/Completed, task creation via planner + task-service. | Go Engineer | `cmd/goal-service`, proto, phase_runs + events; tests |
| **WP4.4** | Evaluation service: Evaluator interface, at least one evaluator (regex/LLM); gRPC Evaluate. | Go Engineer | `internal/evaluation/eval.go`, `cmd/evaluation-service/main.go`, tests |
| **WP4.5** | Identity service: IssueToken, ValidateToken (JWT); secret from config/Vault. | Go Engineer | `proto/identity`, `cmd/identity/main.go`, tests |
| **WP4.6** | Access-control: OPA integration, Check RPC (subject, action, resource → allow/deny); policy docs. | Go Engineer | `proto/access-control`, `cmd/access-control/main.go`, docs/policies |
| **WP4.7** | API gateway: JWT validation + OPA Check middleware; 401/403 for invalid or denied. | Go Engineer | `cmd/api-gateway`, middleware; tests |
| **WP4.8** | Tool execution approval gates: dangerous-action policy, persist to approval_requests, approve API. | Go Engineer | `internal/tools` or `cmd/tool-runtime`, approval API (access-control or api-gateway); tests (migration 0012 already in place) |
| **WP4.9** | LLM usage persistence: publish to astra:usage stream; consumer writes llm_usage + events (async). | Go Engineer | `internal/llm` or `cmd/llm-router`, consumer; tests (migration 0009 already has llm_usage) |

**Parallelization hints:** WP3.1 and WP3.2 can start in parallel; WP3.4 after WP3.1+WP3.2, WP3.5 after WP3.3. WP3.6 and WP3.7 can run in parallel after WP3.4/WP3.5. Within Phase 4: WP4.1, WP4.4, WP4.5, WP4.6 can start in parallel; WP4.2 after WP4.1; WP4.3 after WP4.2; WP4.7 after WP4.5+WP4.6; WP4.8 after WP4.6; WP4.9 when llm-router is stable.

---

## 5. Tech Lead: completion and PRD currency

When **Phase 3** is complete:

1. **`scripts/validate.sh`** — Replace the Phase 3 placeholder `skip_test` calls with real checks (e.g. memory-service and llm-router reachable, prompt-manager GetPrompt, cache-aside / 10ms read path verification or load test). Preserve existing Phase 0–2 and Phase 4+ sections.
2. **PRD §25** — Update the Phase 3 roadmap: check off memory-service, llm-router, prompt-manager, cache-aside, 10ms read path. Mark Phase 3 as complete (or in progress) per project state.
3. **Phase history** — Add or update `docs/phase-history/phase-3.md` with what was built (WPs delivered, key paths, sign-off checklist from phase-3.md).

When **Phase 4** is complete:

1. **`scripts/validate.sh`** — Replace the Phase 4 placeholder `skip_test` calls with real checks (e.g. goal-service E2E, identity JWT + access-control OPA, approval gate flow, llm_usage/events persistence). Preserve existing Phase 0–3 and Phase 5+ sections.
2. **PRD §25** — Update the Phase 4 roadmap: check off planner-service, goal-service, evaluation-service, identity, access-control, api-gateway auth, approval gates, LLM usage audit. Mark Phase 4 as complete (or in progress).
3. **Phase history** — Add or update `docs/phase-history/phase-4.md` with what was built (WPs delivered, key paths, sign-off checklist from phase-4.md).

Per PRD currency rule: do not mark a phase done in the PRD until the corresponding validate.sh and phase-history updates are done.

---

## 6. Sign-off reference

- **Phase 3:** Use the “Sign-off (Phase 3 complete)” checklist in `docs/implementation-plans/phase-3.md`.
- **Phase 4:** Use the “Sign-off (Phase 4 complete)” checklist in `docs/implementation-plans/phase-4.md`.

Security compliance (S2 JWT, S3 OPA, S6 approval gates) must be verified for Phase 4; use the checklist in `.cursor/rules/SECURITY-RULE.mdc`.
