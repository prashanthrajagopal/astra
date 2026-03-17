# Astra — Autonomous Agent Operating System

Astra is the **operating system for autonomous agents**: a production-grade, microkernel-style platform for persistent agents that plan, act, collaborate, remember, and learn. The kernel is minimal and high-performance; agent logic and applications run outside it via SDKs and APIs. Key capabilities include **multi-tenancy** (organizations, teams, users, RBAC, agent visibility hierarchy) and **chat** (WebSocket streaming, sessions, dashboard chat widget). **Target scale:** Millions of agents, 100M+ tasks/day.

---

## Quick Start

### Prerequisites

| Requirement | Purpose |
|-------------|---------|
| **Go 1.25+** | Build and run all services (matches go.mod; Dockerfile uses 1.25) |
| **Postgres** (or Docker) | Primary DB; migrations in `migrations/` |
| **Redis** (or Docker) | Messaging (Redis Streams), ephemeral state, locks |
| **Memcached** (or Docker) | LLM/embedding/tool response cache |
| **psql** (if using native Postgres) | Running migrations when Postgres is not in Docker |
| **Docker** (optional) | Used only when Postgres/Redis/Memcached are not already running |

### Clone, configure, deploy

```bash
cp .env.example .env   # edit if needed (Postgres/Redis/Memcached host, user, password)
./scripts/deploy.sh
```

The deploy script:
- Uses existing Postgres/Redis/Memcached if running, otherwise starts them via Docker
- Runs all SQL migrations in `migrations/`
- Builds all service binaries, restarts all Astra services (task-service, agent-service, scheduler-service, execution-worker, worker-manager, tool-runtime, browser-worker, memory-service, llm-router, prompt-manager, identity, access-control, planner-service, goal-service, evaluation-service, cost-tracker, api-gateway)
- Logs: `logs/*.log`; PIDs: `logs/*.pid`
- Stop: `for f in logs/*.pid; do kill $(cat $f) 2>/dev/null; done`

### GCP (GKE Autopilot)

```bash
cp scripts/.env.gcp.example .env.gcp   # edit GCP_PROJECT, POSTGRES_PASSWORD, etc.
./scripts/gcp-deploy.sh --setup --dev  # first time: Artifact Registry, GKE, Cloud SQL, Redis, Memcached, GCS bucket
./scripts/gcp-deploy.sh --dev          # later: build, push, migrate, helm deploy
```

**Object storage on GCP:** use **Google Cloud Storage** (bucket `gs://${GCP_PROJECT}-astra-workspace` created on `--setup`). Do not run MinIO in GCP; local dev may still use MinIO via `docker compose`. Wire pods to GCS with Workload Identity when services need bucket access.

### Seed agents

```bash
./scripts/seed-agents.sh
```

Creates a default set of agents (Python Expert, Backend Dev, Frontend Dev, E-Commerce Builder, Generalist Coder, Documentation, DevOps, Testing, Chat Assistant). Requires `jq`. Idempotent (skips if agents already exist).

### Verify

- **Dashboard:** `api-gateway` at `/superadmin/dashboard/` — summary stats, goals, tasks, approvals, chat widget (for chat-capable agents).
- **API:** `GET /agents` to list agents; `GET /tasks/{id}`, `GET /graphs/{id}` for results. See [docs/api/openapi.yaml](docs/api/openapi.yaml).

### MLX setup (Apple Silicon)

