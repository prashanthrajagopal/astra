# Slack Integration Design

**Status:** Design  
**Owner:** Principal Architect  
**PRD alignment:** Connects Astra chat agents to Slack; reuses identity, access-control, org model, and existing chat/goal APIs. No new agent types—Slack is an additional channel to existing chat-capable agents.

---

## 1. Overview & Goals

### 1.1 Goals

- **Slack as a channel:** Users interact with Astra chat agents from Slack (DMs with the bot, or in channels where the bot is invited). Messages in Slack are delivered to the same chat pipeline as the dashboard (session + message → agent → streaming or REST reply).
- **Multi-tenant:** One Slack workspace is linked to one Astra organization. Optionally, channels can be bound to specific agents. All traffic is scoped by org and respects RBAC.
- **Secure:** Verify every request from Slack (signing secret), map Slack users to Astra users/org, store tokens and secrets safely (Vault).
- **Fast ack:** Slack expects HTTP 200 within 3 seconds for Events API and slash commands. Heavy work (LLM, goal submission) must be asynchronous; the adapter responds quickly and processes in the background.

### 1.2 Non-Goals (this design)

- Slack as the only way to manage orgs or agents; org/admin and chat remain in dashboard/API.
- Building a separate “Slack-only” agent type; the same chat-capable agents are used.
- Slack Block Kit UI beyond basic message formatting (optional later).

---

## 2. Integration Model

### 2.1 Slack App Shape

| Mechanism | Use | Astra usage |
|-----------|-----|-------------|
| **Events API** | Subscribe to `message` (channels/DM), `app_mention`. Bot receives events at a Request URL. | Primary: user messages → adapter → chat session/message. |
| **Slash commands** | Optional: e.g. `/astra <message>` or `/astra-goal <goal text>`. | Same handler; can create session or submit goal. |
| **Bot user** | Bot is invited to channels or users DM the bot. | One bot per Slack app; org is determined by workspace. |

**Recommendation:** Slack App with **Events API** (Event Subscriptions) and **Bot user**. Slash commands are optional for v1. OAuth (see §4) links a Slack workspace to an Astra org.

### 2.2 Which Astra APIs the Adapter Calls

| User action in Slack | Adapter behavior | Astra API(s) |
|---------------------|------------------|--------------|
| User sends message in channel/DM | Resolve org + optional agent from workspace/channel binding → create or resume chat session → append user message → get assistant reply | `POST /chat/sessions` (or get existing), `POST /chat/sessions/{id}/messages` (dashboard-style append) or equivalent internal path; or enqueue to worker that calls same chat logic. |
| Optional: slash `/astra-goal <text>` | Resolve org + default agent → submit goal | `POST /goals` or `POST /agents/{id}/goals` with JWT or service identity. |

**Chat path:** Existing chat is in api-gateway: REST create session, WebSocket or REST append message. For Slack we need a **non-WebSocket** path: e.g. **POST /chat/sessions/{id}/messages** (already present for dashboard) that returns the assistant reply (sync or 202 + callback). Design assumes the adapter either:

- **Option A (preferred):** Call an **internal** or **org-scoped** chat API that accepts a single user message and returns (or streams) the assistant reply—e.g. same handler as dashboard “append message” that returns the assistant message in the response body. Adapter uses that and then posts the reply back to Slack.
- **Option B:** Adapter enqueues a job (Redis stream or queue); a worker consumes it, calls existing chat logic (e.g. `ProcessRESTMessage` / `ProcessGoalMessage`), then posts result to Slack via Slack API (with bot token). Adapter responds 200 OK to Slack immediately so Slack does not retry.

**Goal path:** If slash command or heuristic triggers goal submission, adapter calls `POST /goals` (or `POST /agents/{id}/goals`) with an org-scoped JWT or internal token, then posts “Goal submitted, id: …” to Slack. Goal execution is async as today.

### 2.3 OAuth for Org/Workspace Linking

- **Slack OAuth v2:** When an org admin “Connects Slack” in Astra (e.g. org dashboard), they are sent to Slack’s OAuth consent. Astra receives the callback with `code` → exchanges for `access_token` (bot token), `team.id` (workspace id), and optionally `authed_user` (for “who installed”).
- **Linking:** Store `workspace_id` (Slack team id) → `org_id` (Astra org). One workspace → one org. Bot token is stored per org (see §5). Re-install or re-authorize updates the token.

---

## 3. Multi-Tenancy

### 3.1 Workspace → Org

- **One Slack workspace ↔ one Astra organization.** The table `slack_workspaces` (or equivalent) stores `(org_id, slack_workspace_id, ...)`. Every incoming Slack request is keyed by `team_id` (Slack) → look up `org_id`.
- **Unlinked workspace:** If `team_id` is not in the table, respond 200 OK with no-op (or a short message “Workspace not connected to Astra”) to avoid Slack retries, and log.

