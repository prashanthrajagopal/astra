# Chat Agents Design

**Status:** Design  
**Owner:** Principal Architect  
**PRD alignment:** Extends Astra with real-time chat-capable agents; reuses identity, access-control, kernel/agents, worker manager, tool runtime, LLM router. No duplicate agent definitions—chat agents are agents with chat capability or a dedicated chat interface to an existing agent.

---

## 1. Overview & Goals

### 1.1 Goals

- **Chat agents**: Agents that are explicitly "chat capable"—users open a WebSocket connection to a chat endpoint and exchange messages. Responses stream back (token-by-token or chunk-by-chunk), not just a single final reply.
- **WebSocket endpoint**: Clients connect (e.g. `wss://api/chat/ws` or per-session URL). Authentication via JWT (query param or first message). Support for reconnection and session continuity.
- **Streaming**: Responses stream to the client (WebSocket frames or SSE) so the UI can show typing/streaming. Define message format: e.g. `{ type: "chunk", content: "..." }`, `{ type: "done" }`, `{ type: "tool_call" }` when agents invoke tools mid-stream.
- **Behind-the-scenes work**: Chat agents can call the same worker/tool runtime and planner as goal-based agents (e.g. "run this code", "search the web"). The design allows the chat agent to dispatch work to workers and stream back results or progress.
- **Chat clients/sessions**: Users create chat sessions (e.g. `POST /chat/sessions` with `agent_id`), receive `session_id` and WebSocket URL. Multiple concurrent sessions per user. Sessions bind user, agent, and WebSocket (or multiple tabs).
- **Integration**: Reuse identity (JWT), access-control, kernel/agents, worker-manager, tool-runtime, LLM router. A "chat agent" is an agent with a chat capability flag or a dedicated chat interface to an existing agent.

### 1.2 Non-Goals (this design)

- Building a separate "chat-only" agent type; chat is a capability or interface to existing agents.
- Replacing goal-based flows; chat and goals can coexist (e.g. optional goal created per turn or fire-and-forget tool calls).

---

## 2. High-Level Architecture

### 2.1 Where WebSockets Live

**Recommendation: WebSocket endpoint on api-gateway (v1); optional dedicated chat-service for scale.**

| Option | Pros | Cons |
|--------|------|------|
| **A: api-gateway only** | Single entry point; same JWT/OPA path; no new service. | Gateway holds long-lived connections; scaling connections = scaling gateway. |
| **B: Dedicated chat-service** | Connections isolated; scale chat independently; clear separation. | Extra service; clients need chat-service URL or gateway must proxy WebSocket. |

**Chosen approach (v1):** WebSocket on **api-gateway**. The gateway performs the HTTP upgrade, validates JWT (query param or first text frame), and runs a **chat handler** that can be implemented in-process (internal/chat) or, in a later phase, delegate to a **chat-service** over gRPC (streaming). Session and message state live in Postgres/Redis regardless.

- **REST**: `POST /chat/sessions`, `GET /chat/sessions`, `GET /chat/sessions/:id` — all on api-gateway, JWT + OPA as today.
- **WebSocket**: `wss://{api-gateway}/chat/ws?session_id=...&token=...` (or token in first message). Same host as REST; auth on upgrade or first message.

**Future option:** Introduce `chat-service` (17th service) that owns WebSocket listener and conversation loop; api-gateway either proxies WebSocket to it or returns a chat-service URL in the session create response. This design does not require it for v1.

### 2.2 Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Client (browser / SDK)                                                   │
│  - REST: POST /chat/sessions, GET /chat/sessions                         │
│  - WebSocket: wss://.../chat/ws?session_id=&token=                       │
└───────────────────────────────┬─────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  api-gateway                                                             │
│  - JWT validation (identity), OPA check (access-control)                 │
│  - REST /chat/sessions → create/list/get session                        │
│  - WebSocket /chat/ws → upgrade, then chat handler (in-process or proxy)  │
│  - Chat handler: session lookup, message loop, stream responses          │
└───────────────────────────────┬─────────────────────────────────────────┘
                                │
        ┌───────────────────────┼───────────────────────┐
        ▼                       ▼                       ▼
