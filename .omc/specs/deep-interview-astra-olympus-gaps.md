# Deep Interview Spec: Astra–Olympus Gap Closure

## Metadata
- Interview ID: astra-olympus-gaps-20260401
- Rounds: 4
- Final Ambiguity Score: 14%
- Type: brownfield
- Generated: 2026-04-01
- Threshold: 20%
- Status: PASSED

## Clarity Breakdown
| Dimension | Score | Weight | Weighted |
|-----------|-------|--------|----------|
| Goal Clarity | 0.90 | 35% | 0.315 |
| Constraint Clarity | 0.85 | 25% | 0.213 |
| Success Criteria | 0.85 | 25% | 0.213 |
| Context Clarity | 0.80 | 15% | 0.120 |
| **Total Clarity** | | | **0.861** |
| **Ambiguity** | | | **14%** |

---

## Goal

Update `docs/PRD.md` with full-fidelity specifications (exact migration DDL, gRPC/REST contracts, Redis stream schemas, package-level implementation notes) for the 14 remaining unimplemented Astra–Olympus gaps. Simultaneously create `docs/olympus-implementation-status.md` as a living accuracy-corrected status document showing what is done vs. remaining.

---

## Constraints

- Match existing PRD style exactly: DDL snippets, Go interface signatures, REST endpoint tables, Redis key/stream patterns
- Migration numbers continue from `0018` (next is `0019`); the enhancement doc's suggestion of `0026–0029` is aspirational — use sequential numbering from current head
- Do not change the PRD's existing sections; append new subsections within the appropriate existing sections (Schema, Services, SDK, etc.)
- All 14 items must be covered; nothing deferred from the spec (deferral is an execution decision, not a spec decision)
- Enhancement doc (`docs/enhancements to astra.md`) is a historical artifact — leave it unchanged
- New status doc is a living doc that can be updated as items are implemented

---

## Non-Goals

- Implementing any code (this spec feeds into autopilot/ralplan for execution)
- Updating the enhancement doc itself
- Designing the Olympus-side components (capability catalog, cascade engine, triage agent — those stay in Olympus)
- Changing any existing PRD sections (additive only)

---

## Acceptance Criteria

- [ ] `docs/olympus-implementation-status.md` created with accurate status table (Phases 9/10/11 marked complete; 14 items marked remaining)
- [ ] PRD Section 11 (Database Schema) updated with DDL for migrations 0019–0023
- [ ] PRD Section 9 (Services) updated with Slack adapter service spec + webhook-ingest service spec
- [ ] PRD Section 10 (gRPC Contracts) updated with GoalService dependency fields and adapter interface
- [ ] PRD Section 12 (Message & Event Protocols) updated with `astra:goals:completed` stream and `astra:slack:*` streams
- [ ] PRD Section 15 (SDK) updated with `PostGoal` agent-to-agent method
- [ ] PRD Section 26 (Implementation Roadmap) updated with Phase 12+ items and their sequencing
- [ ] All 14 gaps have: migration DDL (if schema change), Go package location, REST/gRPC API shape, Redis stream (if applicable), acceptance criteria snippet

---

## Current Implementation Status (Brownfield Context)

### Already Fully Implemented (enhancement doc was outdated)

| Phase | Feature | Evidence |
|-------|---------|---------|
| Phase 9 | Agent profile & documents | `internal/agentdocs/store.go`, `context.go`; migration `0013_agent_profile_and_documents.sql` |
| Phase 10 | Chat agents (WebSocket streaming) | `internal/chat/handler.go`, `store.go`, `protocol.go`; migration `0016_chat.sql` |
| Phase 11 | Multi-tenancy (orgs/teams/roles) | `internal/orgs/`, `internal/rbac/`; migration `0018_multi_tenant.sql` |

### Partially Implemented

| Feature | Done | Missing |
|---------|------|---------|
| Agent-to-agent goal posting | `pkg/sdk/goal.go` has `CreateGoal()` | `goals.source_agent_id` DB field; service-to-service JWT auth; rate limiting |
| Approval system | `approval_requests` table with plan/risky_task types | `required_approvals INT`, `approvals JSONB[]` for dual-approval; REST decide endpoint |

