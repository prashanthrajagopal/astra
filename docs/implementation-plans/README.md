# Astra Implementation Plans — Phases 1–6

**Audience:** Tech Lead. Use these plans to delegate work to Go Engineer, DevOps Engineer, QA Engineer, and DB Architect.

**Source:** PRD Section 25 (Implementation Roadmap), Section 26 (Build Order), and related PRD sections. Example delegation style: `docs/phase0-completion-checklist.md`.

---

## Purpose

Each phase plan is self-contained and includes:

- **Phase goal** — One sentence objective
- **Dependencies** — Previous phase and prerequisites
- **Work packages** — Concrete WP with tasks, owner, deliverables, acceptance criteria
- **Delegation hints** — Who owns what and hand-offs
- **Ordering** — Sequence and parallelization within the phase
- **Risks / open decisions** — Items for Architect or Product

The Tech Lead can open `phase-N.md` and assign work packages to the team; each task is actionable with a clear deliverable and acceptance.

---

## Phase index

| Phase | File | Goal (summary) |
|-------|------|----------------|
| **1** | [phase-1.md](phase-1.md) | Kernel MVP: actors, tasks, scheduler, messaging, events, api-gateway, agent-service, task-service, stub worker; E2E flow with events in Postgres |
| **2** | [phase-2.md](phase-2.md) | Workers & tool runtime: worker registration/heartbeat, execution-worker with Docker sandbox, worker-manager, tool-runtime, browser-worker |
| **3** | [phase-3.md](phase-3.md) | Memory & LLM routing: memory-service (pgvector), LLM router + cache, prompt-manager; 10ms read path (Redis/Memcached) |
| **4** | [phase-4.md](phase-4.md) | Orchestration, eval, security: LLM planner, goal-service, evaluation-service, identity (JWT), access-control (OPA), approval gates, usage audit |
| **5** | [phase-5.md](phase-5.md) | Scale & production: load test, Grafana dashboards, Prometheus alerting, runbooks, cost tracking, Helm (HPA, PDB, limits) |
| **6** | [phase-6.md](phase-6.md) | SDK & applications: AgentContext, MemoryClient, ToolClient, SimpleAgent example, SDK docs and examples |

---

## Dependency chain

```
Phase 0 (Prep) → Phase 1 (Kernel MVP) → Phase 2 (Workers & Tools) → Phase 3 (Memory & LLM)
                                                                        ↓
Phase 6 (SDK) ← Phase 5 (Scale & Hardening) ← Phase 4 (Orchestration, Eval, Security)
```

Phase 6 can start after Phase 4 for minimal SDK; full production readiness assumes Phase 5 complete.

---

## References

- **PRD:** `docs/PRD.md` — §4 Monorepo, §5–9 Kernel/Services, §10–12 Proto/DB/Events, §17 Observability, §22 Cost, §25 Roadmap, §26 Build Order
- **Phase 0 checklist:** `docs/phase0-completion-checklist.md`
- **Phase history / usage / audit design:** `docs/phase-history-usage-audit-design.md`
- **Security:** `.cursor/rules/SECURITY-RULE.mdc` (S1–S6)
- **Migrations:** `migrations/*.sql`; **Proto:** `proto/kernel/`, `proto/tasks/`