┌───────────────┐     ┌─────────────────┐     ┌─────────────────┐
│ identity      │     │ access-control  │     │ agent-service    │
│ (validate JWT)│     │ (chat:use,      │     │ (agent profile,  │
│               │     │  agent access)  │     │  chat_capable)   │
└───────────────┘     └─────────────────┘     └────────┬─────────┘
                                                       │
        ┌───────────────────────┬──────────────────────┼──────────────────────┐
        ▼                       ▼                      ▼                      ▼
┌───────────────┐     ┌─────────────────┐     ┌───────────────┐     ┌───────────────┐
│ llm-router    │     │ worker-manager   │     │ tool-runtime   │     │ memory-service│
│ (streaming   │     │ (dispatch tasks │     │ (tool exec     │     │ (recall for   │
│  complete)   │     │  for tools)      │     │  in sandbox)   │     │  context)     │
└───────────────┘     └─────────────────┘     └───────────────┘     └───────────────┘
```

### 2.3 Chat Handler Responsibilities (in api-gateway or chat-service)

- Validate JWT (query param or first message).
- Load session (session_id → user_id, agent_id); verify session belongs to subject and agent is chat-capable.
- Per user message: optionally load conversation history from Redis/Postgres; build prompt; call LLM (streaming) and/or decide to invoke tools.
- When tool/worker is needed: dispatch to worker-manager/tool-runtime (same path as goal-based execution); stream back `tool_call` / `tool_result` frames; then continue LLM stream or send `done`.
- Persist messages asynchronously (non-blocking for hot path where possible).

---

## 3. Session Model

### 3.1 Entities

- **Chat session**: Binds a **user** (JWT subject), an **agent** (agent_id), and a logical conversation. One user can have multiple sessions (e.g. multiple agents or multiple tabs).
- **Chat message**: A single user or assistant message in a session, with optional tool_call/tool_result metadata.

### 3.2 Lifecycle

| Action | Description |
|--------|-------------|
| **Create** | `POST /chat/sessions` with `agent_id` (and optional `title`). Identity from JWT. access-control checks e.g. `chat:use` and user’s right to chat with that agent. Create row in `chat_sessions`, return `session_id` and WebSocket URL. |
| **List** | `GET /chat/sessions` — list sessions for the authenticated user (optional `?agent_id=`). Served from cache where possible (≤10ms). |
| **Get** | `GET /chat/sessions/:id` — get one session; must belong to user. |
| **Resume** | Client opens WebSocket with `session_id` (and token). Server loads session, verifies ownership, continues conversation. |
| **TTL / expiry** | Sessions can have an optional `expires_at` or TTL (e.g. 7 days of inactivity). Background job or on-next-access check marks expired sessions; reconnection with expired session returns 4xx and message to create a new session. |

### 3.3 Database (see Migration path)

- **chat_sessions**: id, user_id (JWT subject), agent_id, title (optional), status (active, closed), created_at, updated_at, expires_at (optional).
- **chat_messages**: id, session_id, role (user | assistant | system), content (text), tool_calls (JSONB, optional), tool_results (JSONB, optional), created_at. Optional: token_count for caps.

Sessions are the binding between user, agent, and WebSocket; multiple tabs can share the same session_id (last writer or append-only messages).

---

## 4. Message and Streaming Protocol

### 4.1 Transport

- **WebSocket** (primary): Binary or text frames. This design uses **JSON text frames** for both client and server.
- **SSE fallback** (optional): Same JSON message types over SSE (e.g. for environments where WebSocket is blocked). Not required for v1.

### 4.2 Client → Server Frames

All client messages are JSON objects.

| type | Description | Payload |
|-----|-------------|--------|
| **auth** | (Optional) If token not in query param, send once after connect: `{ "type": "auth", "token": "<JWT>" }`. |
| **message** | User chat message. | `{ "type": "message", "content": "<text>", "id": "<client_msg_id>" }` |
| **ping** | Keepalive. | `{ "type": "ping" }` |

- **id** (client_msg_id): Optional idempotency key; server can echo it in the reply envelope for deduplication.
- **content**: Required for `message`; max length enforced (e.g. 64 KiB per message).

### 4.3 Server → Client Frames

All server messages are JSON objects with a **type** field.

| type | Description | Payload |
|------|-------------|--------|
| **chunk** | Streaming text delta (token or chunk). | `{ "type": "chunk", "content": "<delta>", "message_id": "<id>" }` |
| **message_start** | Start of a new assistant message (optional, for UI). | `{ "type": "message_start", "message_id": "<id>" }` |
| **message_end** | End of assistant message. | `{ "type": "message_end", "message_id": "<id>", "usage": { "tokens_in": n, "tokens_out": n } }` |
| **tool_call** | Agent is invoking a tool (mid-stream or before more text). | `{ "type": "tool_call", "call_id": "<id>", "name": "<tool_name>", "arguments": "<json>", "message_id": "<id>" }` |
| **tool_result** | Result of a tool call (streamed or full). | `{ "type": "tool_result", "call_id": "<id>", "result": "<text or json>", "message_id": "<id>", "done": true }` |
| **done** | Turn complete; no more chunks for this user message. | `{ "type": "done", "message_id": "<id>" }` |
| **error** | Error for this turn or connection. | `{ "type": "error", "code": "<code>", "message": "<text>", "message_id": "<id>" }` |
| **pong** | Response to ping. | `{ "type": "pong" }` |
| **session** | Sent once after successful auth/resume. | `{ "type": "session", "session_id": "<id>", "agent_id": "<id>" }` |

- **message_id**: Server-assigned id for the assistant message (or the user message being replied to); links chunks, tool_call, tool_result, message_end, done.
- **usage**: Optional in message_end; tokens_in/tokens_out for billing/display.

### 4.4 Ordering and Batching

- Chunks are sent in order; no interleaving of two different message_ids.
- tool_call is sent when the model emits a tool call; tool_result is sent when the tool run completes (or streams). Then streaming can resume (more chunks) until message_end/done.

---

## 5. How Chat Agents Invoke Workers/Tools and Re-enter the Stream

### 5.1 Model

- The chat handler maintains a **conversation loop** for the session: user message → (optional context from memory-service) → prompt to LLM (streaming).
- When the LLM (or the chat orchestrator) decides to call a tool:
  1. Emit **tool_call** frame(s) to the client.
  2. Call **tool-runtime** (or worker-manager for task-based tools) with the same sandbox and policy checks as goal-based agents (S4, S5, S6).
  3. access-control checks the action (e.g. tool execution for this agent/user).
  4. On result: emit **tool_result** frame(s); optionally append to conversation and continue LLM stream (e.g. “here’s the result; continue”).
- So **workers/tools** are invoked by the same backend (tool-runtime / worker-manager); only the **trigger** is the chat handler instead of the scheduler. Results re-enter the stream as **tool_result** (and optionally more **chunk**/message_end).

### 5.2 Mapping to Existing Stack

| Concern | Reuse |
|--------|--------|
| Agent identity & profile | agent-service (agent_id, profile, chat_capable flag or capability). |
| LLM calls | llm-router; add or use **streaming** Complete RPC (e.g. stream of chunks). |
| Tool execution | tool-runtime (sandbox, OPA, approval gates). Chat handler calls tool-runtime; no task graph required for simple tool call. |
| Long-running or multi-step work | Option A: fire-and-forget task via goal-service (create goal, return goal_id in tool_result; client can poll or subscribe). Option B: synchronous tool call from chat handler and stream result. Prefer synchronous tool call for v1; optional goal creation per turn later. |
| Memory | memory-service for context retrieval (e.g. recent episodes) when building the prompt. |

### 5.3 Optional Goal per Turn

- For “run this code” or “search the web”, the chat handler can call tool-runtime directly and stream back the result.
- If the product later wants a full task graph (e.g. “plan and execute”), the chat handler can create a **goal** via goal-service and stream progress (e.g. TaskCompleted events) as **tool_result** or custom event types; this is an extension, not required for v1.

---

## 6. Auth and Authorization

### 6.1 Authentication

- **REST** `/chat/sessions`: Same as today—Bearer JWT in `Authorization`; identity validates token; api-gateway passes subject to session create/list.
- **WebSocket**: Either:
  - **Query param**: `wss://.../chat/ws?session_id=<id>&token=<JWT>`. Validate token on upgrade; reject with 4xx if invalid.
  - **First message**: If no token in query, require first frame to be `{ "type": "auth", "token": "<JWT>" }`; validate then bind session; else close with 4xx.