### 3.2 Channel and Agent Binding

- **Default agent per org:** Org can have a default `agent_id` for Slack (e.g. “chat-assistant”). All DMs and unbounded channels use this agent.
- **Optional per-channel agent:** Table `slack_channel_bindings(org_id, channel_id, agent_id)`. If the message is in a channel and a binding exists, use that agent; else use org default. Enables e.g. #support → agent A, #dev → agent B.

### 3.3 Scoping and Auth

| Scope | How |
|-------|-----|
| **Per-workspace** | All events for that workspace are tied to one `org_id`. Bot token is org-scoped. |
| **Per-channel** | Optional agent override per channel; still under same org. |
| **Per-org** | All data (sessions, messages, goals) created from Slack for that workspace are under that org. RBAC and visibility (agents, goals) follow existing org rules. |

Slack user → Astra user mapping (see §4) determines which Astra user is used for session ownership and for access-control checks (e.g. `chat:use` for that agent).

---

## 4. Security and Auth

### 4.1 Verifying Requests from Slack

- **Signing secret:** Slack signs every HTTP request with HMAC-SHA256 using the app’s **Signing Secret**. The adapter must verify the `X-Slack-Signature` header (or `X-Slack-Request-Timestamp` + signature) before processing. Invalid signature → 401 and do not process.
- **Replay:** Use `X-Slack-Request-Timestamp`; reject if older than e.g. 5 minutes to avoid replay.

### 4.2 Mapping Slack User → Astra User / Org

- **Org** is known from `team_id` (workspace) → `slack_workspaces.org_id`.
- **User:** Two approaches:
  - **Option A (v1):** Use a single “Slack bot user” per org (e.g. a system user or the org admin who installed). All Slack-originated sessions are owned by that user. Simple but no per-Slack-user identity.
  - **Option B (recommended for production):** Maintain a mapping `slack_user_id` → `astra_user_id` per org. When a Slack user first messages, they are either linked (e.g. “Connect your Slack” in Astra dashboard, storing `slack_user_id` for that Astra user) or a default/guest user is used for that org. Sessions then have the correct `user_id` for RBAC and audit.

Store in DB: e.g. `slack_user_mappings(org_id, slack_user_id, astra_user_id)`.

### 4.3 Storing Slack Tokens and Secrets

- **Bot token (OAuth):** Must not live in code or logs. Options:
  - **Vault:** Preferred. Store per-org Slack bot token in Vault (e.g. `secret/astra/slack/org/{org_id}`). Adapter (or worker) fetches token at runtime when posting back to Slack.
  - **DB with encryption:** If Vault is not used, encrypt the token in a column (e.g. `encrypted_bot_token`) and use a key from env/Vault only for encryption/decryption. Key never in DB.
- **Platform Slack app secrets (Signing Secret, Client ID, Client Secret):** Configurable from the **super-admin UI** so operators do not rely on env vars or Vault for initial setup. Stored in DB (e.g. `platform_settings` or `slack_app_config`) with values encrypted at rest; adapter and OAuth callback read from DB or a cached config. Fallback: env vars (e.g. `SLACK_SIGNING_SECRET`, `SLACK_CLIENT_ID`, `SLACK_CLIENT_SECRET`) for backward compatibility. **Redirect URL** for OAuth can be entered in the UI or derived from a base URL setting.

---

## 5. Components

### 5.1 New Service vs Extending api-gateway

| Option | Pros | Cons |
|--------|------|------|
| **slack-adapter service** (new) | Isolates Slack-specific logic, scales independently, clear boundary. Can enqueue work and respond 200 quickly. | Extra service, another HTTP endpoint to expose (Slack Request URL). |
| **Extend api-gateway** | Single entrypoint, reuse auth middleware (for internal calls). | Gateway must expose a **public** HTTP endpoint for Slack (no JWT from Slack). Signature verification is different from JWT. Mixing Slack and tenant APIs in one process. |

**Recommendation:** **New service `slack-adapter`** (or `cmd/slack-adapter`). It is the only component that speaks Slack’s HTTP contract (signing secret, Events API payloads, slash command payloads). It does not need to serve JWT-protected Astra APIs; it receives Slack requests, verifies signature, resolves org/agent/user, then either:

- Calls api-gateway (or an internal chat API) with an **internal/service token** or server-to-server auth (mTLS + service account), or
- Pushes a job to a **Redis stream** (e.g. `astra:slack:incoming`) and responds 200 OK to Slack immediately; a **worker** (same process or dedicated) consumes the stream, performs chat/goal call, then uses Slack API to post the reply.

