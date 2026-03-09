# Phase 5 & Phase 6 — Execution Memo

**Audience:** Tech Lead (for work assignment to Go Engineer, DevOps Engineer, QA Engineer).  
**Purpose:** Single handoff for implementing Phase 5 (Scale & Production Hardening) and Phase 6 (SDK & Applications).  
**Source plans:** `docs/implementation-plans/phase-5.md`, `docs/implementation-plans/phase-6.md`.

---

## 1. Phase summaries

**Phase 5 — Scale & Production Hardening**  
Harden the system for production: load testing at target scale (10k agents, 1M tasks), observability dashboards (Grafana), alerting (Prometheus), runbooks, cost tracking, SLO enforcement (10ms reads, 50ms scheduling), and Helm chart improvements (HPA, PDB, resource limits). Phase 5 is complete when core WPs 5.1–5.7 are delivered and signed off; WP5.8 (sharding) is optional.

**Phase 6 — SDK & Applications**  
Deliver a public Astra SDK (Go) under `pkg/sdk` with AgentContext, MemoryClient, ToolClient, and a minimum viable sample (SimpleAgent) so application developers can build agent apps without touching kernel internals. Include SDK documentation and at least two examples. Phase 6 minimum is WP6.1–WP6.4; WP6.5 (goal helpers) is optional. SDK must NOT import `internal/*`.

---

## 2. Assumptions

- **Phase 4:** Phase 5 depends on Phase 4 (goal-service, planner, identity, access-control, evaluation, usage audit). If Phase 4 is not yet implemented, execution may assume Phase 4 done or stub those services where needed for load tests and SDK calls. Document any stubs.
- **Infra:** Prometheus, Grafana, and (where applicable) OpenTelemetry collector are available in the deploy environment or are added as part of Phase 5 (e.g. via Helm subcharts or docs).
- **Security:** All designs and implementations must comply with S1–S6 (see `.cursor/rules/SECURITY-RULE.mdc`). No exceptions on hot path (mTLS, JWT, OPA, sandbox, secrets, approval gates).

---

## 3. Phase 5 — Mandatory vs optional

| Scope | Work packages | Note |
|-------|----------------|------|
| **Mandatory (Phase 5 complete)** | WP5.1, WP5.2, WP5.3, WP5.4, WP5.5, WP5.6, WP5.7 | Load test, dashboards, alerting, runbooks, cost/SLO, Helm hardening, observability consistency. All must be delivered and signed off for “Phase 5 complete.” |
| **Optional** | WP5.8 (Sharding and multi-scheduler) | Defer if single-shard meets 1M tasks/day; document decision. |

Prioritize mandatory WPs. WP5.8 can be scheduled after WP5.6 or dropped from the phase if not required for scale.

---

## 4. Phase 6 — Minimum scope and constraints

- **Minimum (Phase 6 complete):** WP6.1 (AgentContext), WP6.2 (MemoryClient / ToolClient), WP6.3 (SimpleAgent), WP6.4 (SDK docs and two examples). Delivered under `pkg/sdk` and `examples/` (or `cmd/simple-agent`), with SDK README and run instructions.
- **Optional:** WP6.5 (goal helpers: CreateGoal, WaitForCompletion). Include if time permits; not required for Phase 6 sign-off.
- **Hard constraint:** SDK lives under `pkg/sdk` and must NOT import any `internal/*` packages. Only `pkg/*`, standard library, and generated proto packages are allowed.

---

## 5. Work packages in execution order

Execution order respects: (1) Phase 5 observability first so load tests and dashboards have data, (2) Phase 6 SDK after Phase 4 (and ideally after Phase 5), (3) dependencies within each phase as in the source plans.