On macOS with Apple Silicon, you can use [MLX-LM](https://github.com/ml-explore/mlx-lm) for local LLM inference with Metal acceleration instead of (or as fallback after) Ollama.

1. **Install MLX-LM** (use the same Python you run from the shell):
   ```bash
   python -m pip install mlx-lm
   ```
   Verify: `python -c "import mlx; print('MLX installed')"`

2. **Start the MLX server** (in a separate terminal; keep it running):
   ```bash
   mlx_lm.server --model mlx-community/Qwen2.5-7B-Instruct-4bit --port 8888
   ```

3. **Configure `.env`**:
   ```bash
   MLX_HOST=http://localhost:8888
   MLX_MODEL=Qwen2.5-7B-Instruct-4bit
   LLM_DEFAULT_PROVIDER=mlx
   ```

4. **Deploy** — Run `./scripts/deploy.sh` as usual. On macOS, the deploy script may auto-detect MLX and set `LLM_DEFAULT_PROVIDER=mlx` if not already set.

**Other models:** Set `MLX_MODEL` to any MLX-compatible model (e.g. `mlx-community/Mistral-7B-Instruct-v0.3-4bit`). Astra talks to MLX via its OpenAI-compatible `/v1/chat/completions` endpoint. See [docs/local-deployment.md](docs/local-deployment.md) for more detail.

---

## Architecture

### 3.1 Platform layers

Applications and SDK sit above the kernel; the kernel is minimal (actors, task graph, scheduler, message bus, state). Infrastructure is shared.

```
┌─────────────────────────────────────────────────────────┐
│  Applications (Agent Apps)                              │
│    └── Astra SDK (agent dev framework)                  │
│          └── Astra Kernel (microkernel)                 │
│                ├ Actor Runtime                          │
│                ├ Task Graph Engine                      │
│                ├ Scheduler                              │
│                ├ Message Bus                             │
│                └ State Manager                           │
├─────────────────────────────────────────────────────────┤
│  Infrastructure                                          │
│    Postgres (source of truth, pgvector)                 │
│    Redis (streams, state, locks)   Memcached (caches)   │
└─────────────────────────────────────────────────────────┘
```

### 3.2 16 services (control-plane / kernel / workers)

**Control-plane:** API gateway, auth, and routing. **Kernel:** task graph, scheduling, and agent/task state. **Workers:** execution, tools, LLM, and evaluation. Identity handles user CRUD, bcrypt login, and enriched JWT (user_id, org_id, org_role, team_ids, is_super_admin). Access-control uses internal/rbac for RBAC and agent visibility hierarchy (global/public/team/private).

```
┌──────────────────────────────────────────────────────────────────────┐
│  CONTROL-PLANE                                                        │
│  api-gateway │ identity (user CRUD, JWT) │ access-control (RBAC)      │
└────────────────────────────┬─────────────────────────────────────────┘
                              │ JWT + OPA
┌─────────────────────────────▼─────────────────────────────────────────┐
│  KERNEL                                                                │
│  agent-service │ goal-service │ planner-service │ scheduler-service   │
│  task-service │ memory-service                                        │
└─────────────────────────────┬─────────────────────────────────────────┘
                              │ Redis Streams (task shards)
┌─────────────────────────────▼─────────────────────────────────────────┐
│  WORKERS                                                               │
│  execution-worker │ browser-worker │ worker-manager │ tool-runtime     │
│  llm-router │ prompt-manager │ evaluation-service                     │
└──────────────────────────────────────────────────────────────────────┘
```

### 3.3 Service topology (16 canonical services + data stores)

All 16 services and their backing stores. Hot-path reads use Redis/Memcached; writes go to Postgres and emit to streams. Api-gateway hosts WebSocket chat (internal/chat); identity and access-control support multi-tenant RBAC.

```
                    ┌─────────────┐
                    │ api-gateway │ ◀── WebSocket chat, internal/chat
                    └──────┬──────┘
         ┌────────────────┼────────────────┐
         ▼                ▼                 ▼
  ┌────────────┐   ┌────────────┐   ┌───────────────┐
  │  identity  │   │access-     │   │ agent-service  │
  │ user CRUD │   │control     │   │ goal-service   │
  │ JWT+claims│   │RBAC, OPA   │   │ planner-svc    │
  └────────────┘   └────────────┘   │ scheduler-svc  │
                                    │ task-service   │
                                    │ memory-service │
                                    └───────┬───────┘
                                            │
    ┌───────────────────────────────────────┼───────────────────────────────────────┐
    ▼                   ▼                   ▼                   ▼                   ▼
┌────────┐         ┌────────┐         ┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│Postgres│         │ Redis  │         │ Memcached   │     │execution-    │     │ llm-router  │
│        │         │Streams │         │(LLM/embed/  │     │worker        │     │ prompt-mgr  │
│source  │         │state   │         │ tool cache) │     │browser-worker │     │ evaluation  │
│of truth│         │locks   │         └─────────────┘     │worker-manager │     │ tool-runtime│
└────────┘         └────────┘                             └──────────────┘     └─────────────┘
```

### 3.4 Deployment (Kubernetes namespace diagram)

Typical namespace layout and flow: external traffic to api-gateway (mTLS between services); workers in dedicated namespace.

```
  ┌─────────────────────────────────────────────────────────────────────┐
  │  control-plane    api-gateway, identity, access-control              │
  │  (ingress / JWT)                                                     │
  └──────────────────────────────┬─────────────────────────────────────┘
                                  │ mTLS
  ┌───────────────────────────────▼─────────────────────────────────────┐
  │  kernel           scheduler-service, task-service, agent-service,    │
  │                   goal-service, planner-service, memory-service      │
  └───────────────────────────────┬─────────────────────────────────────┘
                                  │ Redis Streams / gRPC
  ┌───────────────────────────────▼─────────────────────────────────────┐
  │  workers          execution-worker, browser-worker, worker-manager, │
  │                   tool-runtime, llm-router, prompt-manager, eval-svc  │
  └───────────────────────────────┬─────────────────────────────────────┘
                                  │
  ┌───────────────────────────────▼─────────────────────────────────────┐
  │  infrastructure   Postgres, Redis, Memcached, MinIO/S3               │
  └─────────────────────────────────────────────────────────────────────┘
```

---

## How It Works

This section gives new contributors an end-to-end picture of Astra. **[docs/PRD.md](docs/PRD.md)** is the single source of truth for architecture, APIs, schema, and implementation details.

### 4.1 Core concepts: agents vs workers

- **Agent** = logical entity that owns goals and task graphs. Agents (and kernel services) create tasks via the planner and task-service; they do **not** execute tasks.
- **Worker** = process that pulls from Redis task streams, claims and runs tasks, and reports `CompleteTask` / `FailTask`.

**Summary:** Agents (and kernel services) create and own tasks; workers execute them.

| | Nature | Creates tasks? | Executes tasks? | Canonical services |
|---|--------|----------------|------------------|--------------------|
| **Agent** | Logical entity (goals, DAGs) | Yes (planner, task-service) | No | agent-service, goal-service, planner-service, scheduler-service, task-service |
| **Worker** | Process (pulls from streams) | No | Yes | execution-worker, browser-worker, worker-manager, tool-runtime, llm-router, prompt-manager, evaluation-service |

### 4.2 Request-to-result lifecycle

**From request to result:** A user or API client sends a request (e.g. create a goal) to the **api-gateway**. The gateway authenticates (JWT via identity) and authorizes (access-control/OPA), then routes to control-plane services. **Goals** are handled by **goal-service**, which invokes the **planner-service** (LLM) to produce a **task graph (DAG)**. The **task-service** persists the graph via `CreateGraph`. The **scheduler-service** periodically detects **ready** tasks (all dependencies completed), marks them queued, and **pushes** task messages to **Redis Streams** (sharded: `astra:tasks:shard:<n>`). **Workers** (e.g. **execution-worker**, **browser-worker**) pull from these streams via consumer groups, **claim** a task (lock, set status scheduled/running), **execute** it (LLM calls via **llm-router**, tool runs via **tool-runtime**), then report **CompleteTask** or **FailTask** to the **task-service**. Events are written to Postgres and/or published to Redis; the user gets results via **GET /tasks/{id}**, **GET /graphs/{id}**, or by subscribing to events.

In short: **Entry** (api-gateway, goals) → **Kernel** (actors, task graph, scheduler, message bus, state) → **Workers** (execution-worker, tool-runtime, llm-router) → **Results/events** back to the user.

**Example: coding agent.** When a user asks a coding agent to "write some code for me":

1. **User → goal** — `POST /goals` or `POST /agents/{id}/goals` with goal text.
2. **Goal → planner** — Goal-service calls planner-service (LLM); planner produces a DAG of tasks (e.g. `code_generate`, `shell_exec`) and task-service persists it via `CreateGraph`.
3. **Scheduler → Redis** — Scheduler marks ready tasks, pushes to Redis task stream; execution-worker pulls from the stream, claims and runs tasks (LLM + `file_write` via tool-runtime).
4. **CompleteTask → result** — Worker reports `CompleteTask`; user gets output via `GET /tasks/{id}` or `GET /graphs/{id}`.

```
User → POST /goals → goal-service → planner (LLM) → DAG → CreateGraph
       → scheduler → ready tasks → Redis stream → execution-worker
       → runs (LLM + tools) → CompleteTask → GET /tasks/{id} or /graphs/{id}
```

See [docs/PRD.md](docs/PRD.md) for full detail.

**Request-to-result data flow:**

```
  User/API
      │
      ▼  POST /goals (JWT)
┌─────────────┐     ┌──────────────┐     ┌─────────────────┐
│ api-gateway │────▶│ goal-service │────▶│ planner-service │
└─────────────┘     └──────────────┘     └────────┬────────┘
       │                    │                    │ DAG
       │                    │                    ▼
       │                    │             ┌─────────────┐
       │                    │             │ task-service│ CreateGraph
       │                    │             └──────┬──────┘
       │                    │                    │
       │                    │                    ▼
       │                    │             ┌─────────────┐
       │                    │             │  scheduler   │ ready → queued
       │                    │             └──────┬──────┘
       │                    │                    │ XADD
       │                    │                    ▼
       │                    │             ┌─────────────┐
       │                    │             │ Redis Stream │ astra:tasks:shard:<n>
       │                    │             └──────┬──────┘
       │                    │                    │ XREADGROUP
       │                    │                    ▼
       │                    │             ┌─────────────┐     ┌──────────────┐
       │                    │             │ execution-  │────▶│ tool-runtime │
       │                    │             │ worker      │◀────│ llm-router   │
       │                    │             └──────┬──────┘     └──────────────┘
       │                    │                    │ CompleteTask / FailTask
       │                    │                    ▼
       │                    │             ┌─────────────┐     Postgres events
       │                    │             │ task-service│─────────────────────▶
       │                    │             └──────┬──────┘
       │                    │                    │
       ▼                    ▼                    ▼
  GET /tasks/{id}   GET /graphs/{id}   GET /agents/{id}/goals
```

**Task state machine:**

```
  created ──▶ pending ──▶ queued ──▶ scheduled ──▶ running ──┬──▶ completed
       │         │          │           │            │      │
       │         │          │           │            └──────┴──▶ failed ──▶ (retry → queued | dead-letter)
       │         │          │           │
       │         │          │           └── Scheduler pushes to Redis; worker claims
       │         │          └── All deps completed; scheduler marks ready
       │         └── In graph, waiting for dependencies
       └── Task created in graph
```

**Agent–worker interaction:**

```
  ┌─────────────────────────────────────────────────────────┐
  │  AGENT SIDE (create & own tasks)                         │
  │  agent-service, goal-service, planner-service,           │
  │  scheduler-service, task-service                         │
  │                                                          │
  │  CreateGoal → Plan (DAG) → CreateGraph → Mark ready       │
  │       │                                    │             │
  │       │                                    ▼             │
  │       │                            Push to Redis Stream  │
  └───────┼────────────────────────────────────┼─────────────┘
          │                                    │
          │                                    ▼
  ┌───────┼──────────────────────────────────────────────────┐
  │       │     Redis Streams (astra:tasks:shard:<n>)         │
  │       │     Consumer groups; workers XREADGROUP           │
  └───────┼────────────────────────────────────┼──────────────┘
          │                                    │
          │                                    ▼
  ┌───────┼──────────────────────────────────────────────────┐
  │  WORKER SIDE (execute tasks)                             │
  │  execution-worker, browser-worker                        │
  │  (use tool-runtime, llm-router, prompt-manager)          │
  │                                                          │
  │  Pull → Claim (lock, status scheduled→running)          │
  │    → Execute (LLM + tools) → CompleteTask / FailTask     │
  └──────────────────────────────────────────────────────────┘
```

### 4.3 Agent context and specialization

**What the agent actually does.** Agents do not execute tasks; workers do. The agent's role is **identity, context, and ownership**:

- **Own the goal** — The goal is tied to an agent; the agent is who "asked for this work" and who the result belongs to.
- **Supply context** — The agent's profile (system prompt, config, attached documents) is used when planning and when executing. The planner and workers act *on behalf of* that agent using this context.
- **Define "who"** — So the agent's job is to be the identity and policy (persona, rules, constraints) that shapes what gets planned and how it is executed; workers perform the steps.

**How a specialist agent (e.g. "Python expert") is applied.** The worker is generic; it does not "become" the expert. The agent's expertise is **carried in the task payload**:

1. When a goal is created, **goal-service** assembles the agent's context (system prompt, rules, skills) via `AssembleContext(agentID, goalID)` and passes it to the planner.
2. The **planner** embeds that context in **every task** it creates (e.g. `code_generate`, `shell_exec`) as `agent_context` in the task payload.
3. When the **execution-worker** runs a task, it reads `agent_context` from the payload, builds the prompt (system prompt + rules + task instructions), and sends it to the LLM. The LLM therefore behaves as the "Python expert" because that text is in the prompt. The worker is a generic executor; the expert is the context that travels with the task.

**Language-aware codegen:** Planner and codegen respect user-specified programming language (default Python). `detectLanguageFromTask()` for Ruby, Python, Go, Rust, Java, C#. Simple goals produce 1–3 tasks instead of 8–12.

### 4.4 Chat

**Chat subsystem:** WebSocket streaming via api-gateway; internal/chat for sessions and messages; dashboard chat widget for chat-capable agents.

- **Simple messages** → direct LLM.
- **Complex messages** → route through Goal → Agent → Worker pipeline.
- Sessions, message history, rate limits (30 msg/min), token caps (100K/session), config: `CHAT_ENABLED`, `CHAT_RATE_LIMIT`, `CHAT_TOKEN_CAP`, `CHAT_MAX_MSG_LENGTH`.

```
  Users / Dashboard
        │
        ├── REST (JWT: user_id, org_id, org_role, team_ids, is_super_admin)
        │       │
        ▼       ▼
  ┌─────────────┐     WebSocket /chat/ws
  │ api-gateway │◀─────────────────────────┐
  └──────┬──────┘                          │
         │                                 │
         ├──▶ identity (user CRUD, login)   │
         │                                 │
         ├──▶ access-control (internal/rbac: CanAccessAgent, RBAC)
         │
         └──▶ internal/chat (sessions, messages)
                    │
                    ├── Simple msg → direct LLM
                    └── Complex msg → goal-service → planner → workers
```

### 4.5 Multi-tenancy

**Organizations, teams, and users.** Identity issues JWT with org/role claims; access-control enforces RBAC and agent visibility.

- **Identity:** user CRUD, bcrypt login, enriched JWT with user_id, org_id, org_role, team_ids, is_super_admin.
- **Access-control:** internal/rbac for RBAC; agent visibility hierarchy: **global** / **public** / **team** / **private** with `CanAccessAgent`.
- **Roles:** super_admin, org_admin, org_member, team_admin, agent_admin.
- **Agent visibility:** global agents (e.g. `astra-global-*`) restricted to super admins only.
- **Dashboard:** auto-redirect to `/login` on 401/403.

See [docs/PRD.md](docs/PRD.md) §19.

### 4.6 Approvals

Two types of approval requests:

1. **Plan** — When `AUTO_APPROVE_PLANS` is false, creating a goal creates an approval request for the *implementation plan* before the task graph is created; approving it triggers goal-service to create the graph.
2. **Risky task** — When a worker tries to run a tool that policy marks as dangerous (e.g. `terraform apply`, certain `shell_exec`), the tool-runtime creates an approval request and waits; the dashboard lists these.

The approvals table shows a **Type** column (plan / risky_task) and a detail modal with type-specific content. See [docs/approval-system-extension-spec.md](docs/approval-system-extension-spec.md).

### 4.7 Dashboard

**api-gateway** at `/superadmin/dashboard/`:

- **Summary stats** — Tokens In, Tokens Out (from cost data).
- **Goal detail modal** — Actions, cancel.
- **Generated code** — Click a completed `code_generate` action to open a modal with path and content per file.
- **Pending approvals** — Type (plan / risky_task), type-specific detail modal.
- **Chat widget** — Appears for agents with `chat_capable=true`.
- **Super-admin:** Organizations tab (list/create/edit orgs, manage members), Users tab (paginated, search/filters, create/edit users, manage memberships); super-admins see redacted execution details (no code, prompts).
- **Auto-redirect** to `/login` on 401/403.

---

## Creating and Managing Agents

### Seed agents

```bash
./scripts/seed-agents.sh
```

Creates a default set of agents (Python Expert, Backend Dev, Frontend Dev, E-Commerce Builder, Generalist Coder, Documentation, DevOps, Testing, **Chat Assistant**) in one run. Requires `jq`. The script is **idempotent** (skips creation if agents already exist) and sets `chat_capable=true` for the Chat Assistant so the dashboard chat widget appears on fresh setup. Agent **names are unique** (enforced by migration 0015); seed agents use the `astra-global-*` prefix (restricted to super admins). To de-duplicate existing data without adding the constraint, run `psql "$DATABASE_URL" -f scripts/dedup-agents-by-name.sql`. After deployment, run the seed once; use `GET /agents` to list agents and submit goals to any agent ID.

### Creating custom agents via API

After deployment, create specialized agents via the API Gateway and Identity service:

- **Python Expert** — An agent that only writes Python (3.10+, PEP 8, type hints, production-ready):
  ```bash
  ./scripts/create-python-expert-agent.sh
  ```
  The script obtains a JWT from Identity, creates the agent with `POST /agents` (actor_type `python-expert`), attaches a Python-only rule document, and prints the agent ID and an example `curl` to submit a goal.

- See [docs/api/openapi.yaml](docs/api/openapi.yaml) for the full request/response schema and other agent examples (e.g. e-commerce builder).

### Agent naming and uniqueness

Agent names are unique (enforced by migration 0015). Seed agents use the `astra-global-*` prefix, restricted to super admins.

---

## Connecting to Slack

You can connect Astra chat agents to Slack so users interact with agents from Slack DMs or channels. One Slack workspace is linked to one Astra organization; tokens can be set via OAuth (“Connect Slack”) or by pasting App Configuration Tokens in the org dashboard.

### 1. Run the Slack migration

Ensure the Slack tables exist (run once):

```bash
psql "$DATABASE_URL" -f migrations/0019_slack_integration.sql
psql "$DATABASE_URL" -f migrations/0020_slack_refresh_token.sql
```

(If you use `./scripts/deploy.sh` locally, migrations run there. For GCP, `./scripts/gcp-deploy.sh` runs migrations against Cloud SQL.)

### 2. Get Slack app credentials

Create or open your app at [api.slack.com/apps](https://api.slack.com/apps).

| Field | Where in Slack |
|-------|----------------|
| **Signing Secret** | **Basic Information** → **App Credentials** → click **Show** next to Signing Secret |
| **Client ID** | **Basic Information** → **App Credentials** (or **OAuth & Permissions** → top of page) |
| **Client Secret** | **Basic Information** → **App Credentials** → click **Show** next to Client Secret |
| **OAuth Redirect URL** | You define this; add it in **OAuth & Permissions** → **Redirect URLs** (e.g. `http://localhost:8080/org/api/slack/oauth/callback` or `https://your-astra-host/org/api/slack/oauth/callback`), then **Add** and **Save URLs** |

Do not share these credentials or commit them to code.

### 3. Save credentials in Astra (Super-Admin)

1. Log in as **super-admin** and open **Super-Admin Dashboard** → **Slack** tab.
2. Enter **Signing Secret**, **Client ID**, **Client Secret**, and **OAuth Redirect URL** (the exact URL you added in Slack).
3. Click **Save**.

These are stored in the database (`slack_app_config`). The slack-adapter and api-gateway read them from there (or from env as fallback).

### 4. Start the slack-adapter

The slack-adapter receives events from Slack and enqueues them; a worker consumes the queue, calls the api-gateway chat API, and posts replies back to Slack.

```bash
# From repo root; ensure api-gateway, Postgres, and Redis are running
go run ./cmd/slack-adapter
```

Set in `.env` (or environment):

| Variable | Purpose |
|----------|---------|
| `ASTRA_SLACK_INTERNAL_SECRET` | Shared secret for slack-adapter → api-gateway internal chat endpoint. Set to a random string; same value is not stored in UI. |
| `GATEWAY_INTERNAL_URL` | URL of api-gateway (e.g. `http://localhost:8080`). |
| `SLACK_ADAPTER_PORT` | Port for the adapter (default `8095`). |
| `SLACK_SIGNING_SECRET` | Optional if you saved it in Super-Admin Slack config; otherwise set here so the adapter can verify Slack requests. |

The adapter loads the signing secret from the database at startup if `SLACK_SIGNING_SECRET` is not set.

### 5. Configure Slack: Event Subscriptions and scopes

1. **Event Subscriptions** (in your Slack app):
   - Turn **Enable Events** **On**.
   - Set **Request URL** to the URL where the slack-adapter is reachable by Slack:
     - **Local:** use a tunnel (e.g. [ngrok](https://ngrok.com)): `ngrok http 8095` → use the `https://...` URL, e.g. `https://abc123.ngrok.io/slack/events`.
     - **Hosted:** use your public base URL, e.g. `https://slack-adapter.your-domain.com/slack/events`.
   - Slack will send a `url_verification` challenge; the adapter responds automatically. Save when the URL is verified.
   - Under **Subscribe to bot events**, add e.g. `message.channels`, `message.im`, `message.groups`, `app_mention` (as needed).

2. **OAuth & Permissions**:
   - Under **Bot Token Scopes**, add at least: `chat:write`, `channels:history`, `im:history`, `app_mentions:read` (and optionally `channels:read`, `groups:read`, `users:read`).
   - Under **Redirect URLs**, ensure the same URL you entered in Astra (e.g. `http://localhost:8080/org/api/slack/oauth/callback`) is added and saved.
   - Click **Install to Workspace** (or use **App Configuration Tokens** and paste tokens in Astra as in step 7).

### 6. Connect a workspace in Astra (org dashboard)

As an **org admin**:

1. Go to **Org dashboard**: `http://localhost:8080/{your-org-slug}/dashboard`.
2. Open the **Overview** tab and scroll to **Integrations**.

Then either **A** or **B**:

**A. Connect via OAuth (recommended)**  
- Click **Connect Slack**. You’ll be redirected to Slack to authorize the app. After authorizing, Slack redirects back to Astra and the access and refresh tokens are stored automatically.
- If you see a token-format or “expected pattern” error, use **B** and provide the **Slack Workspace ID** (see below).

**B. Paste tokens manually (e.g. App Configuration Tokens)**  
- In Slack: **Basic Information** (or **OAuth & Permissions**) → **App Configuration Tokens**. Copy the **Access Token** and **Refresh Token** (and optionally the workspace ID).
- In Astra Integrations, under “Or paste tokens from Slack…”:
  - **Slack Workspace ID**: optional but **recommended** for App Configuration Tokens (find it in Slack: **Settings & administration → Workspace settings**, or in the workspace URL). If you leave it blank, Astra will try to detect it via Slack’s `auth.test`; some token types (e.g. `xoxe.xoxp-` / `xoxe-1-`) may not support that and you’ll get an error—in that case, enter the Workspace ID and save again.
  - **Access Token**: paste the access token (e.g. `xoxb-...` or `xoxe.xoxp-...`).
  - **Refresh Token**: paste the refresh token (e.g. `xoxe-1-...`).
- Click **Save tokens**.

3. Set **Default agent for Slack** in the same Integrations section (the agent that will reply to messages in Slack).

### 7. Test in Slack

- Invite the app to a channel: `/invite @YourAppName`, or open a DM with the app.
- Send a message. The flow is: Slack → slack-adapter (verify signature, enqueue) → worker (call gateway chat API, post reply) → message appears in Slack.

If the access token expires (e.g. 12-hour App Configuration Tokens), the worker will use the refresh token to obtain a new access token and retry posting the reply. Ensure **Client ID** and **Client Secret** are saved in Super-Admin Slack config so token rotation works.

### Proactive posts (agent-initiated messages)

The agent (or goal/approval flow) can **proactively post** to Slack without a prior user message—e.g. “Plan pending approval. Please review in the dashboard.”

The api-gateway exposes an internal-only endpoint:

- **`POST /internal/slack/post`**  
  - **Header:** `X-Slack-Internal-Secret` — must match `ASTRA_SLACK_INTERNAL_SECRET`.  
  - **Body (JSON):** `org_id` (required), `channel_id` (optional), `text` (required), `thread_ts` (optional).  
  - If `channel_id` is omitted, the org’s **default notification channel** is used when set (`slack_workspaces.notification_channel_id`). If both are missing, the request fails with 400.

Example:

```bash
curl -X POST http://localhost:8080/internal/slack/post \
  -H "X-Slack-Internal-Secret: $ASTRA_SLACK_INTERNAL_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"org_id":"<org-uuid>","channel_id":"C0123","text":"Test from Astra"}'
```

Omit `channel_id` to use the org’s default notification channel (if configured). Goal-service or access-control can call this when e.g. a plan is pending approval; they need the gateway base URL and `ASTRA_SLACK_INTERNAL_SECRET` in their environment.

### Troubleshooting

- **“The string did not match the expected pattern”** — Usually from Slack’s `auth.test` when Workspace ID is blank and the token type isn’t supported. **Fix:** Enter your **Slack Workspace ID** in the form and save again (we then skip `auth.test`).
- **No reply in Slack** — Ensure slack-adapter is running, Event Subscriptions Request URL is correct and verified, and the org has a **Default agent for Slack** set. Check adapter and api-gateway logs.
- **Token expired** — If you use short-lived tokens, ensure **Refresh Token** is saved and Super-Admin has **Client ID** and **Client Secret** so the adapter can refresh tokens.

Design details: [docs/slack-integration-design.md](docs/slack-integration-design.md).

### External event ingest (agents listening to external sources)

Each **agent** can be configured to listen to **one external data source** (Redis Pub/Sub, GCP Pub/Sub, or WebSocket). Different agents can use different sources. Adapters discover which agent listens to which source via an internal API, then subscribe and forward events to the ingest endpoint.

**1. Per-agent data source (who listens to what)**  
- Configure in Astra: **`PATCH /agents/{id}`** with `ingest_source_type` and `ingest_source_config`.  
- **`ingest_source_type`:** `redis_pubsub` | `gcp_pubsub` | `websocket`. Omit or set empty to clear.  
- **`ingest_source_config`:** JSON for that source, e.g. `{"channel":"events"}` (Redis), `{"project":"p","subscription":"sub"}` (GCP), `{"url":"wss://..."}` (WebSocket).  
- **Which agent listens** is stored per agent in the DB; no adapter-level default agent ID.

**2. Adapters discover bindings**  
- **`GET /internal/ingest/bindings`** (header `X-Ingest-Secret`) returns `{"bindings":[{"agent_id":"...", "ingest_source_type":"redis_pubsub", "ingest_source_config":{...}}, ...]}`.  
- Adapters call this, then for each binding subscribe to that source and **`POST /internal/ingest/event`** with that `agent_id` and the message payload.

**3. Ingest endpoint**  
- **`POST /internal/ingest/event`** — Header `X-Ingest-Secret`; body: `agent_id`, optional `message_type` (default `ExternalEvent`), optional `payload`. Events are delivered to the agent as `SendMessage`. Rate-limited per agent (default 100/min; set `ASTRA_INGEST_RATE_LIMIT` to override). If `ASTRA_INGEST_SECRET` is empty, the secret check is disabled (suitable for local dev only; in production set a strong secret and do not expose internal URLs).

**4. Optional Slack per agent**  
- **`PATCH /agents/{id}`** can set **`slack_notifications_enabled`** (boolean). When true, that agent may send messages to Slack (e.g. when the agent prompt instructs it to notify the user). The org must have Slack connected; posting uses the existing **`POST /internal/slack/post`** flow.

**How to configure each agent to connect to a data source**

Use **`PATCH /agents/{id}`** with a JWT in `Authorization: Bearer <token>`. Set **`ingest_source_type`** and **`ingest_source_config`** (and optionally **`slack_notifications_enabled`**).

1. **Get a token** (e.g. from Identity, local dev):
   ```bash
   TOKEN=$(curl -s -X POST "http://localhost:8085/tokens" \
     -H "Content-Type: application/json" \
     -d '{"subject":"developer","scopes":["admin"],"ttl_seconds":3600}' | jq -r '.token')
   ```

2. **List agents** to get IDs:
   ```bash
   curl -s -H "Authorization: Bearer $TOKEN" "http://localhost:8080/agents" | jq '.agents[] | {id, name}'
   ```

3. **Set this agent's data source** (use the agent UUID from step 2):

   - **Redis Pub/Sub** (e.g. channel `alerts`):
     ```bash
     curl -X PATCH "http://localhost:8080/agents/<agent-uuid>" \
       -H "Authorization: Bearer $TOKEN" \
       -H "Content-Type: application/json" \
       -d '{"ingest_source_type":"redis_pubsub","ingest_source_config":{"channel":"alerts"}}'
     ```

   - **GCP Pub/Sub**:
     ```bash
     curl -X PATCH "http://localhost:8080/agents/<agent-uuid>" \
       -H "Authorization: Bearer $TOKEN" \
       -H "Content-Type: application/json" \
       -d '{"ingest_source_type":"gcp_pubsub","ingest_source_config":{"project":"my-project","subscription":"my-sub"}}'
     ```

   - **WebSocket**:
     ```bash
     curl -X PATCH "http://localhost:8080/agents/<agent-uuid>" \
       -H "Authorization: Bearer $TOKEN" \
       -H "Content-Type: application/json" \
       -d '{"ingest_source_type":"websocket","ingest_source_config":{"url":"wss://events.example.com/stream"}}'
     ```

   - **Clear** (stop listening): send `{"ingest_source_type":""}`.

4. **Config meanings:**  
   | Source          | `ingest_source_type` | `ingest_source_config` example                                      |
   |-----------------|----------------------|----------------------------------------------------------------------|
   | Redis Pub/Sub   | `redis_pubsub`       | `{"channel": "alerts"}`                                              |
   | GCP Pub/Sub     | `gcp_pubsub`         | `{"project": "my-project", "subscription": "my-subscription"}`        |
   | WebSocket       | `websocket`          | `{"url": "wss://example.com/events"}`                                |

   Adapters use **GET /internal/ingest/bindings** to read these and subscribe to the right sources; they then **POST /internal/ingest/event** with the corresponding `agent_id`.

**Dashboard:** In the Super Admin dashboard, you can set these when **creating** an agent (Data source, Data source config, Slack notifications) and when **editing** an agent (click the ✎ Edit button on an agent row to open the Edit Agent modal and change data source and Slack notifications).  
- **Api-gateway `.env`:** Set **`ASTRA_INGEST_SECRET`** (shared with adapters). In production use a strong random value; empty disables the check. Optional **`ASTRA_INGEST_RATE_LIMIT`** (default 100) = max requests per agent per minute.  
- **Adapter `.env`:** `ASTRA_INGEST_URL`, `ASTRA_INGEST_SECRET`, plus source-specific vars (e.g. `EXTERNAL_REDIS_ADDR`, `GCP_PROJECT`/`GCP_SUBSCRIPTION`, `EXTERNAL_WS_URL`). Adapters use **bindings** to know which agent_id to use for each source; they do **not** set a single “default” agent.

---

## Repo Layout

| Path | Contents |
|------|----------|
| `cmd/` | Service entrypoints (api-gateway, scheduler-service, agent-service, task-service, execution-worker, etc.) |
| `internal/` | Kernel and service implementation (actors, kernel, tasks, scheduler, messaging, events, planner, chat, rbac, etc.) |
| `pkg/` | Shared packages (db, config, logger, metrics, grpc, models, otel) |
| `proto/` | Protobuf/gRPC definitions; generated Go in `proto/kernel/`, `proto/tasks/` |
| `migrations/` | Idempotent SQL migrations (0001–0018; includes approval plan type, agents unique name, chat, agent actor_type, multi-tenant) |
| `scripts/` | `deploy.sh` (local), `gcp-deploy.sh` (GKE + GCS workspace bucket), `proto-generate.sh`, `create-python-expert-agent.sh`, `seed-agents.sh`, `dedup-agents-by-name.sql` |
| `deployments/` | Helm charts and K8s manifests |
| `docs/` | PRD, design docs (approval-system, chat-agents), phase history, runbooks |

---

## Roadmap — Multi-channel (deferred)

Broader **multi-channel** access (Slack beyond current partial flow, Microsoft Teams, email bots, unified webhook ingress for arbitrary channels) is **backlogged**—not part of the current agent-platform hardening track. Today: **dashboard chat**, **ingest bindings** (Redis / GCP Pub/Sub / WebSocket), and Slack-related pieces described in [docs/slack-integration-design.md](docs/slack-integration-design.md) and PRD Phase 12. Future work will extend channel bindings and session routing; track in [docs/PRD.md](docs/PRD.md) roadmap.

---

## Documentation

- **[docs/PRD.md](docs/PRD.md)** — Single source of truth: architecture, 16 canonical services, kernel APIs, database schema, Redis streams, deployment, security, and phased roadmap (v3.0 multi-tenant).
- **API reference:** [docs/api/openapi.yaml](docs/api/openapi.yaml) — OpenAPI 3.x spec for all REST/HTTP APIs (agents, goals, tasks, graphs, identity, access-control, chat, and internal services), with example flows; when adding or changing endpoints, update this spec and regenerate clients if used.
- **Design & deployment:** [docs/local-deployment.md](docs/local-deployment.md), [docs/deployment-design.md](docs/deployment-design.md), [docs/phase-history-usage-audit-design.md](docs/phase-history-usage-audit-design.md).
- **Multi-tenancy:** [docs/PRD.md](docs/PRD.md) §19 — organizations, teams, users, RBAC, agent visibility hierarchy, super-admin.
- **Approval system:** [docs/approval-system-extension-spec.md](docs/approval-system-extension-spec.md) — plan vs risky-task approvals, `AUTO_APPROVE_PLANS`, dashboard integration.
- **Chat agents:** [docs/chat-agents-design.md](docs/chat-agents-design.md) — WebSocket chat, streaming, sessions; [docs/chat-agents-implementation-plan.md](docs/chat-agents-implementation-plan.md) — implementation phases.
- **Slack integration:** See **Connecting to Slack** above for step-by-step setup; [docs/slack-integration-design.md](docs/slack-integration-design.md) for architecture and token rotation.
- **Phase history:** [docs/phase-history/](docs/phase-history/) — what was built in each implementation phase (e.g. [phase-0.md](docs/phase-history/phase-0.md)).

---

## Development

### Build

```bash
go build ./...
```

### Proto codegen

```bash
./scripts/proto-generate.sh
```

See [docs/codegen.md](docs/codegen.md).

### CI

CI runs `go vet`, `golangci-lint`, tests, and build. See [.github/workflows/ci.yml](.github/workflows/ci.yml).

### Cursor rules and engineering standards

Implementation follows [docs/PRD.md](docs/PRD.md). All features, APIs, schema, and services are specified there; do not add behavior that isn't in the PRD without updating it. **Cursor rules** in `.cursor/rules/` enforce PRD alignment, security (S1–S6), performance (10 ms reads, 50 ms scheduling), delegation, and PRD currency. Contributions should respect these rules. **Deployment** (including `scripts/deploy.sh`) is intended to be run by the DevOps agent or operator; see [.cursor/commands/deploy.md](.cursor/commands/deploy.md) and [.cursor/skills/devops-deployment/SKILL.md](.cursor/skills/devops-deployment/SKILL.md) for automation.

---

## Changelog

- **PR #1** — Agent actions (enable/disable/delete), PRD v2.1, chat agents design docs.
- **PR #2** — Chat agents: WebSocket streaming, sessions, tools (all 12 phases). Migration 0016: chat_sessions, chat_messages, agents.chat_capable. WebSocket handler with JWT auth and frame types (chunk, tool_call, tool_result, message_end, done). REST: POST/GET /chat/sessions, messages CRUD. Dashboard chat UI: sessions list, message panel, new chat modal, floating chat widget. Memory context, rate limits (30 msg/min), token caps (100K/session). Config: CHAT_ENABLED, CHAT_RATE_LIMIT, CHAT_TOKEN_CAP, CHAT_MAX_MSG_LENGTH.
- **PR #3** — Cancel goals/tasks on dashboard. POST /api/dashboard/goals/{id}/cancel and /tasks/{id}/cancel. Chat routes complex messages through Goal→Agent→Worker pipeline; simple messages go direct to LLM. Auto-approve goals from chat. Agent name/type separation (migration 0017). Seed agents renamed to astra-global-*, added chat-assistant agent.
- **PR #4** — GitVersion with SemVer mainline mode.
- **PR #5** — Multi-tenant architecture (PRD v3.0). Migration 0018: users, organizations, org_memberships, teams, team_memberships, agent_collaborators, agent_admins; ALTER agents/goals/tasks/workers/events/memories with org_id. Identity: user CRUD, bcrypt login, enriched JWT (user_id, org_id, org_role, team_ids, is_super_admin). RBAC engine (internal/rbac): super_admin, org_admin, org_member, team_admin, agent_admin. Agent visibility: global/public/team/private with CanAccessAgent. Super-admin data redaction, dashboard: Organizations tab, Users tab. GKE Helm values, Dockerfile Go 1.25.
- **PR #6** — Pass multi-tenant claims (org_id, org_role, is_super_admin) to access-control check; dashboard auto-redirect to /login on 401/403.
- **PR #7** — Restrict astra-global- agent prefix to super admins only.
- **PR #8** — Language-aware codegen: planner and codegen respect user-specified programming language (default Python). detectLanguageFromTask() for Ruby, Python, Go, Rust, Java, C#. Simple goals produce 1–3 tasks instead of 8–12.
- **Seed (abc0049)** — Seed script sets chat_capable=true for Chat Assistant so dashboard chat widget appears on fresh setup.