This keeps the 10ms rule for “response to Slack” (adapter responds in &lt;3s; heavy work is async).

### 5.2 Event Flow (Slack → Astra → Reply)

1. **Slack** sends HTTP POST to adapter Request URL (event or slash command).
2. **Adapter** verifies signing secret, parses body. For Events API `url_verification`, respond with `challenge` immediately.
3. **Adapter** looks up `team_id` → `org_id`; if not found, 200 OK + no-op.
4. **Adapter** resolves channel → optional agent; else org default agent. Resolves Slack user → Astra user (or default).
5. **Adapter** either:
   - **Sync (only if fast):** Create/resume session, call chat append-message API (internal), wait for reply (with short timeout, e.g. 10s). If timeout or error, post “Sorry, I’m busy, try again” and still return 200.
   - **Async (preferred):** Enqueue message to Redis stream `astra:slack:incoming` with payload (workspace_id, channel_id, user_id, message_ts, text, org_id, agent_id, astra_user_id). Return 200 OK immediately. **Worker** consumes stream: create/resume session, call chat API (or goal API), then post reply to Slack via `chat.postMessage` (using bot token from Vault).
6. **Slack** shows the reply in the channel/DM.

### 5.3 Queue vs Direct HTTP

- **Direct HTTP:** Adapter calls api-gateway’s chat endpoint and waits. Risk: LLM or goal can take &gt;3s → Slack retries, duplicate messages. Only acceptable if chat response is guaranteed fast (e.g. &lt;2s) or if we accept rare duplicates.
- **Queue (recommended):** Adapter enqueues to Redis stream; worker processes and posts to Slack. Adapter responds 200 in &lt;100ms. Fits 10ms philosophy (adapter path is cheap); heavy work is off the request path. Enables retries and rate limiting per org.

**Recommendation:** Use a **Redis stream** `astra:slack:incoming` (or similar). Adapter publishes; one or more workers consume, call Astra chat (and optionally goal) API, then post to Slack. Rate limits and retries can be applied in the worker.

### 5.4 Rate Limits and Retries

- **To Slack API:** Respect Slack’s rate limits (e.g. tier 3 for `chat.postMessage`). Use exponential backoff and retry (with idempotency) when posting replies.
- **From Slack:** Per-org or per-channel rate limit (e.g. max N messages per minute) to avoid abuse; 200 OK but skip processing if over limit and optionally post “Rate limited” once.
- **Worker retries:** On transient failure (e.g. Astra chat API 5xx), retry with backoff and dead-letter after N failures.

---

## 6. Data and Storage

### 6.1 What to Persist

| Data | Purpose |
|------|---------|
| **Slack workspace → org** | `slack_workspaces`: org_id, slack_workspace_id (team_id), bot_token_ref (Vault path or encrypted token), installed_at, optional default_agent_id. |
| **Channel → agent** | `slack_channel_bindings`: org_id, slack_channel_id, agent_id (optional). If absent for a channel, use org default agent. |
| **Slack user → Astra user** | `slack_user_mappings`: org_id, slack_user_id, astra_user_id. For RBAC and session ownership. |
| **Session continuity** | Reuse existing `chat_sessions`. For Slack, we can have one session per (org, slack_channel_id, slack_user_id) or per (org, agent_id, slack_user_id) so that thread/reply stays in the same conversation. Store optional `slack_channel_id` / `slack_thread_ts` in session metadata or a small `slack_sessions` table linking session_id to (workspace_id, channel_id, user_id, thread_ts). |

### 6.2 Schema Additions (High Level)

- **slack_app_config (platform-level, for UI-entered secrets):** id, key (e.g. `signing_secret`, `client_id`, `client_secret`, `oauth_redirect_url`), value_encrypted (or value_ref if using Vault); updated_at. Single row or key-value per Slack app. Super-admin enters these in the UI; adapter and api-gateway OAuth callback read them (with decryption key from env/Vault). Alternative: reuse a generic `platform_settings` table with encrypted value column.
- **slack_workspaces:** id, org_id (FK organizations), slack_workspace_id (unique), bot_token_ref (Vault path or encrypted), default_agent_id (FK agents, nullable), created_at, updated_at.
- **slack_channel_bindings:** id, org_id (FK), slack_channel_id, agent_id (FK), created_at. Unique (org_id, slack_channel_id).
- **slack_user_mappings:** id, org_id (FK), slack_user_id, astra_user_id (FK users), created_at. Unique (org_id, slack_user_id).
- **Optional slack_sessions:** id, chat_session_id (FK chat_sessions), org_id, slack_workspace_id, slack_channel_id, slack_user_id, slack_thread_ts (nullable for thread reply), created_at. Enables “resume same conversation” when the user messages again in the same channel/thread.

