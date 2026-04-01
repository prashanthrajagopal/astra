# Astra–Olympus Implementation Status

**Last updated:** 2026-04-01
**Astra version:** v0.2.0 (Phase 11 complete — multi-tenant)
**Source of truth:** `docs/PRD.md`

Tracks Astra capabilities required for the Olympus LA28 Olympic Agent System. `docs/PRD.md` remains the canonical specification.

---

## Completed Phases

Phases 0-11 are fully implemented and production-ready.

| Phase | Feature | Status | Evidence |
|-------|---------|--------|---------|
| Phase 0–8 | Kernel, workers, memory, scheduler, tools, auth, dashboard, codegen, observability | ✅ COMPLETE | Migrations 0000–0012; all 16 services in `cmd/` |
| Phase 9 | Agent profile & documents (system_prompt, rules, skills, context docs, context assembly) | ✅ COMPLETE | Migration `0013_agent_profile_and_documents.sql`; `internal/agentdocs/store.go`, `context.go` |
| Phase 10 | Chat agents (WebSocket streaming, sessions, tool invocation, LLM routing) | ✅ COMPLETE | Migration `0016_chat.sql`; `internal/chat/handler.go`, `store.go`, `protocol.go` |
| Phase 11 | Multi-tenancy (orgs, teams, users, RBAC, agent visibility global/public/team/private) | ✅ COMPLETE | Migration `0018_multi_tenant.sql`; `internal/orgs/`, `internal/rbac/` |

> **Note:** `docs/enhancements to astra.md` (March 2026) listed Phases 9 and 11 as "NOT complete." That predates implementation. Phases 9, 10, and 11 are complete as of v0.2.0.

---

## Partially Implemented

Foundational work exists but specific fields, auth paths, or logic are missing.

| Feature | What Exists | What Is Missing |
|---------|------------|----------------|
| **Agent-to-agent goal posting** | `pkg/sdk/goal.go` — `CreateGoal(ctx, agentID, goalText, priority)` allows any caller to post goals via REST | `goals.source_agent_id` DB column; service-to-service JWT (agent mints short-lived JWT for goal-service); per-agent rate limiting (max 10 posts/task); cascade depth guard (max depth 5) |
| **Approval system** | `approval_requests` table with `plan` and `risky_task` types; single approver; dashboard UI | `required_approvals INT DEFAULT 1` column; `approvals JSONB DEFAULT '[]'` column (array of `{user_id, decision, decided_at, note}`); REST `POST /approvals/{id}/decide` endpoint; dual-approval threshold logic |

---

## Remaining Gaps (14 items)

Not yet implemented. Full specifications in `docs/PRD.md` (Phases 12-14).

| # | Gap Name | Category | PRD Sections | Migration | Status |
|---|----------|----------|--------------|-----------|--------|
| 1 | Slack integration (Phase 12) | New service | §9, §12, §26 | No | Not started |
| 2 | External agent adapter framework | New package + services | §9, §15, §26 | No | Not started |
| 3 | Webhook ingest service | New service | §9, §11, §12, §26 | Yes (0019) | Not started |
| 4 | Goal-level dependency engine | New package | §10, §11, §26 | Yes (0020) | Not started |
| 5 | GoalCompleted event publication | Modify goal-service | §12, §26 | No | Not started |
| 6 | Goal cascade fields (cascade_id, depends_on_goal_ids) | Schema + API | §10, §11, §26 | Yes (0020) | Not started |
| 7 | Agent-to-agent goal posting (complete) | SDK + service | §11, §15, §26 | Yes (0020) | Partial (see above) |
| 8 | Agent tags, metadata, trust_score | Schema + API | §9, §11, §26 | Yes (0021) | Not started |
| 9 | Dual-approval (two-person rule) | Schema + logic | §11, §18, §26 | Yes (0022) | Partial (see above) |
| 10 | Tool definitions registry | Schema + API | §11, §14, §26 | Yes (0023) | Not started |
| 11 | Chat session external message injection | New API endpoint | §9, §26 | No | Not started |
| 12 | Approval REST API (programmatic decide) | New API endpoint | §9, §26 | No | Not started |
| 13 | Goal priority in scheduler + agent concurrency limits | Logic changes | §8, §26 | No | Not started |
| 14 | Trust score storage + events | Schema (merged with #8) | §11, §12, §26 | Yes (0021) | Not started |

---

## Migration Sequence

New migrations continuing from 0018.

| Migration | Filename | Changes |
|-----------|----------|---------|
| 0019 | `0019_webhook_sources.sql` | `webhook_sources` table (source_id, hmac_secret, schema_type, enabled, org_id) |
| 0020 | `0020_goal_dependencies.sql` | ALTER `goals`: add cascade_id UUID, depends_on_goal_ids UUID[], completed_at TIMESTAMPTZ, source_agent_id UUID |
| 0021 | `0021_agent_tags_trust.sql` | ALTER `agents`: add tags TEXT[], metadata JSONB, trust_score FLOAT; CREATE `agent_trust_events` table |
| 0022 | `0022_dual_approval.sql` | ALTER `approval_requests`: add required_approvals INT DEFAULT 1, approvals JSONB DEFAULT '[]' |
| 0023 | `0023_tool_definitions.sql` | CREATE `tool_definitions` table (name, version, risk_tier CHECK, sandbox CHECK, description, metadata) |

---

## New Redis Keys / Streams

| Key / Stream | Type | Purpose |
|-------------|------|---------|
| `astra:goals:completed` | Stream | GoalCompleted events (goal_id, cascade_id, status, result_summary) |
| `astra:slack:incoming` | Stream | Raw Slack events (slash commands, reactions, DMs) |
| `astra:slack:outgoing` | Stream | Outbound Slack messages queued for delivery |
| `astra:webhooks:raw` | Stream | Raw normalized webhook payloads from external sources |
| `agent:trust:<id>` | Key (TTL 5m) | Cached trust score; invalidated on trust event |
| `agent:goalpost:rate:<id>` | Key (TTL 60s) | Per-agent goal-post rate limit counter (max 10) |
| `agent:active_goals:<id>` | Key (TTL 30s) | Active goal count cache for concurrency admission control |

---

## New Services / Packages

| Name | Type | Port | Purpose |
|------|------|------|---------|
| `cmd/slack-adapter` | New service | 8091 | Slack Events API receiver; HMAC verification; dispatches to stream |
| `cmd/slack-worker` | New service | — | Redis stream consumer; routes Slack events to goal-service or access-control |
| `cmd/webhook-ingest` | New service | 8092 | Generic webhook receiver; HMAC validation; publishes to astra:webhooks:raw |
| `cmd/dtec-adapter` | New service | — | D.TEC ecosystem adapter (June 1 target) |
| `internal/adapters` | New package | — | Adapter interface (DispatchGoal, PollStatus, HandleCallback, ListCapabilities, HealthCheck) |
| `internal/goals` | New package | — | Goal dependency engine (DependencyEngine, OnGoalCompleted); does not exist today |
| `internal/slack` | New package | — | Slack API client, message formatting, OAuth token management |
