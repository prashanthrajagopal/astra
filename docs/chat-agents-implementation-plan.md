# Chat Agents — Implementation Plan

Derived from **docs/chat-agents-design.md**. Ordered by dependency; each phase blocks the next unless noted.

---

## Phase 1: Database schema (DB Architect)

**Owner:** DB Architect  
**Deliverables:** Idempotent SQL migrations; no drops without explicit approval.

| # | Task | Deliverable |
|---|------|-------------|
| 1.1 | Add `chat_sessions` table: `id` (UUID), `user_id` (TEXT), `agent_id` (UUID FK → agents), `title` (TEXT), `status` (TEXT), `created_at`, `updated_at`, `expires_at` (TIMESTAMPTZ nullable). Indexes: `(user_id)`, `(agent_id)`, `(user_id, updated_at)`. | Migration file `migrations/00XX_chat_sessions.sql` |
| 1.2 | Add `chat_messages` table: `id` (UUID), `session_id` (UUID FK → chat_sessions), `role` (TEXT: user \| assistant \| system), `content` (TEXT), `tool_calls` (JSONB), `tool_results` (JSONB), `created_at`. Index: `(session_id, created_at)`. | Migration file `migrations/00XX_chat_messages.sql` |
| 1.3 | Add `agents.chat_capable` BOOLEAN DEFAULT false (Option A per design). | Same or separate migration |

**Exit criteria:** Migrations run cleanly; schema matches design §8.1–8.2.

---

## Phase 2: Session REST API (Tech Lead → Go Engineer)

**Owner:** Tech Lead (delegates to Go Engineer)  
**Depends on:** Phase 1  
**Deliverables:** REST handlers in api-gateway; JWT + OPA; cache for list/get.

| # | Task | Deliverable |
|---|------|-------------|
| 2.1 | Implement `POST /chat/sessions`: body `agent_id`, optional `title`. Validate JWT (identity), OPA `chat:use` for resource `agent:<id>`. Insert into `chat_sessions`, return `session_id`, `agent_id`, WebSocket path (e.g. `/chat/ws?session_id=...`), `created_at`. | Handler + route in api-gateway |
| 2.2 | Implement `GET /chat/sessions`: list sessions for JWT subject; optional `?agent_id=`. Serve from Redis/cache where possible (≤10ms). | Handler + route + cache layer |
| 2.3 | Implement `GET /chat/sessions/:id`: get one session; verify `session.user_id == subject`; return session details. Cache-friendly. | Handler + route |

**Exit criteria:** Session create/list/get work with JWT and OPA; list/get meet 10ms target from cache when populated.

---

## Phase 3: WebSocket upgrade and auth (Tech Lead → Go Engineer)

**Owner:** Tech Lead (delegates to Go Engineer)  
**Depends on:** Phase 2  
**Deliverables:** WebSocket route and auth flow; session frame on success.

| # | Task | Deliverable |
|---|------|-------------|
| 3.1 | Add route `GET /chat/ws` in api-gateway; perform HTTP upgrade to WebSocket. | WebSocket upgrade handler |
| 3.2 | Extract JWT from query param (`token=`) or from first JSON frame `{ "type": "auth", "token": "<JWT>" }`. Validate via identity service. | Auth extraction + validation |
| 3.3 | Load session by `session_id` (query param); verify `session.user_id == JWT subject` and agent is `chat_capable`; OPA check for `chat:connect`. On success send `session` frame `{ type, session_id, agent_id }`; on failure close with 4xx. | Session validation + session frame |

**Exit criteria:** Client can connect with token (query or first message), receive `session` frame; invalid token or wrong session → connection closed with 4xx.

---

## Phase 4: Message protocol (Tech Lead → Go Engineer)

**Owner:** Tech Lead (delegates to Go Engineer)  
**Depends on:** Phase 3  
**Deliverables:** Client frame parsing; server frame types; max message length.