### 6.2 Authorization

- **Who can open a chat to which agent?** access-control policy: e.g. action `chat:use` or `chat:connect`, resource `agent:<agent_id>` (or `session:<session_id>`). Subject = JWT subject (user).
- **Session create**: Check `chat:use` (or equivalent) for resource `agent:<agent_id>`.
- **WebSocket connect**: Load session by session_id; verify session.user_id == subject; verify agent is chat-capable; then allow. Else 403.

### 6.3 Security Compliance (S1–S6)

- **S1 (mTLS)**: Client → api-gateway is TLS; api-gateway → identity/access-control/agent-service etc. over mTLS as today.
- **S2 (JWT)**: All chat REST and WebSocket require JWT.
- **S3 (RBAC/OPA)**: Chat session create and WebSocket connect go through access-control.
- **S4 (sandbox)**: Tool runs from chat use the same tool-runtime sandbox.
- **S5 (secrets)**: No secrets in chat messages or logs; tool-runtime continues to use Vault/ephemeral volumes.
- **S6 (approval gates)**: Dangerous tool actions still go through approval_requests; chat handler checks approval_required and can stream “pending_approval” to client.

---

## 7. Optional: Idempotency, Rate Limits, Limits

### 7.1 Idempotency

- Client can send `id` (client_msg_id) with each `message`. Server stores it; if the same session receives the same `id` again (e.g. retry), server can return cached reply or 409. Optional for v1.

