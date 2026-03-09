# Phase/Build History, Token/LLM Usage, and Audit Logs — Design Note

This document describes how Astra supports (1) phase/build history with file and vector storage, (2) per-request token/LLM usage and metrics visible to the user, and (3) audit logs that make usage and history auditable over time. It aligns with the PRD (docs/PRD.md), the existing events table, Redis Streams, and the 10 ms API read constraint.

---

## 1. Scope and Definitions

- **Phase (build phase)** — A discrete execution run that produces build/execution history: e.g. one goal execution (goal → plan → tasks). Each phase has a unique identifier, a timeline of actions (tasks, artifacts, summaries), and optional human-readable output.
- **Token/LLM usage** — Per LLM request: model name, tokens in/out, latency, and cost (if available). Captured wherever the LLM is invoked (llm-router or callers) and exposed to the user in the response and persisted for audit.
- **Audit logs** — An append-only, queryable record of phase lifecycle events and LLM usage events so that usage and history are auditable over time.

**Constraints respected:** API response time ≤ 10 ms on hot paths; heavy work (vector writes, file I/O, bulk DB writes) is async or background. Existing stack: Postgres (with pgvector), Redis, existing `events` table and messaging (Redis Streams).

---

## 2. Phase/Build History

### 2.1 What Is Stored

For each phase (e.g. one goal run):

- **Identity:** `phase_id` (UUID), `goal_id` (FK to goals), `agent_id`, optional label/name.
- **Lifecycle:** `started_at`, `ended_at`, `status` (e.g. running, completed, failed).
- **Summary:** Short text summary of what was done (e.g. “Planned 5 tasks; completed 4; 1 failed”).
- **Timeline:** Ordered list of significant actions: task created/completed/failed, artifacts produced, key decisions (e.g. plan generated). Stored as structured data (JSONB) and, for human consumption, as a single file.

### 2.2 Where It Is Stored

**Database (Postgres)**

- **`phase_runs`** — One row per phase. Columns: `id` (UUID PK), `goal_id` (UUID, nullable, FK goals), `agent_id` (UUID, FK agents), `name`/`label` (TEXT, optional), `status` (TEXT), `started_at`, `ended_at` (nullable), `summary` (TEXT, nullable), `timeline` (JSONB, optional), `log_file_path` (TEXT, nullable), `created_at`, `updated_at`. Purpose: durable record of each phase, queryable by agent, goal, time range.
- **`phase_summaries`** (pgvector) — For semantic search over “what was done.” Columns: `id` (UUID PK), `phase_id` (UUID FK phase_runs), `content` (TEXT — summary or chunk of phase narrative), `embedding` (VECTOR(1536)), `created_at`. Same embedding dimension and pattern as `memories` (see migrations/0003_memories.sql). Purpose: “find phases where we did X” via similarity search. Writes to this table are **async** (e.g. from a consumer or background job after phase completion) so the API never waits on vector indexing.

**File (human-readable log per phase)**

- **Path convention:** `{phase_history_base}/phases/{phase_id}.md` or `{phase_id}.json`. Base path is configurable (e.g. env `ASTRA_PHASE_HISTORY_DIR`); default suggestion: `./data/phase-history` or equivalent under deployment root.
- **Content:** One file per phase. Markdown: title (phase_id, goal_id, agent_id), started/ended times, status, summary, then a bullet or table of timeline entries (task id, type, outcome, timestamp). Optional: same data as JSON for tooling. File is written **asynchronously** (e.g. by a consumer that handles `PhaseCompleted` or by the service that closes the phase), so the request path is not blocked.

### 2.3 How It Fits Existing Components

- **goal-service / planner-service:** When a goal is accepted and planning starts, the component that owns “run” lifecycle creates a `phase_runs` row (status `running`) and emits a `PhaseStarted` event (see §4). When the run finishes (all tasks terminal or run aborted), it updates the row (status, `ended_at`, `summary`, `timeline`), sets `log_file_path`, and emits `PhaseCompleted`. Summary and optional timeline are then handed off to an async writer for file and to an embedding pipeline for `phase_summaries`.
- **task-service / events:** Task lifecycle events (e.g. TaskCompleted, TaskFailed) already exist; the phase “timeline” can be built from these or from a dedicated phase-scoped writer that subscribes to task events for that goal/graph.
- **memory-service / embedding pipeline:** Reuse the same embedding pipeline used for `memories` to compute embeddings for phase summary text (and optionally for timeline chunks). A small async job or stream consumer writes into `phase_summaries` after phase completion so that semantic search is available without blocking the API.