| # | Task | Deliverable |
|---|------|-------------|
| 4.1 | Parse client frames: `auth`, `message` (content, optional id), `ping`. Reject unknown types; enforce max client message length (e.g. 64 KiB); respond with `error` frame if exceeded. | Client frame parser + validation |
| 4.2 | Implement server frame writers: `chunk`, `message_start`, `message_end`, `tool_call`, `tool_result`, `done`, `error`, `pong`, `session`. JSON text frames; consistent `type` and payload per design §4.3. | Server frame types + helpers |

**Exit criteria:** Client can send `message` and `ping`; server can send all frame types; oversized message rejected with error frame.

---

## Phase 5: Chat conversation loop (Tech Lead → Go Engineer)

**Owner:** Tech Lead (delegates to Go Engineer)  
**Depends on:** Phase 4; llm-router streaming support (add or use streaming Complete RPC)  
**Deliverables:** Per-message loop: load history → prompt → LLM stream → chunks + message_end + done.

| # | Task | Deliverable |
|---|------|-------------|
| 5.1 | On each user `message` frame: load session and recent messages (from Redis/Postgres); build prompt. | Context loading + prompt builder |
| 5.2 | Call llm-router streaming API (e.g. CompleteStream); stream response as `chunk` frames; on completion send `message_end` (with optional usage) and `done`. Persist user and assistant messages asynchronously (non-blocking). | LLM streaming integration + chunk/message_end/done emission |
| 5.3 | Wire message loop to WebSocket handler: one message-in-flight per session (or document concurrency policy). | Chat handler orchestration |

**Exit criteria:** User sends message → receives streamed chunks → message_end → done; messages persisted async.

---

## Phase 6: Tool invocation from chat (Tech Lead → Go Engineer)

**Owner:** Tech Lead (delegates to Go Engineer)  
**Depends on:** Phase 5; tool-runtime (and optionally worker-manager)  
**Deliverables:** tool_call emission; tool-runtime call; tool_result emission; approval handling.

| # | Task | Deliverable |
|---|------|-------------|
| 6.1 | When LLM/orchestrator decides to call a tool: emit `tool_call` frame(s); call tool-runtime (same sandbox/policy as goal-based flow); on result emit `tool_result` frame(s). Optionally continue LLM stream after tool result. | tool_call → tool-runtime → tool_result |
| 6.2 | Respect `approval_required` from access-control; stream `pending_approval` or error to client when action requires human-in-the-loop (S6). | Approval gate integration |

**Exit criteria:** Tool calls from chat invoke tool-runtime; client sees tool_call and tool_result; approval-required flows stream pending_approval/error.

---

## Phase 7: Memory and context (Tech Lead → Go Engineer) — Optional

**Owner:** Tech Lead (delegates to Go Engineer)  
**Depends on:** Phase 5; memory-service  
**Deliverables:** Optional recall when building prompt; token limits respected.

| # | Task | Deliverable |
|---|------|-------------|
| 7.1 | When building prompt, optionally call memory-service for recent context (e.g. last N messages or semantic recall); append to system/context; stay within token limits. | memory-service integration in prompt builder |

**Exit criteria:** Prompt can include memory-service context when configured; no token overflow.

---

## Phase 8: Rate limits and caps (Tech Lead → Go Engineer) — Optional

**Owner:** Tech Lead (delegates to Go Engineer)  
**Depends on:** Phase 5  
**Deliverables:** Per-user/session rate limit; optional token cap; 429/error frame.

| # | Task | Deliverable |
|---|------|-------------|
| 8.1 | Per-user or per-session rate limit (e.g. N messages per minute); return 429 or error frame when exceeded. | Rate limiter + error response |
| 8.2 | Optional: token limit per session; reject with error frame when exceeded. | Token cap check |

**Exit criteria:** Rate limit and optional token cap enforced; client receives clear error frames.

---

## Phase 9: Observability (Tech Lead → Go Engineer / DevOps)

**Owner:** Tech Lead (Go Engineer); DevOps for deploy/runbook notes  
**Depends on:** Phases 2–6  
**Deliverables:** Logs; metrics; tracing.

| # | Task | Deliverable |
|---|------|-------------|
| 9.1 | Log session create, WebSocket connect/disconnect, message counts (structured logging). | Log events |
| 9.2 | Metrics: e.g. `chat_sessions_active`, `chat_messages_total`, `chat_tool_calls_total`. | Metrics (Prometheus-style) |
| 9.3 | Trace chat request end-to-end (session_id, message_id). | Tracing integration |

