# Astra Platform Dashboard — UI/UX Spec (Utilitarian)

**Version:** 1.0  
**Source:** UI/UX Expert → Tech Lead → Go Engineer  
**Delegation:** This spec was produced by the **UI/UX Expert** under direction of the **Tech Lead** and is implementation-ready for the Go Engineer. Apply as-is; no further design iteration required for the utilitarian variant.

---

## Design Philosophy

Utilitarian, information-dense, no decorative elements. Focus on complete platform visibility with minimal chrome. Monospace and system fonts, dark theme, high contrast.

---

## 1. Page Structure & Sections

Single-page layout. All sections below the header in vertical scroll order.

| Section ID | Title | Order |
|------------|-------|-------|
| `#dashboard-header` | Astra Platform Dashboard | 1 |
| `#dashboard-meta` | Last updated, refresh status | 2 |
| `#section-services` | Services | 3 |
| `#section-workers` | Workers | 4 |
| `#section-approvals` | Pending Approvals | 5 |
| `#section-cost` | Cost Summary | 6 |
| `#section-logs` | Log Tails | 7 |
| `#section-pids` | Process IDs | 8 |

---

## 2. Header & Meta (`#dashboard-header`, `#dashboard-meta`)

**Elements:**
- `h1#dashboard-title` — "Astra Platform Dashboard"
- `div#dashboard-meta` — contains:
  - `span#last-updated` — "Last updated: {ISO timestamp}"
  - `span#refresh-status` — "Refreshing" or "Idle" or "Error: {msg}"
  - `span#manual-refresh` — optional "Refresh now" button (`button#btn-refresh`)

**Classes:**
- `dashboard-header`, `dashboard-meta`, `meta-item`

---

## 3. Data Sections — Cards & Tables

### 3.1 Services (`#section-services`)

**Card:** `div.dashboard-card.card-services`
- **Title:** `h2.section-title` — "Services"
- **Content:** `table#table-services.data-table`
  - Columns: `Name`, `Port`, `Type`, `Status`, `Latency (ms)`
  - Row class: `tr-service`; each row has `data-service-name="{name}"`
  - Status cell: `td.td-status` with class `status-healthy` or `status-unhealthy`
  - Empty state: `tr.empty-row` with `td.empty-message` colspan 5 — "No services configured"

**Table structure:**
```html
<table id="table-services" class="data-table">
  <thead><tr><th>Name</th><th>Port</th><th>Type</th><th>Status</th><th>Latency (ms)</th></tr></thead>
  <tbody id="tbody-services"></tbody>
</table>
```

### 3.2 Workers (`#section-workers`)

**Card:** `div.dashboard-card.card-workers`
- **Title:** `h2.section-title` — "Workers"
- **Content:** `table#table-workers.data-table`
  - Columns: `ID`, `Hostname`, `Status`, `Capabilities`, `Last Heartbeat`
  - Row class: `tr-worker`; `data-worker-id="{id}"`
  - Status cell: `td.td-status` with `status-active` or `status-inactive` or `status-stale`
  - Empty state: `td.empty-message` colspan 5 — "No workers registered"

### 3.3 Approvals (`#section-approvals`)

**Card:** `div.dashboard-card.card-approvals`
- **Title:** `h2.section-title` — "Pending Approvals"
- **Content:** `table#table-approvals.data-table`
  - Columns: `ID`, `Tool`, `Action Summary`, `Status`, `Requested At`
  - Row class: `tr-approval`; `data-approval-id="{id}"`
  - Status cell: `status-pending` or `status-approved` or `status-denied`
  - Empty state: `td.empty-message` colspan 5 — "No pending approvals"

### 3.4 Cost (`#section-cost`)

**Card:** `div.dashboard-card.card-cost`
- **Title:** `h2.section-title` — "Cost (7 days)"
- **Content:** `table#table-cost.data-table`
  - Columns: `Day`, `Agent ID`, `Model`, `Tokens In`, `Tokens Out`, `Cost ($)`
  - Row class: `tr-cost`; `data-day="{day}"`
  - Empty state: `td.empty-message` colspan 6 — "No cost data"

### 3.5 Log Tails (`#section-logs`)

**Card:** `div.dashboard-card.card-logs`
- **Title:** `h2.section-title` — "Log Tails"
- **Content:** `div#logs-container` with per-service blocks:
  - Each: `div.log-block` with `data-service="{name}"`
  - Header: `h3.log-block-title` — "{service-name} (last 20 lines)"
  - Content: `pre.log-block-content` — monospace, scrollable
  - Empty state: `pre.empty-message` — "No logs available"

### 3.6 PIDs (`#section-pids`)

**Card:** `div.dashboard-card.card-pids`
- **Title:** `h2.section-title` — "Process IDs"
- **Content:** `table#table-pids.data-table` or `dl#pids-list`
  - Columns/items: Service name → PID
  - Row: `tr-pid`; `data-service-name="{name}"`
  - Empty state: `td.empty-message` — "No PID data"

---

## 4. Refresh Behavior