### 7.2 Rate Limits

- Per user: e.g. max N messages per minute (configurable); 429 + error frame if exceeded.
- Per session: optional max concurrent “thinking” (one message in flight per session is simplest).

### 7.3 Max Message Length

- Client message: e.g. 64 KiB; reject with error frame if exceeded.
- Assistant message: limit by LLM max_tokens; stream truncation or error if hit.

### 7.4 Token Limits per Session

- Optional: cap total tokens (in + out) per session or per day; when exceeded, send error frame and refuse further messages until next day or new session.

---

## 8. Migration Path

### 8.1 New Tables (DB Architect)

- **chat_sessions**: id (UUID), user_id (TEXT, JWT subject), agent_id (UUID FK agents), title (TEXT), status (TEXT), created_at, updated_at, expires_at (TIMESTAMPTZ nullable). Index: (user_id), (agent_id), (user_id, updated_at).
- **chat_messages**: id (UUID), session_id (UUID FK chat_sessions), role (TEXT: user | assistant | system), content (TEXT), tool_calls (JSONB), tool_results (JSONB), created_at. Index: (session_id, created_at).

Idempotent migrations; no drop columns without explicit approval.

### 8.2 Agents and Chat Capability

- **Option A**: Add column `agents.chat_capable BOOLEAN DEFAULT false`. Only agents with chat_capable=true can be used in chat sessions.
- **Option B**: No new column; any agent can have a chat session; capability could be enforced by OPA (e.g. only certain agents allow chat:use). Recommend Option A for clarity.

### 8.3 New Code / Services

- **api-gateway**: New REST routes `POST/GET /chat/sessions`, `GET /chat/sessions/:id`. New WebSocket route `/chat/ws` with upgrade and chat handler. Handler uses identity + access-control; reads/writes sessions and messages (via new internal/chat or chat-service stub).
- **internal/chat** (or **cmd/chat-service**): Session CRUD, message append, conversation loop (load history → LLM stream → tool_call → tool_result → stream). Calls agent-service, llm-router, tool-runtime, memory-service. If in-process, lives in api-gateway; if separate service, api-gateway proxies WebSocket to chat-service.
- **llm-router**: Add or use streaming RPC (e.g. `CompleteStream` returning stream of chunks) so chat can stream tokens. If not present, add in this feature.

### 8.4 Proto / API Changes

- **REST**: Document `POST /chat/sessions` (body: agent_id, optional title), `GET /chat/sessions`, `GET /chat/sessions/:id`. Response: session_id, agent_id, websocket_url (or path), created_at, etc.
- **WebSocket**: Document query params (session_id, token) and frame formats (§4).
- **gRPC (optional)**: If chat-service is separate, define ChatService with e.g. OpenStream(session_id, user_id) returns stream ServerMessage for streaming from chat-service to gateway.

### 8.5 Config and Deploy

- Feature flag or config: enable chat routes and WebSocket (e.g. `CHAT_ENABLED=true`).
- No new Helm service for v1 if chat handler is in api-gateway; only new env and possibly scaling considerations for gateway (connection count).

---

## 9. Implementation Checklist (for Tech Lead)

Ordered steps for implementation and delegation. Do not implement code in the design phase—this checklist is for Tech Lead to assign to Go Engineer, DevOps, QA, DB Architect as appropriate.