---

## 3. Token/LLM Usage and Metrics

### 3.1 What the User Sees

With every request that triggers an LLM call, the user receives:

- **Token/LLM usage:** tokens in, tokens out, which model was used.
- **Metrics:** latency of the LLM call, and cost if available (e.g. from a pricing table keyed by model).

This can be returned in the **response envelope** (e.g. gRPC metadata, or a wrapper message field such as `usage` on the response). The important point: the serving path does **not** wait on Postgres or file I/O to return this; usage is collected in-memory in the request path and attached to the response. Persistence is asynchronous.

### 3.2 Where Usage Is Captured

- **Capture point:** In the component that performs the LLM call. Typically **llm-router**: when it returns a response to a caller (planner-service, execution-worker, memory-service, etc.), it already has model, token counts, and latency. That component (or a thin middleware around it) records a usage record (in-memory for the current request) and returns usage in the response. If cost is computed (e.g. in llm-router from model + token counts), it is included; otherwise the field is null.
- **Caller responsibility:** Callers (e.g. api-gateway, agent-service) that aggregate multiple LLM calls for one “user request” may attach the latest or aggregated usage to the outer response. Single-request flows attach the single usage record.

### 3.3 Where Usage Is Stored (Persistent)

- **Structured table:** **`llm_usage`** — One row per LLM request. Columns: `id` (UUID PK), `request_id` or `correlation_id` (TEXT, nullable, for tying to external request), `agent_id` (UUID, nullable), `task_id` (UUID, nullable), `model` (TEXT), `tokens_in` (INT), `tokens_out` (INT), `latency_ms` (INT, nullable), `cost_dollars` (NUMERIC, nullable), `created_at`. Purpose: analytics, dashboards, per-agent/per-task usage, and audit. Writes to this table must **not** be on the hot path: they go through an async path (see below).
- **Audit trail:** Each usage event is also appended to the **`events`** table (or equivalent append-only audit log) with event_type `LLMUsage` and payload containing the same fields (see §4). This gives an immutable, time-ordered audit log.

**Async write path (keeps API under 10 ms):**

1. In the request path: capture usage in memory → attach to response → return.
2. Publish a message to a Redis stream (e.g. `astra:usage` or a dedicated `astra:audit:usage`) with the usage payload.
3. A dedicated consumer (same process or separate worker) reads from the stream and: (a) inserts into `llm_usage`, (b) optionally inserts an event into `events` with type `LLMUsage` and same payload. No synchronous DB write in the LLM response path.

### 3.4 Fit with Existing Components

- **llm-router:** Records usage per call, returns it in the response, and publishes to the usage stream. No direct Postgres write in the handler.
- **Memcached:** Cached LLM responses remain unchanged; usage is recorded only when a call is actually made (cache hit does not generate a usage row).
- **PRD §22 (Cost Management), §17 (Observability):** Aligns with `astra_llm_token_usage_total`, `astra_llm_cost_dollars` and cost-tracking; the `llm_usage` table is the durable source for those metrics and for per-request audit.

---

## 4. Audit Logs

### 4.1 What Is Auditable

- **Phase/build history:** Phase lifecycle (PhaseStarted, PhaseCompleted, PhaseFailed) and, if desired, a condensed PhaseSummary event. Payload can reference `phase_id`, `goal_id`, `agent_id`, status, timestamps, and optionally a short summary or `log_file_path`.
- **Per-request usage:** Every LLM usage event (model, tokens in/out, latency, cost, agent_id, task_id, request_id). Stored as event type `LLMUsage` with payload containing these fields.

Together, these allow auditors to answer “what was built in this phase?” and “what LLM usage occurred in this time window or for this agent/task?”

### 4.2 Where Audit Logs Live