| Event | Behavior |
|-------|----------|
| Page load | Fetch `GET /api/dashboard/snapshot` once, render, set `#last-updated` |
| Auto-refresh | `setInterval(fetchSnapshot, 5000)` — every 5 seconds |
| During fetch | Set `#refresh-status` to "Refreshing"; show loading state on cards if desired |
| Success | Update all sections; set `#last-updated` to `new Date().toISOString()`; set `#refresh-status` to "Idle" |
| Error | Set `#refresh-status` to "Error: {message}"; keep previous data; add `class="has-error"` to `#dashboard-meta` |
| Manual refresh | `#btn-refresh` click triggers `fetchSnapshot()` immediately (debounce 1s) |

**JS entry point:** `app.js` — `document.addEventListener('DOMContentLoaded', initDashboard)`

---

## 5. Severity & Status Color Rules

Use CSS variables for consistency. Dark theme base.

| Status / Severity | Class | Color (hex) | Use |
|------------------|-------|-------------|-----|
| Healthy / Active / OK | `status-healthy`, `status-active` | `#22c55e` (green) | Service up, worker active |
| Unhealthy / Inactive | `status-unhealthy`, `status-inactive` | `#ef4444` (red) | Service down, worker offline |
| Stale / Warning | `status-stale` | `#f59e0b` (amber) | Worker heartbeat > 30s ago |
| Pending | `status-pending` | `#f59e0b` (amber) | Approval awaiting action |
| Approved | `status-approved` | `#22c55e` (green) | Approval granted |
| Denied | `status-denied` | `#ef4444` (red) | Approval rejected |
| Error | `has-error` | `#ef4444` (red) | Fetch failed |
| Latency OK (≤10ms) | (no class) | default | — |
| Latency warning (>10ms) | `latency-warning` | `#f59e0b` | Exceeds 10ms target |

**CSS variables to define:**
```css
--color-healthy: #22c55e;
--color-unhealthy: #ef4444;
--color-warning: #f59e0b;
--color-text: #e5e7eb;
--color-bg: #111827;
--color-surface: #1f2937;
```

---

## 6. Empty States

Every data section must handle empty responses:

| Section | Condition | Message |
|---------|-----------|---------|
| Services | `services.length === 0` | "No services configured" |
| Workers | `workers.length === 0` | "No workers registered" |
| Approvals | `approvals.length === 0` | "No pending approvals" |
| Cost | `cost.rows.length === 0` | "No cost data" |
| Logs | All log arrays empty | "No logs available" |
| PIDs | No PIDs | "No PID data" |

**Implementation:** Render one row/cell with `class="empty-row"` or `class="empty-message"` and the message. Do not show the table header if empty (optional—utilitarian can keep header for consistency).

---

## 7. Error States

| Scenario | Behavior |
|----------|----------|
| Snapshot fetch fails (network/4xx/5xx) | Show error in `#refresh-status`; add `has-error` to meta; retain last good data in sections |
| Malformed JSON | Same as fetch fails; message: "Invalid response" |
| Partial failure (e.g. workers timeout) | Snapshot still returns; show available data; optional: show partial-error indicator in affected section |

**No global overlay.** Errors are inline. If first fetch fails, sections remain empty with no empty-state message until first success.

---

## 8. Exact IDs & Classes Reference

### IDs (must match)

| ID | Element |
|----|---------|
| `dashboard-header` | header container |
| `dashboard-title` | h1 |
| `dashboard-meta` | meta div |
| `last-updated` | span |
| `refresh-status` | span |
| `btn-refresh` | button |
| `section-services` | section |
| `section-workers` | section |
| `section-approvals` | section |
| `section-cost` | section |
| `section-logs` | section |
| `section-pids` | section |
| `table-services` | table |
| `tbody-services` | tbody |
| `table-workers` | table |
| `tbody-workers` | tbody |
| `table-approvals` | table |
| `tbody-approvals` | tbody |
| `table-cost` | table |
| `tbody-cost` | tbody |
| `table-pids` | table |
| `tbody-pids` | tbody |
| `logs-container` | div |
| `pids-list` | dl (if used) |

### Classes (must use)

| Class | Use |
|-------|-----|
| `dashboard-card` | All section cards |
| `card-services`, `card-workers`, etc. | Section-specific |
| `section-title` | h2 per section |
| `data-table` | All tables |
| `tr-service`, `tr-worker`, etc. | Table rows |
| `td-status` | Status cells |
| `status-healthy`, `status-unhealthy`, etc. | Status styling |
| `empty-row`, `empty-message` | Empty states |
| `log-block`, `log-block-title`, `log-block-content` | Log section |
| `has-error` | Error state |
| `latency-warning` | Latency > 10ms |

---

## 9. File Layout (per blueprint)

```
cmd/api-gateway/dashboard/
  index.html
  static/
    style.css
    app.js
```

**`index.html`** — Semantic structure with all IDs/classes above. No inline styles.  
**`style.css`** — Dark theme, CSS variables, table layout, status colors.  
**`app.js`** — Fetch snapshot, render functions per section, 5s interval, error handling.

---

## 10. Snapshot Contract (Reference)

As defined in `docs/dashboard-implementation-blueprint.md` §5. No changes. Keys: `services`, `workers`, `approvals`, `cost`, `logs`, `pids`.

---

*Spec complete. Go Engineer: implement per this document and `docs/dashboard-implementation-blueprint.md`.*
