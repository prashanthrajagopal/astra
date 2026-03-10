# Design Spec: List Agents and List Workers HTTP APIs

**Status:** Spec only (no Go implementation).  
**Source of truth:** `docs/PRD.md`, `agents` table (Migration 0001, 0013), `workers` table (Migration 0005), worker-manager (Phase 2), agent-service (Phase 1).

---

## 1. List All Agents

### 1.1 Owner and exposure

- **Owner:** API Gateway (public-facing).
- **Backend:** API Gateway proxies to **agent-service**. Agent-service is the canonical owner of agent lifecycle and kernel actor integration (PRD §9); it (or the kernel) has access to the `agents` table and/or kernel `QueryState(entity_type="agent")`.
- **Rationale:** All external control-plane reads go through the gateway with JWT (PRD §18, S2). A single `GET /agents` on the gateway keeps one entry point for clients.

### 1.2 Path, method, and behavior

- **Path:** `GET /agents`
- **Method:** GET
- **Server:** API Gateway (e.g. `http://localhost:8080`)

Behavior: Return a list of all agents (summary fields only). No query parameters required for MVP; optional filters (e.g. `?status=active`) can be added later.

### 1.3 Request

- No body.
- Optional query parameters (future): `status`, `limit`, `offset` (not in initial scope).

### 1.4 Response shape

- **Content-Type:** `application/json`
- **200 OK:** JSON object with an array of agent summaries.

Schema (aligned with PRD §11 `agents` table and agent-service/kernel):

| Field       | Type   | Source / notes |
|------------|--------|-----------------|
| `id`       | string (UUID) | `agents.id` (actor identifier) |
| `actor_type` | string | From spawn/create request; agent-service may store in `config` or dedicated field |
| `name`     | string | `agents.name` |
| `status`   | string | `agents.status` (`active`, `stopped`, `error` per PRD Migration 0008) |
| `created_at` | string (RFC3339) | `agents.created_at` |
| `updated_at` | string (RFC3339) | `agents.updated_at` |

Optional for v1: `config` (JSON) if useful for clients. Omit large fields (e.g. `system_prompt`) from list; use `GET /agents/{id}/profile` for full profile.

Example response:

```json
{
  "agents": [
    {
      "id": "a3ed7085-1b3d-4818-90fa-b54616a45721",
      "actor_type": "ecommerce-builder",
      "name": "E-Commerce Builder",
      "status": "active",
      "created_at": "2026-03-10T12:00:00Z",
      "updated_at": "2026-03-10T14:30:00Z"
    }
  ]
}
```

### 1.5 Auth

- **JWT via API Gateway** (PRD §18, S2). Request must include `Authorization: Bearer <token>`. Gateway validates JWT and enforces access-control before proxying to agent-service.
- **401** if missing or invalid token.

### 1.6 Gateway route / handler

- **New route:** `GET /agents` on api-gateway.
- **Handler:** Validate JWT → optional access-control check for `list:agents` (or equivalent) → call agent-service (new ListAgents RPC or HTTP `GET /agents` on agent-service) → return response.
- **Agent-service:** Must expose a way to list agents (e.g. query `agents` table or kernel `QueryState` with `entity_type=agent`). If agent-service currently has no list API, add an internal HTTP `GET /agents` or gRPC `ListAgents` that returns the above shape; gateway proxies to it.

### 1.7 PRD alignment and update

- PRD §9 defines agent-service and api-gateway; §11 defines `agents` table. **PRD does not currently define `GET /agents` (list all).** This spec adds it. **Action:** Update PRD §9 (Agent Profile & Documents API table or a new “Control-plane REST” subsection) to add: `GET /agents` — List all agents (gateway, JWT). Response: array of agent summaries (`id`, `actor_type`, `name`, `status`, `created_at`, `updated_at`).

---

## 2. List All Workers

### 2.1 Owner and exposure

- **Owner (external):** API Gateway (public-facing).
- **Backend:** API Gateway proxies to **worker-manager**. Worker-manager already exposes `GET /workers` on port 8082 (PRD Phase 2, `cmd/worker-manager`); it uses `internal/workers.Registry` and the `workers` table (PRD §11 Migration 0005).
- **Rationale:** External clients get a single, JWT-protected entry point at the gateway. Internal callers (e.g. dashboard snapshot) continue to call worker-manager directly at 8082.

### 2.2 Path, method, and behavior

- **Path:** `GET /workers`
- **Method:** GET
- **Server (external):** API Gateway (e.g. `http://localhost:8080`)
- **Server (internal):** Worker Manager (e.g. `http://localhost:8082`) — existing endpoint, unchanged.