| ID | One-line description | Primary owner | Deliverable paths / artifacts |
|----|----------------------|---------------|-------------------------------|
| **WP5.7** | Observability: tracing and metrics consistency across services (trace_id, astra_* metrics). | Go Engineer | `pkg/otel`, services; `docs/observability.md` |
| **WP5.2** | Grafana dashboards: Cluster Overview, Agent Health, Cost. | DevOps Engineer | `deployments/grafana/dashboards/` (or equivalent); README for import |
| **WP5.3** | Prometheus alerting rules (failure rate, queue depth, worker availability, LLM cost spike). | DevOps Engineer | `deployments/prometheus/rules/` or Helm values; docs/annotations |
| **WP5.4** | Runbooks: Worker Lost, High Error Rate, Postgres/Redis, LLM cost spike; index. | DevOps Engineer | `docs/runbooks/*.md`, `docs/runbooks/README.md` |
| **WP5.1** | Load testing: 10k agents, 1M tasks; measure p99 read ≤10ms, scheduling median ≤50ms. | QA Engineer | `tests/load/` or scripts; results report; bottleneck analysis |
| **WP5.5** | Cost tracking (aggregate llm_usage by agent/model/day) and SLO alerts (10ms, 50ms). | Go Engineer + DevOps | Metrics/API; Prometheus SLO alerts; `internal/cost` or `cmd/cost-tracking` if implemented |
| **WP5.6** | Helm hardening: HPA, PDB, resource limits; chart validation in CI. | DevOps Engineer | `deployments/helm/astra/templates/`, `values.yaml`; CI step |
| **WP5.8** | (Optional) Sharding and multi-scheduler by agent_id/graph_id. | Go Engineer | `internal/scheduler`; QA for load test with multiple schedulers |
| **WP6.1** | SDK package structure and AgentContext (ID, Memory, CreateTask, PublishEvent, CallTool). | Go Engineer | `pkg/sdk/` (e.g. `context.go`, `client.go`); unit tests |
| **WP6.2** | MemoryClient and ToolClient interfaces; clients calling memory-service and tool-runtime. | Go Engineer | `pkg/sdk/memory.go`, `pkg/sdk/tool.go` (or equivalent); tests |
| **WP6.3** | SimpleAgent example: Plan, Execute, Reflect; runnable against local Astra. | Go Engineer | `examples/simple-agent/` or `cmd/simple-agent`; README |
| **WP6.4** | SDK documentation (README, godoc) and second example (e.g. echo/research agent). | Go Engineer | `pkg/sdk/README.md` or `docs/sdk.md`; `examples/`; run instructions |
| **WP6.5** | (Optional) Goal helpers: CreateGoal, WaitForCompletion in SDK. | Go Engineer | `pkg/sdk/goal.go`; README and example update |

**Cross-phase note:** WP6.1 and WP6.2 can start once Phase 4 services (or stubs) are available. WP6.3 and WP6.4 depend on WP6.1 and WP6.2. Phase 5 and Phase 6 can run in parallel after Phase 4: same Tech Lead coordinates; different owners (e.g. DevOps/QA for P5, Go for P6).

---

## 6. Tech Lead: completion and PRD currency

When **Phase 5** is complete:

1. **`scripts/validate.sh`** — Replace the Phase 5 placeholder `skip_test` calls with real checks (e.g. load test run or result file check, dashboard JSON present, alert rules present, runbooks present, Helm template/dry-run, cost/SLO metrics or alerts). Preserve existing Phase 0–4 and Phase 6 sections.
2. **PRD §25** — Update the Phase 5 roadmap: check off load tests, Grafana dashboards, alerting, runbooks, cost tracking, SLO enforcement, Helm hardening. Mark Phase 5 as complete (or in progress) per project state.
3. **Phase history** — Add or update `docs/phase-history/phase-5.md` with what was built (WPs delivered, key paths, sign-off checklist from phase-5.md).

When **Phase 6** is complete:

1. **`scripts/validate.sh`** — Replace the Phase 6 placeholder `skip_test` calls with real checks (e.g. `pkg/sdk` builds, no `internal` imports in `pkg/sdk`, SimpleAgent build/run or example test, SDK README exists).
2. **PRD §25** — Update the Phase 6 roadmap: check off SDK package, SimpleAgent, documentation and examples. Mark Phase 6 as complete (or in progress).
3. **Phase history** — Add or update `docs/phase-history/phase-6.md` with what was built (WPs delivered, `pkg/sdk` layout, examples, sign-off checklist from phase-6.md).

Per PRD currency rule: do not mark a phase done in the PRD until the corresponding validate.sh and phase-history updates are done.

---

## 7. Sign-off reference

- **Phase 5:** Use the “Sign-off (Phase 5 complete)” checklist in `docs/implementation-plans/phase-5.md`.
- **Phase 6:** Use the “Sign-off (Phase 6 complete)” checklist in `docs/implementation-plans/phase-6.md`.

Security compliance (S1–S6) must be verified at the gate for any code or config that touches auth, networking, tools, or secrets; use the checklist in `.cursor/rules/SECURITY-RULE.mdc`.
