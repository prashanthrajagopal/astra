# Approval System Extension: Two Approval Types and Dashboard Behavior

**Status:** Design spec for implementation  
**Audience:** Tech Lead, Go Engineer, UI/UX (dashboard)  
**References:** Existing flow in `cmd/tool-runtime`, `cmd/access-control`, dashboard (`cmd/api-gateway/dashboard`), migration `0012_approval_requests.sql`, PRD S6 (approval gates).

---

## 1. Overview

Extend Astra’s approval system to support **two approval types** and consistent dashboard behavior:

| Type | Meaning | Created by | When |
|------|--------|------------|------|
| **Plan** | Approve the task DAG/implementation plan before any execution | goal-service | After planner produces DAG, before tasks are created in DB |
| **Risky task** | Approve a specific dangerous tool run (existing) | tool-runtime | When a worker requests execution of a policy-marked dangerous tool |

Both appear in a single **Pending Approvals** list with a **Type** column; row click opens a **dialog** whose content depends on type. **Plan** approvals support an **auto-approve** override so they never show in the list when enabled.

---

## 2. When Plan Approvals Are Created

**Point in flow:** After the planner returns a DAG and **before** `taskStore.CreateGraph()` is called. Tasks are **not** written to the DB until the plan is approved (or auto-approved).

**Service that creates the plan approval request:** **goal-service**.

**Current flow (goal-service POST /goals):**

1. Insert goal (status `pending`).
2. Insert phase_run, emit PhaseStarted.
3. Call `p.Plan()` → get `graph`.
4. Call `taskStore.CreateGraph(ctx, &graph)` → tasks in DB, root tasks become `pending`, scheduler can pick them up.
5. Update goal to `active`.

**New flow when plan approval is required and auto-approve is off:**

1. Insert goal (status `pending`).
2. Insert phase_run, emit PhaseStarted.
3. Call `p.Plan()` → get `graph`.
4. **Check “auto-approve implementation plans”** (see §6). If **on**: go to step 5 (current behavior). If **off**: create an approval request with `request_type = 'plan'`, store serialized plan in `plan_payload` (see §3), return HTTP 202 with `goal_id`, `approval_request_id`, and a clear message that the plan is pending approval; **do not** call `CreateGraph()` and **do not** set goal to `active`. Stop.
5. Call `taskStore.CreateGraph(ctx, &graph)`.
6. Update goal to `active`.
7. Return 201 with `goal_id`, `graph_id`, `task_count`, etc.

When the user **approves** a plan approval, the system applies the plan (see §5): create the graph from the stored payload, then set goal to `active`.

---

## 3. Data Model

**Table:** `approval_requests` (extend existing; migration required).

### 3.1 New / changed columns

| Column | Type | Purpose |
|--------|------|---------|
| `request_type` | TEXT NOT NULL DEFAULT `'risky_task'` | One of `'plan'`, `'risky_task'`. |
| `goal_id` | UUID NULL REFERENCES goals(id) | Set for `request_type = 'plan'`; NULL for risky_task. |
| `graph_id` | UUID NULL | Set for `request_type = 'plan'` (graph to create on approve); NULL for risky_task. |
| `plan_payload` | JSONB NULL | For `request_type = 'plan'` only: serialized plan (see below). NULL for risky_task. |

**Existing columns** for `request_type = 'risky_task'` remain: `task_id`, `worker_id`, `tool_name`, `action_summary`. For `request_type = 'plan'`, `task_id` and `worker_id` are NULL.

**Constraint:** Add check: `request_type IN ('plan', 'risky_task')`.

**Index:** Add `idx_approval_requests_request_type` on `(request_type)` (optional but useful for filtering).

### 3.2 `plan_payload` JSON shape (for type = plan)

Enough to recreate the graph and update the goal. Example:

```json
{
  "goal_id": "uuid",
  "graph_id": "uuid",
  "agent_id": "uuid",
  "goal_text": "string",
  "tasks": [
    {
      "id": "uuid",
      "type": "code_generate",
      "payload": { "description": "...", "instructions": "...", "workspace": "...", ... },
      "priority": 100,
      "max_retries": 3
    }
  ],
  "dependencies": [
    { "task_id": "uuid", "depends_on": "uuid" }
  ]
}
```