### Not Implemented (14 gaps)

| # | Gap | Category | Migration Needed |
|---|-----|----------|-----------------|
| 1 | Slack integration (Phase 12) | New service | No |
| 2 | External agent adapter framework | New package + services | No |
| 3 | Webhook ingest service | New service | Yes (webhook_sources table) |
| 4 | Goal-level dependency engine | New package | Yes (goals schema ext.) |
| 5 | GoalCompleted event publication | Modify goal-service | No |
| 6 | Goal cascade fields (cascade_id, depends_on_goal_ids) | Schema + API | Yes |
| 7 | Agent-to-agent goal posting (complete) | SDK + service | Yes (source_agent_id) |
| 8 | Agent tags + metadata + trust_score | Schema | Yes |
| 9 | Dual-approval (two-person rule) | Schema + logic | Yes |
| 10 | Tool definitions registry | Schema + API | Yes |
| 11 | Chat session external message injection | API | No |
| 12 | Approval REST API (programmatic decide) | API | No |
| 13 | Goal priority in scheduler + concurrency limits | Logic | No |
| 14 | Trust score storage + events | Schema | Yes (merged with #8) |

---

## Full-Fidelity Specs for Each Gap

### Gap 1: Slack Integration (Phase 12)

**PRD Section:** 9 (Services) — new service `slack-adapter`; Section 12 (Message Protocols)

**New service:** `cmd/slack-adapter` (port 8091)

**Architecture:**
```
Slack API ──POST /slack/events──> cmd/slack-adapter
                                     │
                          XADD astra:slack:incoming
                                     │
                          cmd/slack-worker (consumer group)
                                     │
                    ┌────────────────┴──────────────┐
               POST /goals                  POST /approvals/{id}/decide
```

**Redis streams:**
- `astra:slack:incoming` — raw Slack events (slash commands, reactions, DMs)
- `astra:slack:outgoing` — messages queued for Slack delivery

**REST endpoints added to api-gateway:**
```
POST   /internal/slack/post          # proactive post to Slack channel/thread
POST   /slack/events                 # Slack Events API webhook receiver (HMAC verified)
POST   /slack/oauth/callback         # OAuth 2.0 callback
```

**Slash commands handled by slack-worker:**
- `/olympus-trigger <text>` → POST /goals with goal_text
- Reaction ✅ on approval message → POST /approvals/{id}/decide (approve)
- Reaction ❌ on approval message → POST /approvals/{id}/decide (reject)

**Go packages:**
- `cmd/slack-adapter/` — HTTP server, Slack signature verification, event dispatch
- `cmd/slack-worker/` — Redis stream consumer, routes events to goal-service or access-control
- `internal/slack/` — Slack API client, message formatting, OAuth token management

**Secrets:** Slack Bot Token, App Token, Signing Secret — injected from Vault at runtime, never in config files.

---

### Gap 2: External Agent Adapter Framework

**PRD Section:** 9 (Services); 15 (SDK); new package

**New package:** `internal/adapters`

**Go interface:**
```go
// internal/adapters/adapter.go
type Adapter interface {
    DispatchGoal(ctx context.Context, ref AdapterRef, goal AdapterGoal, agentCtx AgentContext) (jobID string, err error)
    PollStatus(ctx context.Context, jobID string) (status AdapterStatus, result string, err error)
    HandleCallback(ctx context.Context, payload []byte) error
    ListCapabilities(ctx context.Context) ([]Capability, error)
    HealthCheck(ctx context.Context) (HealthStatus, error)
}

type AdapterRef struct {
    AdapterID  string
    Ecosystem  string // "dtec" | "agentforce" | "workday"
    Endpoint   string
    AuthRef    string // Vault path to credentials
}

type AdapterStatus string
const (
    AdapterStatusRunning   AdapterStatus = "running"
    AdapterStatusCompleted AdapterStatus = "completed"
    AdapterStatusFailed    AdapterStatus = "failed"
)
```

**Integration with execution-worker:**
When a task has `payload.provider_type != "astra_agent"`, execution-worker calls `adapters.Registry.Get(providerType).DispatchGoal(...)` instead of running locally. It then polls `PollStatus` on a configurable interval until completion.

**New services:** `cmd/dtec-adapter` (for June 1); `cmd/agentforce-adapter`, `cmd/workday-adapter` (deferred to EOY)

**DB table (Olympus-owned, not Astra core):** `olympus_adapters` — adapter registry with health status.

---

### Gap 3: Webhook Ingest Service

**PRD Section:** 9 (Services); 11 (Schema — new migration)

**New service:** `cmd/webhook-ingest` (port 8092)

**Migration: `0019_webhook_sources.sql`**
```sql
CREATE TABLE IF NOT EXISTS webhook_sources (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id   TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    hmac_secret TEXT NOT NULL,         -- stored encrypted; Vault-injected at runtime
    schema_type TEXT NOT NULL DEFAULT 'generic',
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    org_id      UUID REFERENCES orgs(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_sources_org ON webhook_sources(org_id);
```

**Endpoint:**
```
POST /webhooks/{source_id}
```
Processing flow:
1. Look up `webhook_sources` by `source_id`
2. Validate HMAC-SHA256 signature (`X-Hub-Signature-256` header)
3. Assign `trigger_id = gen_random_uuid()`
4. `XADD astra:webhooks:raw * source_id {id} trigger_id {trigger_id} payload {body}`
5. Return `202 Accepted` with `{"trigger_id": "..."}`

**Redis stream:** `astra:webhooks:raw` — consumed by Olympus trigger classifier (not Astra-owned).

**Package:** `cmd/webhook-ingest/` — thin HTTP receiver. All business logic stays in consumers.

---

### Gap 4 + Gap 6: Goal Cascade Fields + Dependency Engine

**PRD Section:** 11 (Schema); 9 (Services — goal-service); 10 (gRPC)

**Migration: `0020_goal_dependencies.sql`**
```sql
ALTER TABLE goals
    ADD COLUMN IF NOT EXISTS cascade_id          UUID,
    ADD COLUMN IF NOT EXISTS depends_on_goal_ids UUID[] DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS completed_at        TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS source_agent_id     UUID REFERENCES agents(id);

CREATE INDEX IF NOT EXISTS idx_goals_cascade_id ON goals(cascade_id) WHERE cascade_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_goals_source_agent ON goals(source_agent_id) WHERE source_agent_id IS NOT NULL;
```

**New package:** `internal/goals/deps.go`

```go
// DependencyEngine watches for GoalCompleted events and unblocks waiting goals.
type DependencyEngine struct {
    store  GoalStore
    events EventBus
}

// OnGoalCompleted is called when a goal transitions to completed/failed.
// It finds goals whose depends_on_goal_ids are now all satisfied and activates them.
func (d *DependencyEngine) OnGoalCompleted(ctx context.Context, goalID uuid.UUID, status string) error
```

**goal-service changes:**
- `POST /goals` accepts `cascade_id` (optional), `depends_on_goal_ids` (optional `[]uuid`)
- On goal create: if `depends_on_goal_ids` is non-empty, goal starts in `blocked` state
- On goal completion: publish `GoalCompleted` to `astra:goals:completed`; call `DependencyEngine.OnGoalCompleted`

**gRPC GoalService proto additions:**
```protobuf
message CreateGoalRequest {
  // ... existing fields ...
  string cascade_id = 10;                     // optional
  repeated string depends_on_goal_ids = 11;   // optional
  string source_agent_id = 12;                // optional (agent-to-agent)
}

message GoalCompletedEvent {
  string goal_id       = 1;
  string cascade_id    = 2;
  string status        = 3;  // "completed" | "failed"
  string result_summary = 4;
  int64  completed_at  = 5;  // unix millis
}
```

**New task state:** `blocked` — goal created but waiting on `depends_on_goal_ids`.

---

### Gap 5: GoalCompleted Event Publication

**PRD Section:** 12 (Message & Event Protocols)

**Stream: `astra:goals:completed`**

```
Fields:
  goal_id         string   UUID of completed goal
  cascade_id      string   UUID of parent cascade (empty if standalone)
  org_id          string   Organization scope
  status          string   "completed" | "failed"
  result_summary  string   Short summary (truncated at 500 chars)
  completed_at    string   RFC3339 timestamp
```

**goal-service change:** When all tasks in a goal graph reach terminal state (completed or failed), goal-service calls `XADD astra:goals:completed * goal_id {id} status {s} ...`

This is additive — it does not affect existing `astra:events` publication.

---

### Gap 7: Agent-to-Agent Goal Posting (Complete)

**PRD Section:** 15 (SDK); 11 (Schema — covered in Gap 4's migration)

The `pkg/sdk` already has `GoalClient.CreateGoal()`. The missing pieces:

**SDK addition (`pkg/sdk/context.go`):**
```go
// PostGoal allows an agent to post a goal to another agent during task execution.
// Internally uses service-to-service gRPC with the agent's JWT as caller identity.
// Rate limited: max 10 goal posts per task execution.
func (ctx *AgentContext) PostGoal(targetAgentID, goalText string, priority int, opts ...PostGoalOption) (goalID string, err error)
```

**Service-to-service auth:**
- execution-worker mints a short-lived JWT (`sub=agent:<agent_id>`, `aud=goal-service`, TTL=60s) when an agent calls `PostGoal`
- goal-service validates this JWT and records `goals.source_agent_id = <agent_id>`
- Rate limit: `INCR agent:goalpost:rate:<agent_id>` in Redis, expire 60s, max 10

**Guard rails:**
- Max cascade depth: goal-service rejects `PostGoal` if the originating goal's cascade depth > 5
- Failed `PostGoal` returns an error to the agent; does not abort the parent task

---

### Gap 8 + Gap 14: Agent Tags, Metadata, Trust Score

**PRD Section:** 11 (Schema)

**Migration: `0021_agent_tags_trust.sql`**
```sql
ALTER TABLE agents
    ADD COLUMN IF NOT EXISTS tags         TEXT[]  DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS metadata     JSONB   DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS trust_score  FLOAT   DEFAULT 0.5
        CHECK (trust_score >= 0.0 AND trust_score <= 1.0);

CREATE INDEX IF NOT EXISTS idx_agents_tags     ON agents USING GIN(tags);
CREATE INDEX IF NOT EXISTS idx_agents_metadata ON agents USING GIN(metadata);

-- Trust event log (append-only; computation logic is Olympus-owned)
CREATE TABLE IF NOT EXISTS agent_trust_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id    UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    org_id      UUID NOT NULL REFERENCES orgs(id),
    event_type  TEXT NOT NULL,    -- "task_completed" | "task_failed" | "manual_adjust"
    delta       FLOAT NOT NULL,   -- signed score change
    new_score   FLOAT NOT NULL,
    metadata    JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_trust_events_agent ON agent_trust_events(agent_id, created_at DESC);
```

**API additions:**
```
PATCH  /agents/{id}           # existing — now accepts tags[], metadata{}, trust_score
GET    /agents?tag=transport  # existing — new ?tag= filter
POST   /agents/{id}/trust     # new: record trust event (internal/access-control only)
```

**Redis key:** `agent:trust:<agent_id>` — cached trust score (TTL 5 min), invalidated on `POST /agents/{id}/trust`.

**TrustScoreUpdated event:** Published to `astra:events` when trust score changes.

---

### Gap 9: Dual-Approval (Two-Person Rule)

**PRD Section:** 11 (Schema); 18 (Security, Policy, Governance)

**Migration: `0022_dual_approval.sql`**
```sql
ALTER TABLE approval_requests
    ADD COLUMN IF NOT EXISTS required_approvals INT     NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS approvals          JSONB   NOT NULL DEFAULT '[]';

-- approvals JSONB schema (array of objects):
-- [{"user_id": "uuid", "decision": "approved"|"rejected", "decided_at": "RFC3339", "note": "..."}]
```

**Logic change (`internal/rbac` or `cmd/access-control`):**
- `POST /approvals/{id}/decide` appends to `approvals[]`, does NOT overwrite `status`
- Status transitions to `approved` only when `count(decision=="approved") >= required_approvals`
- Status transitions to `rejected` on any single rejection (fail-fast)
- Dashboard shows "1 of 2 approved" for dual-approval items

**API shape:**
```
POST /approvals/{id}/decide
Body: {"decision": "approved"|"rejected", "note": "optional"}
Response: {
  "approval_id": "...",
  "status": "pending"|"approved"|"rejected",
  "approvals_received": 1,
  "approvals_required": 2,
  "can_execute": false
}
```

---

### Gap 10: Tool Definitions Registry

**PRD Section:** 11 (Schema); 14 (Tool Runtime & Sandboxing)

**Migration: `0023_tool_definitions.sql`**
```sql
CREATE TABLE IF NOT EXISTS tool_definitions (
    id           UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT    NOT NULL,
    version      TEXT    NOT NULL DEFAULT '1.0.0',
    risk_tier    TEXT    NOT NULL DEFAULT 'low'
                         CHECK (risk_tier IN ('low','medium','high','critical')),
    sandbox      TEXT    NOT NULL DEFAULT 'wasm'
                         CHECK (sandbox IN ('wasm','docker','firecracker','none')),
    description  TEXT,
    metadata     JSONB   DEFAULT '{}',
    org_id       UUID    REFERENCES orgs(id),   -- NULL = platform-global
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(name, version)
);

CREATE INDEX IF NOT EXISTS idx_tool_defs_org ON tool_definitions(org_id);
CREATE INDEX IF NOT EXISTS idx_tool_defs_risk ON tool_definitions(risk_tier);
```

**tool-runtime change:** Before executing any tool, look up `tool_definitions` by name. If `risk_tier IN ('high','critical')`, require an active approved `approval_request` for the task. Combine with `agents.trust_score` for dynamic approval bypass (trust > 0.8 + risk_tier=high → auto-approve).

**API:**
```
POST   /tools/definitions          # register tool
GET    /tools/definitions          # list (filterable by ?risk_tier=, ?org_id=)
GET    /tools/definitions/{id}
PATCH  /tools/definitions/{id}
DELETE /tools/definitions/{id}
```

---

### Gap 11: Chat Session External Message Injection

**PRD Section:** 9 (Services — api-gateway chat routes)

**No schema change required.** `chat_messages` table already has a `role` column.

**New endpoint:**
```
POST /chat/sessions/{session_id}/messages
Body: {"role": "system"|"assistant", "content": "...", "source_service": "cascade-engine"}
Authorization: Service-to-service JWT (aud=api-gateway)
Response: {"message_id": "...", "session_id": "..."}
```

**Behavior:** Message is inserted into `chat_messages` directly (no LLM call). If the session has an active WebSocket subscriber, the message is pushed over the socket using the existing `protocol.go` streaming mechanism.

**Use case:** Olympus cascade engine posts status updates into a chat session so operators see live cascade progress without polling.

---

### Gap 12: Approval REST API (Programmatic Decide)

**PRD Section:** 9 (Services — access-control)

Currently approvals can only be decided via the dashboard UI. This adds a REST endpoint callable by external services (Slack worker, cascade engine).

**Endpoint (already partly exists; extend it):**
```
POST   /approvals/{id}/decide
GET    /approvals?status=pending&org_id={org_id}
GET    /approvals/{id}
```

The `POST /approvals/{id}/decide` endpoint:
- Validates service JWT or user JWT (both accepted)
- If service JWT: records `decided_by = service:<service_name>`
- Integrates with dual-approval logic (Gap 9)
- Publishes `ApprovalDecided` event to `astra:events`

**Idempotency:** Calling decide twice with the same decision is a no-op (returns current state).

---

### Gap 13: Goal Priority in Scheduler + Agent Concurrency Limits

**PRD Section:** 8 (Scheduler); 9 (Services — goal-service)

**No schema migration required** — `goals.priority` column already exists.

**Scheduler change (`internal/scheduler`):**
- When pushing tasks to `astra:tasks:shard:<n>`, include `priority` field in stream message
- Consumer group `XREADGROUP` already reads in arrival order; add priority-aware reorder buffer: tasks with priority > 5 (scale 1–10) are elevated to front of local processing queue

**Goal admission control (`internal/goal-service`):**
- `agents` table already has `config JSONB`; read `config.max_concurrent_goals` (default: unlimited)
- On `POST /goals`: count `SELECT COUNT(*) FROM goals WHERE agent_id=$1 AND status NOT IN ('completed','failed')`
- If count >= `max_concurrent_goals`: return `429 Too Many Requests` with `{"error": "agent at concurrency limit", "active_goals": N, "limit": M}`
- Cache active goal count in Redis: `agent:active_goals:<agent_id>` (TTL 30s, invalidated on goal terminal transition)

---

## Technical Context (Brownfield)

### Package locations for new code
```
internal/adapters/     — adapter framework (new)
internal/goals/deps.go — dependency engine (new file in existing package)
internal/slack/        — Slack API client (new)
cmd/slack-adapter/     — Slack Events API receiver (new service, port 8091)
cmd/slack-worker/      — Slack stream consumer (new service, no port)
cmd/webhook-ingest/    — Webhook receiver (new service, port 8092)
cmd/dtec-adapter/      — D.TEC adapter (new service)
```

### Migration sequence
```
0019_webhook_sources.sql          — webhook_sources table
0020_goal_dependencies.sql        — goals schema extension
0021_agent_tags_trust.sql         — agents tags/trust + agent_trust_events
0022_dual_approval.sql            — approval_requests extension
0023_tool_definitions.sql         — tool_definitions table
```

### Redis keys/streams added
```
astra:goals:completed              — GoalCompleted events (new stream)
astra:slack:incoming               — raw Slack events (new stream)
astra:slack:outgoing               — outbound Slack messages (new stream)
astra:webhooks:raw                 — raw webhook payloads (new stream)
agent:trust:<id>                   — cached trust score (TTL 5m)
agent:goalpost:rate:<id>           — agent-to-agent rate limit counter (TTL 60s)
agent:active_goals:<id>            — active goal count cache (TTL 30s)
```

---

## Ontology (Key Entities)

| Entity | Type | Fields | Relationships |
|--------|------|--------|---------------|
| Gap | core domain | id, category, status, migration_needed | belongs to PRD section |
| PRD.md | external system | sections 1–27 | receives additive updates |
| olympus-implementation-status.md | supporting doc | gap table, done/remaining | tracks Gap status |
| Migration | supporting | number, filename, DDL | belongs to Gap |

## Ontology Convergence
| Round | Entities | New | Changed | Stable | Stability |
|-------|----------|-----|---------|--------|-----------|
| 1 | 3 | 3 | - | - | N/A |
| 2 | 4 | 1 | 0 | 3 | 75% |
| 3 | 4 | 0 | 0 | 4 | 100% |
| 4 | 4 | 0 | 0 | 4 | 100% |

---

## Interview Transcript

<details>
<summary>Full Q&A (4 rounds)</summary>

### Round 1
**Q:** When you say 'understand what needs to be implemented' — what is the desired output?
**A:** Gap analysis + PRD updates
**Ambiguity:** 39% (Goal: 0.70, Constraints: 0.45, Criteria: 0.55, Context: 0.75)

### Round 2
**Q:** Should the PRD update cover all remaining gaps, or focus on a subset?
**A:** All remaining gaps (complete)
**Ambiguity:** 31% (Goal: 0.80, Constraints: 0.60, Criteria: 0.60, Context: 0.75)

### Round 3
**Q:** What level of detail should the PRD additions have?
**A:** Full fidelity (match existing PRD style: DDL, gRPC, REST, Redis schemas)
**Ambiguity:** 21% (Goal: 0.85, Constraints: 0.70, Criteria: 0.80, Context: 0.75)

### Round 4
**Q:** The enhancement doc says Phases 9/11 are 'NOT complete', but they ARE implemented — should correcting this be part of the deliverable?
**A:** PRD + new implementation status doc
**Ambiguity:** 14% (Goal: 0.90, Constraints: 0.85, Criteria: 0.85, Context: 0.80)

</details>
