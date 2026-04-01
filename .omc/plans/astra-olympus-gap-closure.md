# Work Plan: Astra-Olympus Gap Closure (PRD + Status Doc)

**Plan ID:** astra-olympus-gaps-20260401
**Type:** Documentation / Specification Update
**Estimated Complexity:** MEDIUM
**Files Modified:** 2 (docs/PRD.md, docs/olympus-implementation-status.md)

---

## RALPLAN-DR Summary (Short Mode)

### Principles (5)

1. **PRD is single source of truth** -- All architectural specs, schema DDL, API contracts, and roadmap items belong in `docs/PRD.md`. No shadow specs.
2. **Additive only** -- Existing PRD sections must not be modified; new subsections are appended within existing numbered sections. **Exception:** Section 9 header, service count table, and ToC anchor text are factual metadata (not architectural prose) and must be updated in-place to remain accurate — this is a correction, not a behavioral change.
3. **Style consistency** -- New content must match existing PRD conventions: SQL DDL in fenced blocks, Go interfaces in fenced blocks, REST endpoints in markdown tables, Redis streams as numbered subsections with field tables.
4. **Sequential migration numbering** -- Migrations continue from the current head (0018) as 0019-0023. No gaps, no aspirational numbers.
5. **All 14 gaps covered** -- Nothing deferred from the spec. Every gap gets its PRD subsection with the appropriate artifacts (DDL, API, package location, Redis stream, acceptance snippet).

### Decision Drivers (top 3)

1. **Executor clarity** -- The plan must produce PRD content that an executor can implement against without ambiguity about where code goes, what the schema looks like, or what the API contract is.
2. **Minimal PRD disruption** -- A 2400-line PRD is already well-structured. Changes must slot cleanly into existing sections without restructuring.
3. **Accuracy of status tracking** -- The status doc must correctly reflect what is actually implemented (Phases 9-11 done) vs. the 14 remaining gaps.

### Viable Options (2)

**Option A: In-place PRD update + separate status doc (RECOMMENDED)**
- Pros: Preserves PRD as single source of truth; status doc is a lightweight living tracker; matches the spec's explicit requirements; executor can find all specs in one place.
- Cons: PRD grows by ~400-500 lines; larger file to navigate.

**Option B: Separate extension document with PRD cross-references**
- Pros: Keeps PRD at current size; extension doc is self-contained.
- Cons: Violates the project's "PRD is single source of truth" principle (CLAUDE.md explicitly states this); creates a second place to look for specs; risks going stale independently; the existing `docs/enhancements to astra.md` already demonstrated this failure mode (it had incorrect implementation status).

**Decision: Option A.** Option B is invalidated by the project's own PRD currency rule and the historical evidence that the enhancement doc went stale. The PRD growth is manageable (~20% increase) and the content is additive subsections, not restructuring.

---

## 1. Requirements Summary

Update `docs/PRD.md` with full-fidelity specifications for 14 remaining Astra-Olympus integration gaps, and create `docs/olympus-implementation-status.md` as a living accuracy-corrected status tracker.

**Scope of PRD changes by section:**

| PRD Section | Gaps Covered | Content Type |
|---|---|---|
| Section 8 (Scheduler) | Gap 13 | Priority reorder buffer, concurrency limit logic |
| Section 9 (Services) | Gaps 1, 2, 3, 11, 12 | New service specs (slack-adapter, webhook-ingest), new endpoints (chat injection, approval REST) |
| Section 10 (gRPC Contracts) | Gaps 4, 6 | GoalService proto additions (cascade_id, depends_on_goal_ids, source_agent_id) |
| Section 11 (Schema) | Gaps 3, 4, 6, 7, 8, 9, 10, 14 | Migrations 0019-0023 DDL |
| Section 12 (Message Protocols) | Gaps 1, 5 | New streams: astra:goals:completed, astra:slack:*, astra:webhooks:raw |
| Section 14 (Tool Runtime) | Gap 10 | Tool definitions registry, risk-tier gating |
| Section 15 (SDK) | Gaps 2, 7 | PostGoal agent-to-agent method, adapter interface |
| Section 18 (Security) | Gap 9 | Dual-approval two-person rule |
| Section 26 (Roadmap) | All 14 | Phase 12+ items with sequencing |