All fields required to call `taskStore.CreateGraph()` with a reconstructed `tasks.Graph` (and to show goal_text/summary in the UI) must be in `plan_payload`. IDs for tasks and dependencies are those produced by the planner so the graph is identical when applied.

### 3.3 Backward compatibility

- Existing rows: treat as `request_type = 'risky_task'` (default). Migration sets `request_type = 'risky_task'` where NULL.
- tool-runtime: when inserting a risky approval, set `request_type = 'risky_task'` explicitly; `goal_id`, `graph_id`, `plan_payload` NULL.

---

## 4. APIs

### 4.1 GET /approvals/pending (access-control)

**Existing:** Returns list of pending approval requests.

**Change:**

- SELECT must include `request_type`, `goal_id`, `graph_id`, and for list purposes a **short summary** for plans (e.g. first 200 chars of `goal_text` or a dedicated `plan_summary` field in `plan_payload` if desired). Include `plan_payload` only if the list response is used for the dialog; otherwise keep list lean and use GET by id for full details (see below).
- **Recommendation:** List response includes `request_type`, `goal_id`, `graph_id`, and a small `summary` or `goal_text` snippet for type=plan; full `plan_payload` only in GET by id.

Response shape (each item) — extend existing:

- `request_type`: `"plan"` | `"risky_task"`.
- For risky_task: existing fields (`id`, `task_id`, `worker_id`, `tool_name`, `action_summary`, `status`, `requested_at`, …).
- For plan: `id`, `goal_id`, `graph_id`, `request_type`, `status`, `requested_at`, and either a short `summary`/`goal_text` or the full `plan_payload` (product choice: list vs one extra GET).

### 4.2 GET /approvals/{id} (access-control) — NEW

- **Purpose:** Full details for the approval request so the dashboard can render the dialog (plan vs risky task).
- **Returns:** Single approval request by id with all fields:
  - Common: `id`, `request_type`, `status`, `requested_at`, `decided_at`, `decided_by`.
  - If `request_type = 'risky_task'`: `task_id`, `worker_id`, `tool_name`, `action_summary`.
  - If `request_type = 'plan'`: `goal_id`, `graph_id`, `plan_payload` (full), and optionally a short `goal_text` at top level for convenience.
- **404** if not found.

Dashboard: on row click, call GET /approvals/{id}, then render dialog from the payload (plan vs risky).

### 4.3 POST /approvals/{id}/approve and POST /approvals/{id}/deny (access-control)

**Existing:** Set `status` to `approved` or `denied`, set `decided_at`, `decided_by`.

**Change:**

- For `request_type = 'plan'` and status `approved`: after updating the row, **invoke “apply plan”** (see §5) so the graph is created and the goal is activated. Option A: access-control calls goal-service `POST /internal/apply-plan` with `{ "approval_id": "..." }`. Option B: api-gateway dashboard approve handler, after proxying to access-control, calls goal-service apply-plan when the approval response indicates type=plan. Option A keeps “apply” logic triggered in one place (access-control).
- For `request_type = 'risky_task'`, behavior unchanged (only status update).
- For `request_type = 'plan'` and status `denied`: only update status; optionally goal-service could mark the goal as failed/cancelled (optional; can be a follow-up).

### 4.4 POST /goals (goal-service)

**When plan approval is required and auto-approve is off:**

- After `p.Plan()` succeeds, create one row in `approval_requests` with `request_type = 'plan'`, `goal_id`, `graph_id`, `plan_payload` = serialized graph (and goal_text/agent_id as in §3.2), `status = 'pending'`.
- Return **202 Accepted** with body e.g. `{ "goal_id": "...", "approval_request_id": "...", "message": "Plan pending approval" }` (and optionally `graph_id`). Do **not** call `CreateGraph()` and do **not** set goal to `active`.

**When plan approval is not required (auto-approve on):**

- Behave as today: call `CreateGraph()`, set goal to `active`, return 201 with `goal_id`, `graph_id`, `task_count`, etc.

### 4.5 POST /internal/apply-plan (goal-service) — NEW

