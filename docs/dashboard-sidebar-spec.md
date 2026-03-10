# Dashboard Right-Sidebar Design Spec

Fill the empty right third of the Astra Platform Dashboard with at-a-glance widgets using only data from `/api/dashboard/snapshot`. No new backend or API changes.

---

## 1. Layout

- **Two-column layout**: Left column ~65–70% width, right sidebar ~30–35%.
- **Left column**: All existing content (summary grid, charts, recent goals, services, workers, approvals, cost, logs, pids) unchanged in structure and behavior.
- **Right sidebar**: New widget cards; scrolls with the page (no fixed/sticky unless desired for UX).
- **Responsive**: On narrow viewports (e.g. max-width 900px), stack vertically: main column full width, then sidebar full width below.

---

## 2. HTML Structure

### 2.1 Body structure after `<header>`

- Insert a **wrapper** div that contains both the main content and the sidebar:
  - **Class**: `dashboard-wrapper`
  - **Children** (in order):
    1. **Main column**: `<div class="dashboard-main">` — contains all current sections that are currently direct children of `<body>` after the header (i.e. `#section-summary` through `#section-pids`).
    2. **Sidebar**: `<aside class="dashboard-sidebar">` — contains the new widget sections below.

So the structure is:

```
body
  header#dashboard-header (unchanged)
  div.dashboard-wrapper
    div.dashboard-main
      section#section-summary.summary-grid
      section#section-charts.charts-grid
      section#section-recent-goals
      section#section-services
      section#section-workers
      section#section-approvals
      section#section-cost
      section#section-logs
      section#section-pids
    aside.dashboard-sidebar
      (new widget sections)
```

### 2.2 Sidebar widget sections (inside `aside.dashboard-sidebar`)

Each widget is a **section** with class `dashboard-card sidebar-card`. Use existing `.dashboard-card` and add `.sidebar-card` for sidebar-specific styling (e.g. padding/sizing). Order:

1. **Health at a glance**
   - `<section class="dashboard-card sidebar-card" id="sidebar-health">`
   - `<h3 class="section-title">Health at a glance</h3>`
   - Element for text summary: `<div id="health-summary-text">` (e.g. "12 healthy / 1 unhealthy")
   - Optional: small canvas for donut: `<canvas id="chart-health-donut"></canvas>` (if Chart.js donut is used; else omit and keep text only)

2. **Task queue**
   - `<section class="dashboard-card sidebar-card" id="sidebar-task-queue">`
   - `<h3 class="section-title">Task queue</h3>`
   - Waiting count: `<span id="task-queue-waiting">0</span>` (with a label "Waiting" or similar)
   - Running count: `<span id="task-queue-running">0</span>` (label "Running")
   - Optional: wrap in a small stacked bar or two big numbers in a line (e.g. "Waiting: 5 · Running: 2")

3. **Cost summary (7d)**
   - `<section class="dashboard-card sidebar-card" id="sidebar-cost">`
   - `<h3 class="section-title">Cost (7d)</h3>`
   - `<div id="cost-summary-total">Total: $0.00</div>`

4. **Worker utilization**
   - `<section class="dashboard-card sidebar-card" id="sidebar-workers">`
   - `<h3 class="section-title">Worker utilization</h3>`
   - `<div id="worker-util-summary">` (e.g. "3 active workers, 2 tasks running")

5. **Pending approvals**
   - `<section class="dashboard-card sidebar-card" id="sidebar-approvals">`
   - `<h3 class="section-title">Pending approvals</h3>`
   - `<div id="approvals-summary-text">` (e.g. "No pending approvals" or "3 pending" + short list)
   - Optional: `<ul id="approvals-summary-list"></ul>` for first 3 items (tool name or id)

Use consistent IDs so JS can populate them: `health-summary-text`, `task-queue-waiting`, `task-queue-running`, `cost-summary-total`, `worker-util-summary`, `approvals-summary-text`, `approvals-summary-list`.

---

## 3. CSS Rules

### 3.1 Grid layout for two columns

- **Selector**: `.dashboard-wrapper`
  - `display: grid`
  - `grid-template-columns: 1fr 320px` (or `minmax(0, 2fr) minmax(280px, 1fr)` to get ~65–70% / ~30–35%)
  - `gap: 16px`
  - `align-items: start`
  - Use existing `--color-*` variables; no new colors.

- **Selector**: `.dashboard-main`
  - `min-width: 0` (for grid overflow)

- **Selector**: `.dashboard-sidebar`
  - `min-width: 0`
  - Sidebar scrolls with page (default block flow).

### 3.2 Responsive

- **Selector**: `@media (max-width: 900px)` (or similar breakpoint)
  - `.dashboard-wrapper`: `grid-template-columns: 1fr` so the sidebar goes below the main column.

### 3.3 Sidebar cards

- **Selector**: `.sidebar-card` (in addition to `.dashboard-card`)
  - Reuse `.dashboard-card` background, border, padding.
  - Optional: `margin-bottom: 12px` (or same as other `.dashboard-card`).
  - Font size for values: e.g. `14px` for big numbers, `12px` for labels, consistent with existing `.section-title` and `.summary-value` where appropriate.