1. **DB schema (DB Architect)**  
   - Add migration: `chat_sessions` table (id, user_id, agent_id, title, status, created_at, updated_at, expires_at); indexes.  
   - Add migration: `chat_messages` table (id, session_id, role, content, tool_calls, tool_results, created_at); index (session_id, created_at).  
   - Add `agents.chat_capable` BOOLEAN column (default false) in same or separate migration.

2. **Session REST API (Go Engineer)**  
   - Implement session create/list/get in api-gateway (or dedicated handler package): POST/GET /chat/sessions, GET /chat/sessions/:id.  
   - Use identity (JWT) and access-control (chat:use, resource agent:<id>).  
   - Persist to Postgres; return session_id and WebSocket path (e.g. /chat/ws?session_id=...).  
   - Ensure list/get read path is cacheable (e.g. Redis) for ≤10ms where possible.

3. **WebSocket upgrade and auth (Go Engineer)**  
   - In api-gateway, add route for /chat/ws; perform HTTP upgrade.  
   - Extract JWT from query param or first JSON frame (type auth).  
   - Validate JWT via identity; load session; verify session.user_id == subject and agent is chat_capable; OPA check for chat:connect.  
   - On success, send `session` frame; on failure, close with 4xx.

4. **Message protocol (Go Engineer)**  
   - Implement client frame parsing (message, ping, auth).  
   - Implement server frame writing (chunk, message_start, message_end, tool_call, tool_result, done, error, pong).  
   - Enforce max message length for client messages; respond with error frame if exceeded.

5. **Chat conversation loop (Go Engineer)**  
   - For each user `message` frame: load session and recent messages (from Redis/Postgres); build prompt; call LLM (streaming).  
   - Stream chunks to client (chunk frames); on message_end send message_end and done.  
   - Integrate with llm-router: add or use streaming Complete RPC and wire to chunk emission.

6. **Tool invocation from chat (Go Engineer)**  
   - When LLM or orchestrator decides to call a tool: emit tool_call frame; call tool-runtime (or worker-manager) with same policy/sandbox as goal-based flow; on result emit tool_result frame.  
   - Optionally continue LLM stream after tool result.  
   - Respect approval_required from access-control; stream pending_approval or error to client.

7. **Memory and context (Go Engineer)**  
   - Optional: when building prompt, call memory-service for recent context (e.g. last N messages or semantic recall).  
   - Append to system prompt or context window; stay within token limits.

8. **Rate limits and caps (Go Engineer)**  
   - Optional: per-user or per-session rate limit (e.g. N messages per minute); return 429 or error frame.  
   - Optional: token limit per session; reject with error when exceeded.

9. **Observability (Go Engineer / DevOps)**  
   - Log session create, WebSocket connect/disconnect, message counts.  
   - Metrics: chat_sessions_active, chat_messages_total, chat_tool_calls_total.  
   - Trace chat request end-to-end (session_id, message_id).

10. **Config and feature flag (DevOps / Go Engineer)**  
    - Add CHAT_ENABLED (or similar) and WebSocket path to api-gateway config.  
    - Document env vars and Helm values if applicable.

11. **QA**  
    - Test: create session → connect WebSocket with token → send message → receive streamed chunks and done.  
    - Test: tool_call and tool_result in stream.  
    - Test: auth failure (invalid token, wrong session owner), 403.  
    - Test: rate limit and max message length if implemented.

12. **PRD and docs update (Tech Lead)**  
    - Update PRD with chat agents: session model, WebSocket endpoint, streaming protocol, integration with existing 16 services.  
    - Add chat to api-gateway REST table and reference this design doc.  
    - Update validate.sh if new validations are required for chat.

---

## 10. Summary

- **WebSocket on api-gateway** (v1); optional chat-service later for scale.  
- **Sessions** in Postgres (chat_sessions, chat_messages); create/list/resume with TTL option.  
- **Protocol**: JSON frames (client: auth, message, ping; server: chunk, message_end, tool_call, tool_result, done, error, pong, session).  
- **Workers/tools**: Same tool-runtime and access-control; chat handler invokes them and streams results back.  
- **Auth**: JWT (query or first message) + OPA (chat:use / chat:connect).  
- **Migration**: New tables, optional agents.chat_capable, new routes and WebSocket handler; optional streaming LLM RPC; no duplicate agent definitions.
