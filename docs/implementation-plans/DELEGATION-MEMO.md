# Delegation Memo — Phases 1–6 Implementation

**To:** Tech Lead  
**From:** Principal Architect  
**Re:** Implementation of Phases 1–6 per implementation plans  
**Date:** 2025-03-09

---

## 1. Delegation

Implementation of **Phases 1–6** is delegated to the **Tech Lead** per the following plans:

- `docs/implementation-plans/phase-1.md` — Kernel MVP  
- `docs/implementation-plans/phase-2.md` — Workers & Tool Runtime  
- `docs/implementation-plans/phase-3.md` — Memory & LLM Routing  
- `docs/implementation-plans/phase-4.md` — Orchestration, Eval, Security  
- `docs/implementation-plans/phase-5.md` — Scale & Production Hardening  
- `docs/implementation-plans/phase-6.md` — SDK & Applications  

Execute phases **in order**: Phase 1 → Phase 2 → Phase 3 → Phase 4 → Phase 5 → Phase 6. A phase is complete only when **all** of its work packages are done and the **acceptance criteria** in that phase’s plan are met.

---

## 2. Phase 1 work packages

For Phase 1, assign each work package to the role specified in the phase plan. The Tech Lead **coordinates and tracks** completion; do **not** implement or run shell commands yourself.

| ID     | Summary                          | Assign to (per plan)     |
|--------|----------------------------------|--------------------------|
| WP1.1  | Shared packages (pkg/)           | Go Engineer; DevOps (CI) |
| WP1.2  | Event store (internal/events)    | Go Engineer; QA/Go       |
| WP1.3  | Messaging / Redis Streams        | Go Engineer; QA/Go       |
| WP1.4  | Actor runtime (internal/actors)  | Go Engineer             |
| WP1.5  | Kernel manager (internal/kernel) | Go Engineer             |
| WP1.6  | Task model and state machine     | Go Engineer; QA/Go       |
| WP1.7  | Scheduler loop                   | Go Engineer; QA/Go       |
| WP1.8  | Planner stub                     | Go Engineer             |
| WP1.9  | Agent lifecycle                  | Go Engineer; QA/Go       |
| WP1.10 | gRPC services and API surface    | Go Engineer; QA/Go       |
| WP1.11 | cmd/entrypoints                  | Go Engineer; DevOps      |
| WP1.12 | End-to-end and integration tests | QA; Go Engineer          |

Assign each WP to the **appropriate role** (Go Engineer, DevOps Engineer, QA Engineer, DB Architect) as specified in the task tables and delegation hints in `phase-1.md`. The Tech Lead coordinates dependencies and ordering per Section 5 of the phase plan.

---

## 3. Tech Lead constraints

- **Do not write code or run shell commands.** Your role is to delegate, coordinate, and review.
- Delegate implementation to **Go Engineer**, **DevOps Engineer**, **QA Engineer**, and **DB Architect** only.
- Track progress, unblock dependencies, and ensure deliverables and acceptance criteria from each plan are met before marking a phase complete.

---

## 4. Phase completion and phase history

When **Phase 1** is fully complete (all WPs done, acceptance criteria in `phase-1.md` met), the Tech Lead must:

1. **Update the PRD** — ensure `docs/PRD.md` is updated (e.g. roadmap checkboxes, new sections, or schema) to reflect the completed phase before marking the phase done.
2. **Update phase history** — e.g. add or update `docs/phase-history/phase-1.md` to record completion (date, sign-off checklist, any notes).
3. **Then proceed to Phase 2** — open `phase-2.md`, assign WP2.1–WP2.6 to the appropriate roles, and repeat.

The same rule applies for every phase: when a phase is complete, update the PRD and phase history (e.g. `docs/phase-history/phase-N.md`), and only then start the next phase.

---

## 5. Summary

- **Phases:** 1 → 2 → 3 → 4 → 5 → 6, in order.  
- **Phase 1:** Assign WP1.1–WP1.12 per `phase-1.md`; coordinate and track; no implementation by Tech Lead.  
- **Done =** all WPs delivered + acceptance criteria met.  
- **After each phase:** update `docs/PRD.md` (roadmap, schema, or new sections as needed), then update `docs/phase-history/phase-N.md`, then begin the next phase.