- **Primary:** **`events` table** (existing). Add new event types and use the existing `(event_type, actor_id, payload, created_at)` shape. New types: `PhaseStarted`, `PhaseCompleted`, `PhaseFailed`, `PhaseSummary`, `LLMUsage`. `actor_id` can be the phase_id or agent_id as appropriate; payload is JSONB with event-specific fields. All phase and usage events are appended here so there is a single, append-only audit trail alongside existing task/agent events.
- **Optional:** If product or compliance later requires a separate audit schema (e.g. `audit.events` or a dedicated `audit_log` table), a consumer can duplicate selected event types from the `events` table or from the same streams that feed `events`. This design does not require a second store for MVP; the existing `events` table is the canonical audit log.

### 4.3 Structure of Audit Entries

- **Phase events:**  
  `event_type` ∈ {`PhaseStarted`, `PhaseCompleted`, `PhaseFailed`, `PhaseSummary`}.  
  `payload`: e.g. `{ "phase_id": "...", "goal_id": "...", "agent_id": "...", "status": "...", "started_at": "...", "ended_at": "...", "summary": "...", "log_file_path": "..." }`.
- **Usage events:**  
  `event_type` = `LLMUsage`.  
  `payload`: e.g. `{ "request_id": "...", "agent_id": "...", "task_id": "...", "model": "...", "tokens_in": 0, "tokens_out": 0, "latency_ms": 0, "cost_dollars": null }`.

Indexes on `events(actor_id)` and `events(event_type)` (already present per migrations/0007_indexes.sql) support filtering by agent and by event type. An index on `events(created_at)` (or composite with event_type) improves time-range audit queries.

---

## 5. Schema and Migration Ideas

The following are concise schema/migration ideas. Implementation details (exact names, constraints) should be aligned with the DB Architect and existing migrations (e.g. 0001_initial_schema.sql, 0006_events.sql, 0003_memories.sql).

### 5.1 New Tables

1. **`phase_runs`**  
   - `id` UUID PRIMARY KEY, `goal_id` UUID REFERENCES goals(id) ON DELETE SET NULL, `agent_id` UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE, `name` TEXT, `status` TEXT NOT NULL, `started_at` TIMESTAMPTZ NOT NULL, `ended_at` TIMESTAMPTZ, `summary` TEXT, `timeline` JSONB, `log_file_path` TEXT, `created_at`, `updated_at`.  
   - Constraint: `status` IN ('running', 'completed', 'failed', 'cancelled').

2. **`phase_summaries`**  
   - `id` UUID PRIMARY KEY, `phase_id` UUID NOT NULL REFERENCES phase_runs(id) ON DELETE CASCADE, `content` TEXT NOT NULL, `embedding` VECTOR(1536), `created_at` TIMESTAMPTZ NOT NULL DEFAULT now().  
   - Index: `phase_summaries(phase_id)`.  
   - Index: ivfflat (or hnsw) on `embedding` for similarity search (same pattern as `memories`).

3. **`llm_usage`**  
   - `id` UUID PRIMARY KEY, `request_id` TEXT, `agent_id` UUID REFERENCES agents(id) ON DELETE SET NULL, `task_id` UUID REFERENCES tasks(id) ON DELETE SET NULL, `model` TEXT NOT NULL, `tokens_in` INT NOT NULL, `tokens_out` INT NOT NULL, `latency_ms` INT, `cost_dollars` NUMERIC(12,6), `created_at` TIMESTAMPTZ NOT NULL DEFAULT now().  
   - Indexes: `(agent_id, created_at)`, `(task_id, created_at)`, `(created_at)` for analytics and audit queries.

### 5.2 Events Table

- No new columns. Add new `event_type` values: `PhaseStarted`, `PhaseCompleted`, `PhaseFailed`, `PhaseSummary`, `LLMUsage`. Optional: add index on `(created_at)` or `(event_type, created_at)` if audit queries by time range are heavy.

### 5.3 Redis Stream (optional but recommended)

- **`astra:usage`** (or `astra:audit:usage`) — Fields: `request_id`, `agent_id`, `task_id`, `model`, `tokens_in`, `tokens_out`, `latency_ms`, `cost_dollars`, `timestamp`. Consumer: persists to `llm_usage` and appends to `events` with type `LLMUsage`. Enables async write and batching.

### 5.4 Migration Sketch (single migration file idea)