**Exit criteria:** Operations can monitor chat usage and debug via logs/metrics/traces.

---

## Phase 10: Config and feature flag (DevOps)

**Owner:** DevOps Engineer  
**Depends on:** Phases 2–3 (to know what to toggle)  
**Deliverables:** Config and deploy notes.

| # | Task | Deliverable |
|---|------|-------------|
| 10.1 | Add `CHAT_ENABLED` (or equivalent) and WebSocket path to api-gateway config; document env vars and Helm values. | Config schema + docs |
| 10.2 | Deploy/observability notes: gateway scaling for connection count, any health-check or readiness changes for /chat/ws. | Runbook or deploy doc update |

**Exit criteria:** Chat can be enabled/disabled via config; deploy and ops docs updated.

---

## Phase 11: QA test plan (QA Engineer)

**Owner:** QA Engineer  
**Depends on:** Phases 2–6 (session API, WebSocket, protocol, loop, tools)  
**Deliverables:** Test plan and execution.

| # | Task | Deliverable |
|---|------|-------------|
| 11.1 | Test: create session → connect WebSocket with token → send message → receive streamed chunks and done. | Test case + result |
| 11.2 | Test: tool_call and tool_result in stream. | Test case + result |
| 11.3 | Test: auth failure (invalid token, wrong session owner) → 403 or 4xx close. | Test case + result |
| 11.4 | Test: rate limit and max message length (if implemented). | Test cases + results |

**Exit criteria:** Test plan document; critical paths passing.

---

## Phase 12: PRD and docs update (Tech Lead)

**Owner:** Tech Lead  
**Depends on:** Implementation complete (Phases 1–11)  
**Deliverables:** PRD and validate.sh updated.

| # | Task | Deliverable |
|---|------|-------------|
| 12.1 | Update PRD: chat agents (session model, WebSocket endpoint, streaming protocol); integration with existing services; add chat to api-gateway REST table; reference docs/chat-agents-design.md. | PRD edits |
| 12.2 | Update `scripts/validate.sh`: add validation for chat (e.g. session create, WebSocket connect, message flow) where applicable. | validate.sh changes |

**Exit criteria:** PRD is source of truth for chat; validate.sh reflects chat capability.

---

## Dependency summary

```
Phase 1 (DB) ─────────────────────────────────────────────────────────────┐
     │                                                                      │
     ▼                                                                      │
Phase 2 (REST sessions) ──► Phase 3 (WS + auth) ──► Phase 4 (protocol)     │
     │                              │                        │              │
     │                              └────────────────────────┼──────────────┤
     │                                                       ▼              │
     │                                              Phase 5 (chat loop)     │
     │                                                       │              │
     │                                                       ▼              │
     │                                              Phase 6 (tools)        │
     │                                                       │              │
     ├───────────────────────────────────────────────────────┼──────────────┤
     │                                                       ▼              │
     │                                              Phase 7 (memory, opt)   │
     │                                              Phase 8 (limits, opt)   │
     │                                              Phase 9 (observability) │
     │                                                                      │
     └──────────────────────────────────────────────────────► Phase 10 (config)
                                                             Phase 11 (QA)
                                                             Phase 12 (PRD)
```

---

## Role summary

| Role | Phases | Focus |
|------|--------|--------|
| **DB Architect** | 1 | chat_sessions, chat_messages, agents.chat_capable migrations |
| **Tech Lead** | 2–9, 12 | Break tasks, delegate to Go Engineer, PRD/validate.sh |
| **Go Engineer** | 2–9 | REST, WebSocket, protocol, chat loop, tools, memory, limits, observability |
| **DevOps** | 9 (notes), 10 | Metrics/deploy notes, CHAT_ENABLED, WebSocket path, runbook |
| **QA Engineer** | 11 | Test plan: session → WS → stream; tool_call/result; auth/403; limits |

---

*Generated from docs/chat-agents-design.md. Use this plan in PR descriptions or a project board to track chat agents implementation.*