All tables scoped by org_id; indexes on (slack_workspace_id), (org_id, slack_channel_id), (org_id, slack_user_id).

### 6.3 Config / Migration

- New migration (e.g. `00XX_slack_integration.sql`) adds the above tables and any FKs. Idempotent (IF NOT EXISTS). No drop columns without explicit approval.
- **Secrets:** Signing secret and Vault paths come from env or Vault; no secrets in migrations.

---

## 7. PRD and Roadmap

See **§8** below for the suggested PRD section and **§9** for build order and dependencies.

---

## 8. Suggested PRD Section (Short)

**Slack integration (design):** Users can interact with Astra chat agents from Slack. One Slack workspace is linked to one Astra organization via OAuth. A dedicated **slack-adapter** service receives Slack Events API (and optional slash commands), verifies request signature, resolves org/agent/user, and either enqueues to a Redis stream for async processing or calls an internal chat API. A worker consumes the stream, calls existing chat (and optionally goal) APIs with org-scoped identity, and posts replies back to Slack via the Slack API. Data: `slack_workspaces`, `slack_channel_bindings`, `slack_user_mappings`; bot tokens in Vault. Design: **docs/slack-integration-design.md**.

---

## 9. Build Order and Dependencies

| Order | Deliverable | Dependencies |
|-------|-------------|--------------|
| 1 | **DB migration** — slack_workspaces, slack_channel_bindings, slack_user_mappings (and optional slack_sessions). | None. |
| 2 | **Internal or org-scoped chat “append message” API** — If not already exposed in a way the adapter can call (e.g. with service/internal auth), add an endpoint or internal function that: given org_id, agent_id, user_id, session_id (or create), and message text, runs the same logic as dashboard append and returns the assistant reply (or 202 + webhook). | Existing chat session + message flow in api-gateway. |
| 3 | **slack-adapter service** — HTTP server with Request URL for Slack; verify signing secret; parse Events API and slash commands; resolve org/agent/user; enqueue to Redis stream; 200 OK within 3s. | Migration; config (signing secret, Redis, Vault path). |
| 4 | **Slack worker** — Consume `astra:slack:incoming`; load bot token (Vault); create/resume session; call chat API; post reply to Slack; retries and rate limiting. | Adapter; chat API; Vault for tokens. |
| 5 | **Platform Slack secrets UI** — Super-admin dashboard: form to enter and save **Slack app** Signing Secret, Client ID, Client Secret, OAuth Redirect URL. Stored in DB (encrypted). Adapter and OAuth callback read from DB (or env fallback). | Migration for slack_app_config or platform_settings; super-admin UI. |
| 6 | **OAuth flow** — Org dashboard “Connect Slack” → redirect to Slack OAuth (using Client ID/Redirect URL from platform config) → callback exchanges code using Client Secret; stores workspace + bot token (in Vault) and inserts/updates slack_workspaces. | Identity/org context; Vault; platform Slack secrets (from UI or env). |
| 7 | **Org settings UI** — Optional: default agent for Slack; per-channel agent binding; Slack user mapping (e.g. “Link Slack user” for org members). | OAuth; slack_workspaces/bindings. |
| 8 | **Slash command (optional)** — e.g. `/astra-goal` → goal submit; reuses goal-service API with org-scoped token. | Goal-service; adapter. |

**Dependencies on existing Astra:** Chat sessions and messages (Phase 10); org and RBAC (Phase 11); identity (JWT or service tokens); Redis; Postgres; Vault for secrets. Platform Slack secrets can be entered from the super-admin UI or, as fallback, from env.

---

## 10. Summary

| Area | Decision |
|------|----------|
| **Integration** | Slack App with Events API + Bot user; optional slash commands. OAuth links workspace → org. Adapter calls chat (session + append message) and optionally goal API. |
| **Multi-tenancy** | One workspace → one org; optional per-channel agent; all data scoped by org_id. |
| **Security** | Verify Slack signing secret; map Slack user → Astra user; store bot tokens in Vault (or encrypted in DB). Platform Slack app secrets (signing secret, client id/secret, redirect URL) configurable from super-admin UI, stored encrypted in DB; env fallback. |
| **Components** | New **slack-adapter** service; Redis stream for async work; worker consumes and posts to Slack. Adapter responds 200 quickly. |
| **Data** | slack_workspaces, slack_channel_bindings, slack_user_mappings; optional slack_sessions for thread continuity. |
| **Performance** | Adapter path &lt;3s (prefer &lt;100ms + enqueue); heavy work in worker; no synchronous Postgres on Slack request path for processing. |