- `0009_phase_history_and_usage.sql`:  
  - CREATE TABLE phase_runs (...);  
  - CREATE TABLE phase_summaries (...);  
  - CREATE INDEX on phase_summaries(phase_id);  
  - CREATE INDEX on phase_summaries USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);  
  - CREATE TABLE llm_usage (...);  
  - CREATE INDEX on llm_usage(agent_id, created_at);  
  - CREATE INDEX on llm_usage(task_id, created_at);  
  - CREATE INDEX on llm_usage(created_at);  
  - CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);  
  All idempotent (IF NOT EXISTS, etc.).

---

## 6. Implementation Order

Implement in the following order so that phase history, usage, and audit are usable incrementally.

1. **LLM usage capture and response metadata**  
   - In llm-router (or the layer that calls the LLM): capture model, tokens in/out, latency, optional cost per request.  
   - Attach usage to the response (gRPC metadata or response message field).  
   - No persistence yet; user sees usage on each response.

2. **Async usage persistence and audit**  
   - Add Redis stream `astra:usage` and publish usage from llm-router after each call.  
   - Add `llm_usage` table and a consumer that: reads from stream, inserts into `llm_usage`, and appends to `events` with type `LLMUsage`.  
   - Result: usage is visible in API and persisted for analytics and audit.

3. **Phase run table and lifecycle events**  
   - Add `phase_runs` table.  
   - In goal/planner path: create row on phase start, update on phase end with summary and timeline (from task events or internal state).  
   - Emit `PhaseStarted`, `PhaseCompleted`/`PhaseFailed` (and optionally `PhaseSummary`) into `events` (and optionally to `astra:events` stream for real-time subscribers).  
   - Result: phase lifecycle is in DB and in the audit log.

4. **Phase history file (human-readable)**  
   - Configure `ASTRA_PHASE_HISTORY_DIR`.  
   - When a phase completes, enqueue a job or stream message to write `{phase_id}.md` (and optionally `.json`) under `{base}/phases/`.  
   - Consumer or background writer writes file and updates `phase_runs.log_file_path`.  
   - Result: each phase has a human-readable log file.

5. **Phase summaries and semantic search (pgvector)**  
   - Add `phase_summaries` table.  
   - After phase completion, async job or consumer: build summary text (from `phase_runs.summary` and optionally `timeline`), run through existing embedding pipeline, insert into `phase_summaries`.  
   - Expose a read API (e.g. on memory-service or a small phase-history service) that runs similarity search on `phase_summaries` (and optionally returns `phase_runs` rows).  
   - Result: users can query “phases where we did X” semantically; all heavy work is off the hot path.

---

## 7. Development-phase history (file + optional vector)

In addition to **runtime** phase runs (stored in `phase_runs`, `phase_summaries`, and `events`), we maintain a **development-phase history** for “what was built” in each implementation phase:

- **Location:** `docs/phase-history/` — one file per phase (e.g. `phase-0.md`, `phase-1.md`) with goals, what was implemented, and decisions.
- **README:** `docs/phase-history/README.md` explains the convention and how it differs from runtime `phase_runs`.
- **Optional vector ingestion:** Content from these files can be embedded and stored in `phase_summaries` (or a dedicated dev-history index) so agents can answer “what did we do in Phase X?” or “when did we add feature Y?” via semantic search. Ingestion can be a one-off or CI step; not required for MVP.

User-facing **token/LLM usage** is shown on every request (response metadata) and persisted asynchronously to `llm_usage` and `events` (type `LLMUsage`) for audit and metrics.

---

## 8. References

- **PRD:** docs/PRD.md — §11 Database Schema, §12 Message & Event Protocols, §17 Observability, §22 Cost Management & LLM Routing.  
- **Migrations:** migrations/0001_initial_schema.sql (agents, goals, tasks), 0003_memories.sql (pgvector pattern), 0006_events.sql (events table), 0007_indexes.sql, 0009_phase_history_and_usage.sql.  
- **Messaging:** .cursor/skills/messaging-reference/SKILL.md — Redis Streams and consumer patterns.  
- **Performance:** Hot-path reads from cache only; event and usage writes async via streams and consumers.
- **Development phase history:** docs/phase-history/README.md, docs/phase-history/phase-0.md.