**Status document scope:**
- Accurate status for Phases 9, 10, 11 (all COMPLETE)
- 14 remaining gaps with category, migration needs, and implementation status
- Partially implemented items (agent-to-agent goal posting, approval system)

---

## 2. Acceptance Criteria

- [ ] **AC1:** `docs/olympus-implementation-status.md` exists with a status table marking Phases 9/10/11 as complete and all 14 gaps as remaining
- [ ] **AC2:** PRD Section 11 contains DDL for migrations 0019 (`webhook_sources`), 0020 (`goal_dependencies`), 0021 (`agent_tags_trust`), 0022 (`dual_approval`), 0023 (`tool_definitions`) -- each with exact SQL matching the spec
- [ ] **AC3:** PRD Section 9 contains new service specs for `slack-adapter` (port 8091), `slack-worker`, and `webhook-ingest` (port 8092) with REST endpoint tables
- [ ] **AC4:** PRD Section 9 contains chat session external message injection endpoint (`POST /chat/sessions/{session_id}/messages`)
- [ ] **AC5:** PRD Section 9 contains programmatic approval REST API spec (`POST /approvals/{id}/decide`, `GET /approvals`)
- [ ] **AC6:** PRD Section 10 contains GoalService proto additions: `CreateGoalRequest` fields (cascade_id, depends_on_goal_ids, source_agent_id) and `GoalCompletedEvent` message
- [ ] **AC7:** PRD Section 12 contains stream specs for `astra:goals:completed`, `astra:slack:incoming`, `astra:slack:outgoing`, `astra:webhooks:raw` with field tables
- [ ] **AC8:** PRD Section 14 contains `tool_definitions` registry spec with API table and risk-tier gating logic
- [ ] **AC9:** PRD Section 15 contains `PostGoal` SDK method signature and adapter framework interface
- [ ] **AC10:** PRD Section 18 contains dual-approval logic spec (two-person rule, fail-fast rejection)
- [ ] **AC11:** PRD Section 8 contains priority-aware scheduler reorder buffer and agent concurrency limit specs
- [ ] **AC12:** PRD Section 26 contains Phase 12+ roadmap items with correct sequencing and checklist format
- [ ] **AC13:** All new Redis keys/streams are documented: `agent:trust:<id>`, `agent:goalpost:rate:<id>`, `agent:active_goals:<id>`
- [ ] **AC14:** Every gap specifies: Go package location, and (where applicable) migration DDL, REST/gRPC API shape, Redis stream, acceptance snippet
- [ ] **AC15:** No existing PRD content is modified -- all changes are additive (new subsections appended within existing sections)
- [ ] **AC16:** New PRD content matches existing style: SQL in fenced `sql` blocks, Go in fenced `go` blocks, REST in markdown tables, streams as numbered subsections with field tables
- [ ] **AC17:** Section 9 header updated from "16 Canonical Microservices" to "21 Canonical Microservices"; service table adds `cost-tracker` (row #17, port 8090, which was omitted from the PRD table despite existing in `cmd/cost-tracker/`) and 4 new services (slack-adapter #18, slack-worker #19, webhook-ingest #20, dtec-adapter #21); PRD Table of Contents anchor for Section 9 updated to match
- [ ] **AC18:** PRD Section 12 Core Message Types table (Goal lifecycle row) includes `GoalCompleted` event type

---

## 3. Implementation Steps

### Step 1: Create `docs/olympus-implementation-status.md`

**What:** Create the living status document with accurate implementation state.

**Content structure:**
1. Header with document purpose and last-updated date
2. "Completed Phases" table -- Phases 9, 10, 11 with evidence (migration files, package paths)
3. "Partially Implemented" table -- agent-to-agent goal posting (has `pkg/sdk/goal.go` CreateGoal, missing `source_agent_id` field + service JWT + rate limiting), approval system (has `approval_requests` table, missing `required_approvals` + `approvals JSONB`)
4. "Remaining Gaps" table -- all 14 gaps with columns: #, Gap Name, Category, PRD Section(s), Migration Needed (Y/N), Status (Not Started)
5. "Migration Sequence" -- 0019-0023 with filenames
6. "New Redis Keys/Streams" summary
7. "New Services/Packages" summary

**Acceptance:** File exists, all 14 gaps listed, Phases 9-11 marked complete with evidence, partially-implemented items accurately described.

---

### Step 2: Update PRD Section 11 (Database Schema) -- Migrations 0019-0023

**What:** Append 5 new migration subsections after the existing Migration 0018 content (which ends around line 1335).

**Insert after:** The last migration subsection in Section 11 (Migration 0018: Multi-Tenant). Append before the `---` separator preceding Section 12.

**New subsections to add:**
- `## Migration 0019: Webhook Sources` -- `webhook_sources` table DDL (Gap 3)
- `## Migration 0020: Goal Dependencies` -- ALTER goals with cascade_id, depends_on_goal_ids, completed_at, source_agent_id + indexes (Gaps 4, 6, 7)
- `## Migration 0021: Agent Tags & Trust` -- ALTER agents with tags, metadata, trust_score + `agent_trust_events` table (Gaps 8, 14)
- `## Migration 0022: Dual Approval` -- ALTER approval_requests with required_approvals, approvals JSONB (Gap 9)
- `## Migration 0023: Tool Definitions` -- `tool_definitions` table DDL (Gap 10)

**DDL content:** Use exact SQL from `.omc/specs/deep-interview-astra-olympus-gaps.md` (Gaps 3, 4+6, 8+14, 9, 10 sections respectively). Only adjust formatting (indentation, fencing) to match PRD style.

**Acceptance:** Each migration has a subsection header matching PRD style (`## Migration 00XX: Name`), contains idempotent DDL (`IF NOT EXISTS`, `IF EXISTS`), and includes indexes.

---

### Step 3: Update PRD Sections 8, 9, 10, 12, 14, 15, 18 -- Service Specs, APIs, Protocols

**What:** Append new subsections within each referenced PRD section for the non-schema gaps.

**Section 8 (Scheduler) -- insert after existing content, before Section 9:**
- Subsection: `## Goal Priority & Agent Concurrency Limits (Phase 12+)` -- priority-aware reorder buffer spec, `config.max_concurrent_goals` admission control, 429 response shape, Redis key `agent:active_goals:<id>` (Gap 13)

**Section 9 (Services) -- in-place factual corrections + new subsections (Principle 2 exception applies):**
- Update PRD Table of Contents anchor for Section 9 (currently "16 Canonical Microservices") to "21 Canonical Microservices"
- Update Section 9 header from "16 Canonical Microservices" to "21 Canonical Microservices"
- Add `cost-tracker` as row #17 to the service table (port 8090 — already exists in `cmd/cost-tracker/` but was omitted from the PRD table)
- Add 4 new service rows: slack-adapter #18, slack-worker #19, webhook-ingest #20, dtec-adapter #21
- Subsection: `## Slack Integration (Phase 12)` -- slack-adapter architecture, slash commands, Go packages, secrets (Gap 1)
- Subsection: `## External Agent Adapter Framework` -- adapter interface, integration with execution-worker, new services (Gap 2)
- Subsection: `## Webhook Ingest Service` -- endpoint, processing flow, Redis stream (Gap 3)
- Subsection: `## Chat External Message Injection` -- POST /chat/sessions/{session_id}/messages spec (Gap 11)
- Subsection: `## Approval REST API (Programmatic)` -- POST /approvals/{id}/decide, GET /approvals endpoints, idempotency (Gap 12)

**Section 10 (gRPC Contracts) -- append after existing `task.proto` section:**
- Subsection: `## goal.proto (Phase 12+ additions)` -- CreateGoalRequest field additions, GoalCompletedEvent message (Gaps 4, 6)

**Section 12 (Message & Event Protocols) -- append after stream #6 (astra:usage):**
- Stream `### 7. astra:goals:completed` -- GoalCompleted event fields (Gap 5)
- Stream `### 8. astra:slack:incoming` -- raw Slack events (Gap 1)
- Stream `### 9. astra:slack:outgoing` -- outbound Slack messages (Gap 1)
- Stream `### 10. astra:webhooks:raw` -- raw webhook payloads (Gap 3)
- New Redis keys subsection: `agent:trust:<id>`, `agent:goalpost:rate:<id>`, `agent:active_goals:<id>` (Gaps 8, 7, 13)

**Section 14 (Tool Runtime) -- append after existing Go interface:**
- Subsection: `## Tool Definitions Registry (Phase 12+)` -- API table, risk-tier gating logic, trust score bypass (Gap 10)

**Section 15 (SDK) -- append after Agent Skeleton:**
- Subsection: `## Agent-to-Agent Goal Posting` -- PostGoal method, service-to-service auth, rate limiting, cascade depth guard (Gap 7)
- Subsection: `## External Agent Adapter Interface` -- Adapter interface definition from spec (Gap 2)

**Section 18 (Security) -- append after Tool & Action Governance:**
- Subsection: `## Dual-Approval (Two-Person Rule)` -- required_approvals logic, fail-fast rejection, API response shape (Gap 9)

**Acceptance:** Each subsection uses the correct PRD style. No existing content is modified. All Go interfaces, REST tables, and Redis stream field tables are present.

---

### Step 4: Update PRD Section 26 (Implementation Roadmap) -- Phase 12+

**What:** Append new phase definitions after Phase 11.

**Insert after:** Phase 11 content (line ~2373), before the `---` separator preceding Section 27.

**New phases to add:**

```
## Phase 12 -- Olympus Integration Foundation (6-8 weeks)

Goal: Schema extensions, goal dependencies, agent metadata, and approval enhancements
that enable Olympus orchestration.

- [ ] Migration 0019: webhook_sources table
- [ ] Migration 0020: goal dependencies (cascade_id, depends_on_goal_ids, source_agent_id)
- [ ] Migration 0021: agent tags, metadata, trust_score + agent_trust_events
- [ ] Migration 0022: dual-approval (required_approvals, approvals JSONB)
- [ ] Migration 0023: tool_definitions registry
- [ ] `internal/goals/` -- **new package** (does not exist today; goal logic currently lives in `cmd/goal-service/`). Create `internal/goals/deps.go` with DependencyEngine (OnGoalCompleted). Add to Section 27 Build Order.
- [ ] GoalCompleted event publication to astra:goals:completed
- [ ] Agent-to-agent PostGoal in pkg/sdk (service JWT, rate limiting, cascade depth)
- [ ] Agent tags/metadata/trust_score API extensions
- [ ] Dual-approval logic in internal/rbac
- [ ] Approval REST API (programmatic decide)
- [ ] Tool definitions registry API + risk-tier gating
- [ ] Goal priority reorder buffer in scheduler
- [ ] Agent concurrency limits (max_concurrent_goals)
- [ ] Chat external message injection endpoint

## Phase 13 -- External Integrations (4-6 weeks)

Goal: Slack integration and webhook ingestion for external event-driven triggers.

- [ ] cmd/slack-adapter (port 8091) + internal/slack/
- [ ] cmd/slack-worker (Redis stream consumer)
- [ ] Slack streams: astra:slack:incoming, astra:slack:outgoing
- [ ] cmd/webhook-ingest (port 8092)
- [ ] Webhook stream: astra:webhooks:raw

## Phase 14 -- Adapter Framework (4-6 weeks)

Goal: External agent adapter framework for cross-platform orchestration.

- [ ] internal/adapters/ package with Adapter interface
- [ ] cmd/dtec-adapter (first adapter, June 1 target)
- [ ] execution-worker adapter dispatch integration
- [ ] cmd/agentforce-adapter, cmd/workday-adapter (deferred to EOY)
```

**Acceptance:** Phase 12-14 items cover all 14 gaps. Each phase has a Goal line, checklist items, and follows the existing phase format.

---

### Step 5: Final Validation Pass

**What:** Review the completed PRD changes for consistency and completeness.

**Checks:**
1. All 14 gaps from the spec have at least one subsection in the PRD
2. Migration numbers are sequential (0019-0023) with no gaps or duplicates
3. New service count matches updated Section 9 header
4. No existing PRD text was modified (only additions)
5. All fenced code blocks have correct language tags (sql, go, protobuf)
6. Cross-references between sections are consistent (e.g., Section 11 migration referenced by Section 9 service spec)
7. Status doc accurately reflects the PRD additions
8. Redis key/stream names are consistent between Section 12 and Section 13 (caching)

**Acceptance:** Zero inconsistencies found. Every gap maps to at least one PRD subsection. Status doc matches PRD content.

---

## 4. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| PRD insertion point drift if PRD is edited concurrently | Low | Medium | Use unique section headers as anchors, not line numbers. Verify anchors before inserting. |
| Migration number collision if another PR adds 0019 first | Low | High | Check `migrations/` directory immediately before writing. Re-number if needed. |
| Inconsistency between spec DDL and PRD style | Medium | Low | Use spec DDL verbatim for correctness; only adjust formatting (indentation, fencing) to match PRD style. |
| Missing gap coverage | Low | High | Use the 14-gap checklist as a completion tracker. Verify each gap has PRD content before marking step complete. |

---

## 5. Verification Steps

1. **Gap coverage check:** For each of the 14 gaps, confirm the PRD has: (a) at least one subsection, (b) package location, (c) migration DDL if applicable, (d) API shape if applicable, (e) Redis stream if applicable.
2. **Diff review:** `git diff docs/PRD.md` should show only additions (no deletions, no modifications to existing lines).
3. **Status doc review:** `docs/olympus-implementation-status.md` should list all 14 gaps and correctly mark Phases 9-11 as complete.
4. **Style check:** Spot-check 3 new subsections against adjacent existing subsections for style match (table formatting, code fencing, header levels).
5. **Migration sequence:** Verify 0019-0023 are sequential and no file in `migrations/` already uses those numbers.
6. **Build check:** `go vet ./...` should pass (no code changes, but verifies nothing is broken).
7. **Section header count:** Section 9 header says "21 Canonical Microservices" and the service table has exactly 21 rows (17 existing including cost-tracker + 4 new). ToC anchor matches.

---

## Gap-to-Step Mapping

| Gap # | Gap Name | Steps Covered |
|---|---|---|
| 1 | Slack integration | Steps 3 (S9, S12), 4 (Phase 13) |
| 2 | External agent adapter framework | Steps 3 (S9, S15), 4 (Phase 14) |
| 3 | Webhook ingest service | Steps 2 (S11), 3 (S9, S12), 4 (Phase 13) |
| 4 | Goal-level dependency engine | Steps 2 (S11), 3 (S10), 4 (Phase 12) |
| 5 | GoalCompleted event publication | Steps 3 (S12), 4 (Phase 12) |
| 6 | Goal cascade fields | Steps 2 (S11), 3 (S10), 4 (Phase 12) |
| 7 | Agent-to-agent goal posting | Steps 2 (S11), 3 (S15), 4 (Phase 12) |
| 8 | Agent tags/metadata/trust_score | Steps 2 (S11), 3 (S9), 4 (Phase 12) |
| 9 | Dual-approval | Steps 2 (S11), 3 (S18), 4 (Phase 12) |
| 10 | Tool definitions registry | Steps 2 (S11), 3 (S14), 4 (Phase 12) |
| 11 | Chat external message injection | Steps 3 (S9), 4 (Phase 12) |
| 12 | Approval REST API | Steps 3 (S9), 4 (Phase 12) |
| 13 | Goal priority + concurrency | Steps 3 (S8), 4 (Phase 12) |
| 14 | Trust score (merged with #8) | Steps 2 (S11), 3 (S12), 4 (Phase 12) |

---

## ADR: In-Place PRD Update + Separate Status Doc

**Decision:** Extend `docs/PRD.md` in-place with new subsections for all 14 gaps (Option A), and create `docs/olympus-implementation-status.md` as a separate living status tracker.

**Drivers:**
1. PRD is the contractual single source of truth per CLAUDE.md and the PRD's own maintenance policy (PRD §1)
2. Executor clarity requires all specs in one canonical location
3. Accuracy of status tracking must not conflict with PRD completeness

**Alternatives Considered:**

| Option | Summary | Why Rejected |
|--------|---------|-------------|
| B: Separate extension doc | New `docs/olympus-spec.md` with PRD cross-references | Violated PRD currency rule; historical evidence — `docs/enhancements to astra.md` went stale with incorrect Phase 9/11 status within weeks of writing; the failure was structural, not process |
| A+B hybrid | Extension doc for new gaps, PRD only for completed phases | Creates two places to look up specs; future contributors won't know which is authoritative |

**Why Option A:** The PRD currency rule is explicit in CLAUDE.md ("Never add behavior that is not in the PRD without updating it first"). A 20% growth in PRD size is acceptable given the content is purely additive subsections within existing sections. The `olympus-implementation-status.md` document satisfies the "living tracker" need without undermining PRD authority.

**Consequences:**
- PRD grows from ~2400 to ~2900 lines — acceptable; sections remain navigable via ToC
- Status doc becomes the "progress view"; PRD remains the "contract"
- Future PRD readers will see Phase 12–14 specs alongside Phases 0–11 — they are clearly scoped under new phase headers

**Follow-ups:**
- Consider adding a PRD navigation guide when it exceeds 3500 lines
- Phase 12 has 15 checklist items; may be split into 12a/12b during execution planning
- `agentforce-adapter` and `workday-adapter` are deferred to EOY; their service count is not included in the 21-service total

---

## Improvements Changelog

Applied during Architect + Critic consensus review:

1. **Service count corrected** (was 16+4=20, now 17+4=21) — `cost-tracker` was present in `cmd/` and PRD Phase 5 text but missing from the service table. Added as row #17. AC17 and Verification Step 7 updated.
2. **AC18 added** — GoalCompleted must appear in Core Message Types table (PRD Section 12, Goal lifecycle row). Not previously an explicit acceptance criterion.
3. **Principle 2 exception documented** — Section 9 header, service table, and ToC anchor updates are factual corrections (not architectural prose changes). Exception explicitly scoped in Principle 2.
4. **`internal/goals/` clarified as new package** — no such package exists today; goal logic currently in `cmd/goal-service/`. Step 4 Phase 12 checklist updated; Build Order impact noted.
5. **Verification Step 7 updated** — references 21 services (not 20); explicitly checks ToC anchor match.
6. **Spec file path made explicit** — Step 2 now references `.omc/specs/deep-interview-astra-olympus-gaps.md` by path instead of "spec file."
7. **ToC update added to AC17 scope** — PRD Table of Contents anchor for Section 9 must match the updated header.