### 3.4 Task queue display

- **Selector**: `#sidebar-task-queue` (or a child container)
  - Display "Waiting" and "Running" on one line (e.g. flex with gap), or stacked.
  - IDs: `#task-queue-waiting`, `#task-queue-running` for the numeric values.

### 3.5 Approvals list

- **Selector**: `#approvals-summary-list`
  - List style: short lines (e.g. first 3 approval tool names or ids); `list-style` and padding to match dark theme.

---

## 4. JavaScript Logic

All data comes from the existing `fetchSnapshot()` response. After the current `renderSummary`, charts, and table render calls, call the new render functions with the same snapshot object (or the relevant slices). No new API calls.

### 4.1 Data mapping (from snapshot)

- **Health**: `data.services` — array of `{ name, port, type, healthy, latency_ms }`. Count `healthy === true` vs `healthy === false`.
- **Task queue**: `data.jobs.tasks` — `waiting = (tasks.pending || 0) + (tasks.queued || 0) + (tasks.scheduled || 0)`, `running = tasks.running || 0`.
- **Cost**: `data.cost.rows` — array of `{ day, agent_id, model, tokens_in, tokens_out, cost_dollars }`. Sum `cost_dollars` (parse as number) for all rows; format as "Total: $X.XX".
- **Workers**: `data.workers` — same active count logic as summary (status active/online); running count from `data.jobs.tasks.running`.
- **Approvals**: `data.approvals` — array; count = length; show first 3 (e.g. tool_name or id) in a short list.

### 4.2 New render functions

1. **renderHealthSummary(services)**
   - **Arg**: `services` — array from snapshot.
   - **Logic**: Count healthy vs unhealthy. Set `#health-summary-text` to e.g. "N healthy / M unhealthy". If Chart.js donut is used: update or create a small doughnut chart in `#chart-health-donut` with two segments (healthy, unhealthy) and colors `--color-healthy`, `--color-unhealthy`.

2. **renderTaskQueueSummary(tasks)**
   - **Arg**: `tasks` — `jobs.tasks` object.
   - **Logic**: `waiting = (tasks.pending || 0) + (tasks.queued || 0) + (tasks.scheduled || 0)`, `running = tasks.running || 0`. Set `#task-queue-waiting` to waiting, `#task-queue-running` to running. Optional: render a tiny stacked bar (e.g. two divs with flex or a small canvas) — spec leaves this to implementer.

3. **renderCostSummary(cost)**
   - **Arg**: `cost` — snapshot `cost` object with `rows` array.
   - **Logic**: Sum `cost.rows[].cost_dollars` (parse float); set `#cost-summary-total` to `"Total: $X.XX"` (2 decimal places). If no rows, "Total: $0.00".

4. **renderWorkerUtilization(workers, tasks)**
   - **Arg**: `workers` — array; `tasks` — `jobs.tasks`.
   - **Logic**: Active workers = same filter as in `renderSummary` (status active/online). Running = `tasks.running || 0`. Set `#worker-util-summary` to e.g. "X active workers, Y tasks running".

5. **renderApprovalsSummary(approvals)**
   - **Arg**: `approvals` — array from snapshot.
   - **Logic**: If length === 0: set `#approvals-summary-text` to "No pending approvals"; clear or hide `#approvals-summary-list`. Else: set text to e.g. "N pending"; populate `#approvals-summary-list` with first 3 items (e.g. list item per approval showing `tool_name` or `id`).

### 4.3 Integration in fetchSnapshot().then(d => …)

After the existing calls:

- `renderHealthSummary(d.services || []);`
- `renderTaskQueueSummary(d.jobs && d.jobs.tasks ? d.jobs.tasks : {});`
- `renderCostSummary(d.cost || { rows: [] });`
- `renderWorkerUtilization(d.workers || [], d.jobs && d.jobs.tasks ? d.jobs.tasks : {});`
- `renderApprovalsSummary(d.approvals || []);`

---

## 5. Chart.js (optional)

If a small health donut is added:

- Reuse existing Chart.js already loaded for the dashboard.
- Chart type: doughnut; two segments: healthy count, unhealthy count; colors from `--color-healthy` and `--color-unhealthy`; minimal options (no legend if space is tight, or small legend).
- Create chart on first run, update `.data.datasets[0].data` on subsequent runs (same pattern as `renderTaskChart`).

---

## 6. Deliverables Checklist

- [ ] **index.html**: Wrap current sections (summary through pids) in `div.dashboard-main`; add `aside.dashboard-sidebar` with the five widget sections and element ids above.
- [ ] **style.css**: Add `.dashboard-wrapper` grid, `.dashboard-main`, `.dashboard-sidebar`, `.sidebar-card`, responsive rule, and any sidebar-specific value/label styles.
- [ ] **app.js**: Implement `renderHealthSummary`, `renderTaskQueueSummary`, `renderCostSummary`, `renderWorkerUtilization`, `renderApprovalsSummary`, and call them from the snapshot callback with the correct arguments.

---

## 7. Security & PRD

- No new API or backend; all data from existing `/api/dashboard/snapshot`. No new secrets or auth changes. Layout and rendering only.