- **Caller:** access-control (or api-gateway) after approving a plan approval request.
- **Body:** `{ "approval_id": "uuid" }`.
- **Behavior:**
  1. Load approval request by id (goal-service must have read access to `approval_requests`, e.g. same Postgres).
  2. Validate `request_type = 'plan'` and `status = 'approved'`.
  3. Deserialize `plan_payload` into a `tasks.Graph` (same shape as planner output).
  4. Call `taskStore.CreateGraph(ctx, &graph)`.
  5. Update goal to `active`: `UPDATE goals SET status = 'active' WHERE id = $1` (goal_id from plan_payload).
  6. Return 200. On failure (e.g. graph already applied, invalid payload), return 4xx and do not change goal.
- **Idempotency:** If the goal is already `active` and tasks for that graph_id already exist, return 200 without re-inserting (or 409 if product prefers).

---

## 5. Backend Flow for Plan Approval

1. **Creation:** goal-service POST /goals (see §2). If plan approval required and auto-approve off → insert `approval_requests` (type=plan, plan_payload, goal_id, graph_id), return 202; no CreateGraph.
2. **Auto-approve check:** In goal-service, before creating a plan approval, read the auto-approve setting (env or, if implemented, a settings API). If auto-approve plans is on, skip creating the approval and call CreateGraph + set goal active (current behavior).
3. **User approves (dashboard):** User clicks Approve on a plan row → dashboard calls POST /api/dashboard/approvals/{id}/approve → api-gateway proxies to access-control POST /approvals/{id}/approve → access-control updates row to `approved`, then, if `request_type = 'plan'`, calls goal-service POST /internal/apply-plan with that id → goal-service loads approval, applies plan (CreateGraph + set goal active).
4. **Scheduler:** No change. Once CreateGraph is called (either immediately when auto-approved or after apply-plan), tasks exist and root tasks are `pending`; scheduler’s FindReadyTasks picks them up as today.

---

## 6. Auto-Approve for Plans

**Requirement:** When “auto-approve implementation plans” is on, plan approvals are never created; plans are applied immediately and never appear in Pending Approvals.

**Options:**

1. **Env var (minimal):** goal-service reads `AUTO_APPROVE_PLANS` (default `false`). If `true`, after Plan() always call CreateGraph and set goal active; never create a plan approval request.
2. **Dashboard toggle + backend:** A setting (e.g. in a `settings` or `user_preferences` table, or a small key-value store) key `auto_approve_plans`. Dashboard shows a toggle; changing it calls e.g. POST/GET /api/settings or /api/dashboard/settings. goal-service, when handling POST /goals, consults this setting (same DB or HTTP to api-gateway) to decide whether to create a plan approval or proceed as in (1).

**Recommendation:** Implement (1) first; add (2) as a follow-up so the dashboard can turn auto-approve on/off without redeploy. If (2) is implemented, persistence should be in the backend (DB or config service), not only localStorage, so goal-service can read it.

**Placement of toggle in UI:** Dashboard header or a small “Settings” / “Approvals” section near the Pending Approvals card. Label: e.g. “Auto-approve implementation plans”. If stored in backend, the dashboard GET snapshot (or a dedicated GET settings) should return `auto_approve_plans: boolean` so the toggle can reflect current state.

---

## 7. Dashboard UI

### 7.1 Pending Approvals table

- **Add column “Type”:** Values “Plan” or “Risky task” from `request_type`. Displayed so the user can tell which is which at a glance.
- **Columns (revised):** e.g. **Type** | ID (or truncated) | Tool / Goal summary | Action summary / Plan summary | Status | Requested at | Actions.
  - For Plan: “Tool” can be “—”; use goal_text snippet or “Implementation plan” in summary.
  - For Risky task: keep current tool_name and action_summary.
- **Row click:** Clicking a row (or an explicit “View” control) opens a **dialog/modal**; do not trigger approve/reject on row click. Approve/Reject stay in the dialog (and optionally still in the table row).

### 7.2 Approval detail dialog (modal)

- **Open:** When user clicks a row in Pending Approvals (or “View” on that row). Dashboard calls GET /api/dashboard/approvals/{id} (api-gateway proxies to access-control GET /approvals/{id}) to get full details.
- **Content by type:**
  - **Risky task:** Show tool name, action summary, task_id, worker_id, requested_at, and any other fields from the approval request. Buttons: Approve, Reject.
  - **Plan:** Show goal_id, graph_id, goal_text, and a list of tasks in the DAG (e.g. type + description from plan_payload.tasks). Optionally a short summary line. Buttons: Approve, Reject.
