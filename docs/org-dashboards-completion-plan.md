# Org Dashboards Completion Plan

**Status:** **OBSOLETE** — Multi-tenant (LTI) feature has been removed. Astra is single-platform only; there are no organizations, teams, or org-level dashboards. This plan is retained for historical reference only.

**Audience:** Project Manager, Architect, Tech Lead.  
**References:** `docs/PRD.md`, `cmd/api-gateway/main.go`, `cmd/api-gateway/dashboard/`.

---

## 1. Current State

### 1.1 URL and routing (today)

| Pattern | Handler | Notes |
|--------|---------|--------|
| `/superadmin/dashboard/` | File server (dashboard/*) | Full UI: Overview, Organizations, Users. |
| `/dashboard/`, `/dashboard` | Redirect → `/superadmin/dashboard/` | Legacy alias. |
| `/login` | `handleLoginPage` | Platform login; redirects to super-admin dashboard. |
| `/{org-slug}` or `/{org-slug}/` | `handleOrgLoginPage` | Org login page (template in main.go). |
| `POST /{org-slug}/login` | `handleOrgLoginPost` | Org login; JWT stored in localStorage. |
| `GET /{org-slug}/dashboard` | `handleOrgDashboardPage` | Org dashboard page (inline HTML in main.go). |
| `GET /org/api/teams`, `POST /org/api/teams`, … | `registerMultiTenantRoutes` | Org-level API; `org_id` via header or query. |
| `GET /org/`, `GET /org/dashboard` | **Not registered** | `org` is in `reservedPaths`, so these hit mux but have no handler → 404. |

Org UX today: user goes to `/{org-slug}` → login → `/{org-slug}/dashboard`. There is no `/org/` or `/org/dashboard` route.

### 1.2 Super-admin dashboard

- **Location:** `cmd/api-gateway/dashboard/` (index.html, app.js, style.css, openapi.yaml).
- **Served at:** `/superadmin/dashboard/`.
- **Features:** Overview (summary cards, task/goal/service/agent charts, agents table, recent goals, services, workers, approvals, cost, logs, PIDs), Organizations (CRUD, add org admins), Users (paginated table, search/filter, detail modal, suspend/activate/reset-password/role/move-org).
- **APIs:** `/superadmin/api/dashboard/snapshot`, goals, approvals, settings, agents, chat, etc. All require super_admin JWT.

### 1.3 Org dashboard (today)

- **Location:** Inline HTML/JS in `cmd/api-gateway/main.go` (`orgDashboardHTML`).
- **Served at:** `GET /{org-slug}/dashboard` only (no `/org/dashboard`).
- **Implemented:**
  - Header with org name/slug, sign out → `/{org-slug}`.
  - Tabs: Overview, Agents, Goals, Teams, Members.
  - **Overview:** Cards for Agents, Teams, Members, Goals. Agents/Teams/Members call `/agents`, `/org/api/teams`, `/org/api/members` (with `org_id`). Goals card is static "—".
  - **Agents:** Table from `GET /agents` (no org filter in backend today — shows all agents).
  - **Goals:** Placeholder text: "Org-scoped goal listing coming soon" + link to super-admin dashboard.
  - **Teams:** List + Create Team (GET/POST `/org/api/teams`). No edit/delete in UI; API supports PATCH/DELETE.
  - **Members:** List + Invite Member (GET/POST `/org/api/members`). No role change/remove in UI; API supports PATCH/DELETE.
- **Not implemented (PRD org dashboard):** Org-scoped goals (list + full detail: goal_text, task payloads, execution results, code), Workers, Approvals, Cost, agent create/edit/visibility/collaborators/admins, member role change/remove, team edit/delete/members, org_admin-only access enforcement in UI.

### 1.4 Org home (PRD)

- **PRD:** `/org/` — all org members: agents they can access (filtered by visibility), recent goals, quick-submit goal form, chat.
- **Today:** No `/org/` or `/{org-slug}/home` page. Only `/{org-slug}` (login) and `/{org-slug}/dashboard` (admin-oriented dashboard). No dedicated “org home” for general members.

### 1.5 Org-level APIs (existing)

- **Teams:** GET/POST `/org/api/teams`, PATCH/DELETE `/org/api/teams/{id}`, POST/DELETE `/org/api/teams/{id}/members`.
- **Members:** GET/POST `/org/api/members`, PATCH/DELETE `/org/api/members/{uid}`.
- **Agents:** POST `/org/api/agents`, POST/GET/DELETE `/org/api/agents/{id}/collaborators`, POST/DELETE `/org/api/agents/{id}/admins`.
- **Org context:** `extractOrgID(r)` from `X-Org-Id` header or `org_id` query. JWT must carry org context; dashboard passes `org_id` in query.

Missing for dashboards: org-scoped **snapshot** (or equivalent), org-scoped **goals** list and goal detail, org-scoped **cost**, org-scoped **approvals** (and optionally workers). GET `/agents` is not explicitly org-scoped in the gateway; agent-service must filter by org when JWT has org_id.

---

## 2. PRD Specification (target)

| Item | PRD reference |
|------|----------------|
| **Org Home** (`/org/`) | All org members. Agents (visibility-filtered), recent goals, quick-submit goal form, chat. |
| **Org Dashboard** (`/org/dashboard`) | Org-admin only. Members (list, invite, change roles, remove); Teams (create, manage membership); Agents (full list, create, edit visibility, collaborators/admins); Goals (full detail: goal_text, task payloads, execution results, code); Workers; Approvals; Cost (org-scoped). |
| **Auth** | Org home: org JWT. Org dashboard: org_admin JWT. |
| **URL table** | `/org/` = org home, `/org/dashboard` = org admin dashboard. |

PRD does not use `/{org-slug}` in the URL table; it uses `/org/` and `/org/dashboard`, implying org is taken from JWT/session. Context message also describes org-specific routes as `/{org-slug}`, `/{org-slug}/login`, `/{org-slug}/dashboard`, so both conventions exist and need alignment.

---

## 3. Gaps: Missing or Incomplete Org Dashboard Features

### 3.1 URL and routing alignment

- **Gap:** PRD documents `/org/` and `/org/dashboard`; implementation uses `/{org-slug}/dashboard` (and no org home).
- **Options:** (A) Add `/org/` and `/org/dashboard` as routes that resolve org from JWT (and optionally redirect to `/{org-slug}` if not logged in). (B) Keep only `/{org-slug}` and `/{org-slug}/dashboard` and update PRD to match. (C) Support both: `/org/` and `/org/dashboard` when logged in with org context; `/{org-slug}` and `/{org-slug}/dashboard` for direct link and login flow.
- **Recommendation:** (C) — Keep `/{org-slug}` and `/{org-slug}/dashboard` for login and deep links; add `GET /org/` and `GET /org/dashboard` that require org JWT and redirect to `/{org-slug}` or `/{org-slug}/dashboard` when org is known (from JWT). Document in PRD that both patterns are supported.

### 3.2 Org home (all members)

| # | Feature | Acceptance criteria | Priority |
|---|--------|---------------------|----------|
| OH1 | Org home page | Page at `/{org-slug}/` (after login) or `/org/` (with JWT) showing org context and navigation to home vs dashboard. | P0 |
| OH2 | Agents list (visibility-filtered) | List only agents the current user can access (global + org public + team/private per CanAccessAgent). Backend: org-scoped agent list API or ensure GET /agents filters by JWT org_id and visibility. | P0 |
| OH3 | Recent goals | List recent goals for the org (or for current user’s accessible agents). Pagination or limit. | P0 |
| OH4 | Quick-submit goal form | Form to submit a new goal to a chosen agent; calls existing goal submission API with org context. | P0 |
| OH5 | Chat (if enabled) | Same chat widget or flow as super-admin dashboard, scoped to org / user. | P1 |

### 3.3 Org admin dashboard (org_admin only)

| # | Feature | Acceptance criteria | Priority |
|---|--------|---------------------|----------|
| AD1 | Org_admin gate | Dashboard and org-scoped write APIs require org_role=admin (enforced in gateway + UI: show dashboard only for org admins; 403 otherwise). | P0 |
| AD2 | Members management | List with role; invite (existing); change role (PATCH); remove (DELETE). UI: role dropdown and remove button per row. | P0 |
| AD3 | Teams management | List; create (existing); edit (PATCH); delete (DELETE); manage team members (add/remove). UI: edit/delete and member management. | P0 |
| AD4 | Agents (full list + management) | List all org agents (all visibilities); create agent (POST /org/api/agents); edit visibility; manage collaborators (add/remove); manage agent admins (add/remove). UI: table + create/edit/collaborators/admins modals or panels. | P0 |
| AD5 | Goals (org-scoped list + full detail) | List goals for org (paginated); open goal detail: full goal_text, task list with payloads, execution results, code (e.g. generated code modal). Reuse or mirror super-admin goal detail pattern, with org-scoped API. | P0 |
| AD6 | Workers | List workers that have run tasks for this org (or org-scoped worker view). Same data as super-admin workers but filtered by org. | P1 |
| AD7 | Approvals | List and act on pending approvals for org agents (plan + risky_task). Approve/reject from dashboard. Org-scoped approval API or filter. | P0 |
| AD8 | Cost | Org-scoped LLM usage (tokens/cost per agent, per model, per day). Table or chart; 7-day window consistent with super-admin. | P1 |

### 3.4 Backend / API gaps

| # | Gap | Acceptance criteria |
|---|-----|---------------------|
| API1 | Org-scoped agent list | GET /agents (or GET /org/api/agents) when called with org JWT returns only agents for that org (and global). Agent-service/gateway filters by org_id. |
| API2 | Org-scoped goals list and detail | GET /org/api/goals (list), GET /org/api/goals/{id} (full detail with tasks, results, code). Or reuse existing goal APIs with org_id filter and ensure gateway passes org. |
| API3 | Org-scoped snapshot or equivalents | Org dashboard needs: counts (goals, tasks, agents, workers, approvals), cost summary. Either org-scoped snapshot endpoint or dedicated org-scoped read APIs for each. |
| API4 | Org-scoped approvals | List and approve/reject with org scope (only approvals for agents in the org). |
| API5 | Org-scoped cost | Query cost/usage by org_id; used by org dashboard cost widget. |

### 3.5 UI/UX and consistency

| # | Item | Notes |
|---|------|--------|
| UX1 | Move org dashboard out of main.go | Replace inline `orgDashboardHTML` with static files under e.g. `cmd/api-gateway/dashboard/org/` or `cmd/api-gateway/org-dashboard/` and serve like super-admin dashboard for maintainability. |
| UX2 | Shared components | Reuse or align patterns (tables, modals, charts) with super-admin dashboard where possible. |
| UX3 | Navigation | Org home: link to org dashboard (for admins). Org dashboard: link back to org home; sign out → org login page. |

---

## 4. Recommended Build Order and Ownership

Ownership is expressed in terms of the existing hierarchy (Architect, Tech Lead, Go Engineer, DevOps, QA, UI/UX). Project Manager tracks and delegates.

| Phase | Work | Owner | Deps |
|-------|------|--------|------|
| 1 | **Routing and URL alignment** — Decide and document whether to add `/org/` and `/org/dashboard` (JWT-based) alongside `/{org-slug}` and `/{org-slug}/dashboard`. Implement chosen scheme (handlers + redirects). Ensure `org` in reservedPaths does not break new routes. | Architect + Tech Lead | — |
| 2 | **Org_admin enforcement** — Enforce org_admin for org dashboard and org write APIs (gateway middleware or handler checks). Return 403 for non-admins on dashboard and mutating org APIs. | Tech Lead → Go Engineer | Phase 1 |
| 3 | **Org-scoped APIs (read)** — Org-scoped agent list (GET /agents filtered by JWT org_id or GET /org/api/agents). Org-scoped goals list and goal detail (new or existing endpoints with org_id). Org-scoped cost and approvals (new or filter existing). | Architect (spec) → Tech Lead → Go Engineer | Phase 2 |
| 4 | **Org dashboard backend** — Org-scoped snapshot or equivalent (counts, approvals, cost summary) for org dashboard overview. | Tech Lead → Go Engineer | Phase 3 |
| 5 | **Org dashboard UI (admin)** — Replace inline org dashboard with static assets. Implement Members (role change, remove), Teams (edit, delete, members), Agents (create, edit visibility, collaborators, admins), Goals (list + full detail), Approvals (list + approve/reject), Cost. Restrict access to org_admin. | Tech Lead → Go Engineer / UI/UX | Phase 4 |
| 6 | **Org home** — New page: visibility-filtered agents, recent goals, quick-submit goal form. Optional chat. | Tech Lead → Go Engineer / UI/UX | Phase 3, 5 |
| 7 | **Workers and cost polish** — Workers table and cost charts/tables on org dashboard if not done in phase 5. | Go Engineer | Phase 5 |
| 8 | **Validation and docs** — Update `scripts/validate.sh` for org dashboard and org home (e.g. smoke tests). Update PRD §URL table and §Phase 11.7 to reflect final URL scheme and checklist. | Tech Lead, QA | All |

Suggested order: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8. 6 can start once 3 is done; 7 can be in parallel or after 5.

---

## 5. URL and Routing Alignment (Recommendation)

- **Keep and document:**
  - `/{org-slug}` — org login page (no JWT required).
  - `POST /{org-slug}/login` — org login.
  - `GET /{org-slug}/dashboard` — org admin dashboard (after login; org from slug).
  - `GET /org/api/*` — org-level API (org from JWT `X-Org-Id` or query `org_id`).

- **Add and document:**
  - `GET /org/` — Org home. Requires valid org JWT. If JWT has org_id, resolve org (e.g. slug) and serve org home (or redirect to `/{org-slug}/` for consistency). If no org in JWT, redirect to `/login` or org picker.
  - `GET /org/dashboard` — Org admin dashboard. Requires org_admin JWT. Redirect to `/{org-slug}/dashboard` using org from JWT, or serve same dashboard with org context from JWT (avoid duplicate HTML).

- **PRD update:** In §19 and Phase 11.7, state that:
  - Org home is available at **both** `/org/` (JWT org context) and `/{org-slug}/` (after login).
  - Org admin dashboard is available at **both** `/org/dashboard` (JWT) and `/{org-slug}/dashboard` (after login).
  - Org-scoped APIs live under `/org/api/` and require org JWT; `org_id` may be passed via header or query for compatibility.

---

## 6. Summary

| Area | Exists today | To do |
|------|----------------|--------|
| **Routes** | `/{org-slug}`, `/{org-slug}/login`, `/{org-slug}/dashboard` | Add optional `/org/`, `/org/dashboard`; document both. |
| **Org home** | No | New page: agents (visibility-filtered), recent goals, goal form, chat. |
| **Org dashboard** | Stub at `/{org-slug}/dashboard` (Overview, Agents, Goals placeholder, Teams, Members) | Full admin dashboard: members/teams/agents/goals/approvals/cost (and workers); org_admin gate; move UI to files. |
| **Org APIs** | Teams, members, agents (create, collaborators, admins) | Org-scoped agent list, goals list/detail, snapshot-like read, approvals, cost. |
| **Auth** | JWT with org_id; org_id passed in query/header | Enforce org_admin for dashboard and mutating org APIs. |

This plan is for planning and delegation only; implementation is to be done by the assigned roles (Architect, Tech Lead, Go Engineer, UI/UX, QA) per the hierarchy.
