# Phase 9 — Agent Profile & Context Management: Execution Memo

**Owner:** Architect → Tech Lead
**Duration:** 3-4 weeks
**Depends on:** Phases 1-4 (kernel, goal-service, planner, agent-service all operational)

---

## Work Packages

### WP1: Database Migration

**File:** `migrations/0013_agent_profile_and_documents.sql`
**Owner:** DB Architect → Go Engineer
**Dependencies:** None (foundation)
**Effort:** 0.5 days

- ALTER agents table: add `system_prompt TEXT DEFAULT ''`
- CREATE `agent_documents` table (id, agent_id, goal_id, doc_type, name, content, uri, metadata, priority, created_at, updated_at)
- CHECK constraint on doc_type: `rule`, `skill`, `context_doc`, `reference`
- Indexes: agent_id, goal_id, (agent_id, doc_type)
- Trigger: `update_updated_at` on agent_documents
- All DDL idempotent (`IF NOT EXISTS`, `IF EXISTS`)

---

### WP2: Agent Document Store

**File:** `internal/agentdocs/store.go`
**Owner:** Go Engineer
**Dependencies:** WP1 (migration applied)
**Effort:** 2-3 days

- `Store` struct with `*sql.DB` and `*redis.Client`
- `CreateDocument(ctx, doc) (UUID, error)` — insert into agent_documents, invalidate Redis cache
- `ListDocuments(ctx, agentID, opts) ([]Document, error)` — filter by doc_type, goal_id; cache-aside from `agent:docs:{agent_id}`
- `DeleteDocument(ctx, docID) error` — delete from DB, invalidate cache
- `GetProfile(ctx, agentID) (*AgentProfile, error)` — read system_prompt + config from agents table; cache-aside from `agent:profile:{agent_id}`
- `UpdateProfile(ctx, agentID, profile) error` — update agents row, invalidate Redis cache
- Redis cache keys: `agent:profile:{id}` (Hash, 5min TTL), `agent:docs:{id}` (JSON string, 5min TTL)
- Cache invalidation on every write (delete key, next read repopulates)
- Large doc support: if `content` is nil and `uri` is set, content lives in MinIO/S3

---

### WP3: Context Assembly

**File:** `internal/agentdocs/context.go`
**Owner:** Go Engineer
**Dependencies:** WP2
**Effort:** 1-2 days

- `AgentContext` struct: `SystemPrompt string`, `Rules []Document`, `Skills []Document`, `ContextDocs []Document`
- `AssembleContext(ctx, agentID, goalID *UUID) (*AgentContext, error)`:
  1. Fetch agent profile (WP2)
  2. Fetch global agent documents (goal_id IS NULL) by type
  3. If goalID set, fetch goal-scoped documents and merge
  4. Sort rules by priority (lower = higher precedence)
  5. Return assembled context
- `SerializeContext(ac *AgentContext) (json.RawMessage, error)` for embedding in task payloads

---

### WP4: API Gateway Endpoints

**Files:** `cmd/api-gateway/main.go` (route registration), handler functions
**Owner:** Go Engineer
**Dependencies:** WP2, WP3
**Effort:** 2-3 days

New endpoints (all behind JWT + access-control middleware):

| Method | Path | Handler | Notes |
|---|---|---|---|
| PATCH | `/agents/{id}` | `handleUpdateAgent` | Updates system_prompt and/or config |
| GET | `/agents/{id}/profile` | `handleGetProfile` | Served from Redis cache |
| POST | `/agents/{id}/documents` | `handleCreateDocument` | Body: {doc_type, name, content/uri, metadata, priority, goal_id?} |
| GET | `/agents/{id}/documents` | `handleListDocuments` | Query params: ?doc_type=rule&goal_id=... |
| DELETE | `/agents/{id}/documents/{doc_id}` | `handleDeleteDocument` | |

All read endpoints must serve from cache (10ms SLA).

---

### WP5: Goal-Service Context Integration

**Files:** `cmd/goal-service/main.go`
**Owner:** Go Engineer
**Dependencies:** WP2, WP3
**Effort:** 1-2 days

- Update `POST /goals` request body to accept optional `documents` array
- On goal creation: persist each inline document as goal-scoped (`goal_id` set) via `agentdocs.Store`
- Before calling planner: call `agentdocs.AssembleContext(agentID, &goalID)` to build full context
- Pass serialized `agent_context` to planner-service in the plan request body

---

### WP6: Planner Context Propagation

**Files:** `cmd/planner-service/main.go`, `internal/planner/planner.go`
**Owner:** Go Engineer
**Dependencies:** WP5
**Effort:** 1 day

- Accept `agent_context` field in plan request
- When generating task DAG, embed `agent_context` JSON into each task's `payload` under the key `agent_context`
- No changes to planning logic itself; context is pass-through into task payloads

---

### WP7: Execution Worker Context Usage

**Files:** `cmd/execution-worker/main.go`
**Owner:** Go Engineer
**Dependencies:** WP6
**Effort:** 1 day

- When building the LLM prompt for a task, extract `agent_context` from `task.Payload`
- Prepend `system_prompt` as the system message
- Append rules, skills, and context docs in priority order as context in the prompt
- No changes to task lifecycle or worker registration

---

### WP8: Tests & Validation

**Files:** `internal/agentdocs/*_test.go`, `tests/integration/`, `scripts/validate.sh`
**Owner:** QA Engineer
**Dependencies:** WP2-WP7
**Effort:** 2-3 days

- Unit tests: store CRUD, cache hit/miss/invalidation, context assembly with/without goal docs, priority sorting
- Integration test: full flow — create agent with profile → attach documents → submit goal with inline docs → verify planner receives context → verify worker task payload contains agent_context
- Update `scripts/validate.sh` with Phase 9 structural and functional checks

---

## Dependency Graph

```
WP1 (migration)
 └──> WP2 (store)
       ├──> WP3 (context assembly)
       │     ├──> WP4 (API gateway)
       │     ├──> WP5 (goal-service)
       │     │     └──> WP6 (planner propagation)
       │     │           └──> WP7 (worker usage)
       │     └──> WP8 (tests — starts after WP3, completes after WP7)
       └──> WP4 (API gateway — also needs WP2 directly)
```

## Implementation Order

| Order | Work Package | Estimated Effort | Depends On |
|---|---|---|---|
| 1 | WP1: Database Migration | 0.5 days | — |
| 2 | WP2: Agent Document Store | 2-3 days | WP1 |
| 3 | WP3: Context Assembly | 1-2 days | WP2 |
| 4 | WP4: API Gateway Endpoints | 2-3 days | WP2, WP3 |
| 5 | WP5: Goal-Service Integration | 1-2 days | WP2, WP3 |
| 6 | WP6: Planner Propagation | 1 day | WP5 |
| 7 | WP7: Worker Context Usage | 1 day | WP6 |
| 8 | WP8: Tests & Validation | 2-3 days | WP2-WP7 |

**Note:** WP4 and WP5 can be parallelized once WP3 is complete.

**Total estimated effort:** 11-16 engineer-days (3-4 weeks with one engineer, ~2 weeks with two).