Behavior: Return all workers from the worker registry (id, hostname, status, capabilities, last_heartbeat). Gateway proxies to worker-manager `GET /workers`.

### 2.3 Request

- No body.
- Optional query parameters (future): `status`, `capability` (not in initial scope).

### 2.4 Response shape

- **Content-Type:** `application/json`
- **200 OK:** JSON array of worker objects (or object with `workers` key; align with worker-manager’s current response).

Schema (aligned with PRD §11 `workers` table and `internal/workers.WorkerInfo`):

| Field            | Type              | Source / notes |
|------------------|-------------------|----------------|
| `id`             | string (UUID)     | `workers.id` |
| `hostname`       | string            | `workers.hostname` |
| `status`         | string            | `workers.status` (`active`, `draining`, `offline` per Migration 0008) |
| `capabilities`   | array of strings  | `workers.capabilities` (JSONB) |
| `last_heartbeat`| string (RFC3339)  | `workers.last_heartbeat` |
| `created_at`     | string (RFC3339)  | `workers.created_at` (optional in response) |

Example response:

```json
{
  "workers": [
    {
      "id": "b4fe8196-2c4e-4929-a1gb-c65727b56832",
      "hostname": "mac-mini.local",
      "status": "active",
      "capabilities": ["general", "code_generate", "shell_exec"],
      "last_heartbeat": "2026-03-10T14:35:00Z"
    }
  ]
}
```

(If worker-manager currently returns a bare array, gateway may wrap it in `{"workers": [...]}` for consistency with `GET /agents`, or leave as-is; spec should match implementation.)

### 2.5 Auth

- **External (via gateway):** JWT via API Gateway. `Authorization: Bearer <token>`. **401** if missing or invalid.
- **Internal (worker-manager 8082):** No JWT; used by gateway and dashboard over internal network (mTLS in production per S1).

### 2.6 Gateway route / handler

- **New route:** `GET /workers` on api-gateway.
- **Handler:** Validate JWT → optional access-control check for `list:workers` → HTTP GET to worker-manager `GET /workers` (e.g. `WorkerManagerAddr + "/workers"`) → return response (optionally normalize to `{ "workers": [...] }` if backend returns array).

### 2.7 PRD alignment and update

- PRD Phase 2 already defines worker-manager with `GET /workers` and the worker registry. **PRD does not currently define a gateway proxy for listing workers.** This spec adds the gateway path. **Action:** Update PRD §9 (or the REST API section) to add: `GET /workers` — List all workers (gateway, JWT). Proxies to worker-manager. Response: array of worker objects (`id`, `hostname`, `status`, `capabilities`, `last_heartbeat`).

---

## 3. OpenAPI (Swagger) placement

- **List agents:** Add `GET /agents` under **Gateway — Agents** in `docs/api/openapi.yaml`, server API Gateway (8080), security `BearerAuth`, request/response examples and schema as above.
- **List workers:** Add or update **Gateway — Workers** (new tag) and document `GET /workers` on the API Gateway (8080) with `BearerAuth` and the same response shape. The existing Worker Manager `GET /workers` (port 8082) can remain documented as the internal endpoint; in the spec description, state that the gateway proxies to worker-manager.

---

## 4. Security compliance (design-time)

| Constraint | List agents | List workers |
|------------|-------------|--------------|
| S2 (JWT auth) | PASS — gateway enforces JWT | PASS — gateway enforces JWT |
| S1 (mTLS)     | N/A (external client → gateway); gateway → agent-service over mTLS | N/A (external → gateway); gateway → worker-manager over mTLS |
| S3 (RBAC/OPA) | Optional: gateway calls access-control for `list:agents` | Optional: gateway calls access-control for `list:workers` |

---

## 5. Implementation checklist (for developers)

- [ ] **Agent-service:** Implement list-agents (DB query on `agents` and/or kernel QueryState); expose HTTP `GET /agents` or gRPC `ListAgents` used by gateway.
- [ ] **API Gateway:** Add `GET /agents` route; JWT + proxy to agent-service; return `{ "agents": [...] }`.
- [ ] **API Gateway:** Add `GET /workers` route; JWT + proxy to worker-manager `GET /workers`; return response (same shape as worker-manager or normalized).
- [ ] **OpenAPI:** Add both paths under gateway with schemas and BearerAuth (see `docs/api/openapi.yaml`).
- [ ] **PRD:** Update `docs/PRD.md` with `GET /agents` and `GET /workers` in the REST API table / §9.