- **Behavior:** Approve/Reject in the dialog call the existing POST approve/reject endpoints, then close the dialog and refresh the pending list (and optionally the goals/task counts).

### 7.3 Auto-approve plans toggle (if implemented)

- **Where:** Dashboard header or a small settings/approvals section (e.g. above or beside the Pending Approvals table).
- **Persistence:** Prefer backend (GET/POST /api/dashboard/settings or similar) so goal-service can read it; if only front-end for first cut, localStorage is acceptable with a note that goal-service will only respect env var until backend setting exists.
- **Label:** e.g. “Auto-approve implementation plans”. When on, plan approvals are never created (goal-service checks this before creating a plan approval).

---

## 8. API Gateway (dashboard routes)

- **GET /api/dashboard/approvals/{id}:** New. Proxy to access-control GET /approvals/{id}. Used by the dashboard to fetch full approval details for the dialog.
- **POST /api/dashboard/approvals/{id}/approve** and **/reject:** Existing; no change to request/response. If apply-plan is triggered from access-control after approve, no change needed here; if product prefers api-gateway to call apply-plan after a plan approval, then after proxy success, if response or a follow-up GET indicates type=plan, call goal-service apply-plan (api-gateway must know goal-service address).

---

## 9. Tool-Runtime (risky task) Changes

- When inserting into `approval_requests`, set `request_type = 'risky_task'` (and leave `goal_id`, `graph_id`, `plan_payload` NULL). Migration default handles existing rows.

---

## 10. Access-Control Summary of Code Changes

- **GET /approvals/pending:** Include `request_type`, and for type=plan include `goal_id`, `graph_id`, and a brief summary (e.g. goal_text truncation or from plan_payload).
- **GET /approvals/{id}:** New handler; return full row including `plan_payload` for type=plan.
- **POST /approvals/{id}/approve:** After updating status to `approved`, if `request_type = 'plan'`, call goal-service POST /internal/apply-plan with this approval id. Use a configured goal-service URL (e.g. env GOAL_SERVICE_ADDR or existing config).

---

## 11. Security and Compliance (S6)

- Approval gates remain human-in-the-loop for both plan and risky tool execution.
- Plan approval: human approves the DAG before any task is created or run.
- Risky task: unchanged; dangerous tool runs still require explicit approve/deny.
- Auto-approve plans is an explicit override (config or dashboard); document as an operational choice.

---

## 12. Implementation Checklist (for Tech Lead)

- [ ] **DB (DB Architect):** Migration adding `request_type`, `goal_id`, `graph_id`, `plan_payload` to `approval_requests`; backfill and constraint.
- [ ] **goal-service:** Check auto-approve (env first); create plan approval and return 202 when required; implement POST /internal/apply-plan; read approval and apply plan on approve.
- [ ] **access-control:** Extend GET pending with type and plan summary; add GET /approvals/{id}; in approve handler, call goal-service apply-plan for type=plan.
- [ ] **tool-runtime:** Set request_type='risky_task' on insert.
- [ ] **api-gateway:** Add GET /api/dashboard/approvals/{id} proxy; optional: settings endpoint for auto_approve_plans if dashboard toggle is backed by backend.
- [ ] **Dashboard:** Table column Type; row click → fetch GET approval by id → open dialog (plan vs risky content); Approve/Reject in dialog; optional: auto-approve plans toggle and persistence.

---

## 13. References

- Existing approval flow: `cmd/tool-runtime/main.go` (check, insertPending), `cmd/access-control/main.go` (handlePending, handleApprove, handleDeny), `internal/dashboard/snapshot.go` (collectApprovals), dashboard `app.js` (renderApprovals, submitApprovalAction).
- Goal flow: `cmd/goal-service/main.go` (POST /goals → Plan → CreateGraph).
- Schema: `migrations/0012_approval_requests.sql`.
- PRD: S6 approval gates; 16 canonical services (goal-service, access-control, api-gateway).
