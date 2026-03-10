# Astra Platform Dashboard — Material Design 3 Theme Spec

This document specifies the Material Design 3 (M3) theming applied to the Astra Platform Dashboard (vanilla HTML/CSS/JS + Chart.js). Implementation uses **Option A: pure CSS + M3 tokens** — no React, no Material Web Components; only Google Fonts (Roboto) and CSS custom properties.

---

## 1. Design approach

- **Option chosen:** Option A — Pure CSS with M3 design tokens (CSS variables), Roboto from Google Fonts, and existing HTML structure.
- **Default theme:** Dark (to match current dashboard). Light theme variables are defined for a future toggle.
- **Scope:** Header, summary cards, chart containers, data tables, sidebar cards, action buttons (Approve/Reject), log blocks, lists/links. Chart.js palettes use M3 token values.

---

## 2. M3 color tokens (CSS variables)

### 2.1 Dark theme (default)

| Token | CSS variable | Value | Usage |
|-------|--------------|-------|--------|
| **Primary** | `--md-sys-color-primary` | `#a8c7fa` | Primary actions, links, key UI |
| **On primary** | `--md-sys-color-on-primary` | `#003258` | Text on primary |
| **Primary container** | `--md-sys-color-primary-container` | `#004a77` | Tonal surfaces (e.g. summary accents) |
| **On primary container** | `--md-sys-color-on-primary-container` | `#d3e3fd` | Text on primary container |
| **Secondary** | `--md-sys-color-secondary` | `#9ecbf5` | Secondary emphasis |
| **On secondary** | `--md-sys-color-on-secondary` | `#003258` | Text on secondary |
| **Tertiary** | `--md-sys-color-tertiary` | `#c8b8ff` | Tertiary/charts, accents |
| **On tertiary** | `--md-sys-color-on-tertiary` | `#2e1065` | Text on tertiary |
| **Error** | `--md-sys-color-error` | `#f2b8b5` | Error, failed, reject |
| **On error** | `--md-sys-color-on-error` | `#601410` | Text on error |
| **Error container** | `--md-sys-color-error-container` | `#8c1d18` | Error surfaces |
| **Surface** | `--md-sys-color-surface` | `#1c1b1f` | Main background (cards, panels) |
| **Surface dim** | `--md-sys-color-surface-dim` | `#141316` | Page background |
| **Surface bright** | `--md-sys-color-surface-bright` | `#3b383e` | Elevated surfaces |
| **Surface container** | `--md-sys-color-surface-container` | `#211f23` | Card/section container |
| **Surface container high** | `--md-sys-color-surface-container-high` | `#2b292d` | Higher elevation |
| **Surface variant** | `--md-sys-color-surface-variant` | `#44474e` | Variant surfaces (e.g. table header) |
| **On surface** | `--md-sys-color-on-surface` | `#e6e1e5` | Primary text on surface |
| **On surface variant** | `--md-sys-color-on-surface-variant` | `#c4c6d0` | Secondary text |
| **Outline** | `--md-sys-color-outline` | `#8e9099` | Borders, dividers |
| **Outline variant** | `--md-sys-color-outline-variant` | `#44474e` | Subtle borders |

**Semantic aliases** (for status and charts):

| Alias | Maps to | Use |
|-------|---------|-----|
| `--md-ref-success` | `#7dd87d` | Healthy, completed, approved |
| `--md-ref-warning` | `#e6c547` | Pending, running, warning |
| `--md-ref-error` | `var(--md-sys-color-error)` | Failed, unhealthy, reject |

### 2.2 Light theme (for future toggle)

Defined under `[data-theme="light"]` (or `.theme-light`):

- `--md-sys-color-surface` → `#fef7ff`
- `--md-sys-color-surface-dim` → `#ded8e1`
- `--md-sys-color-on-surface` → `#1c1b1f`
- `--md-sys-color-primary` → `#0061a4`
- `--md-sys-color-on-primary` → `#ffffff`
- (Full set in `style.css`; dark remains default.)

---

## 3. Typography

- **Font family:** Roboto (M3 recommended) for UI; Roboto Mono for log content. Load via Google Fonts:  
  `https://fonts.googleapis.com/css2?family=Roboto:wght@400;500;700&family=Roboto+Mono:wght@400&display=swap`
- **Scale (M3 type scale):**
  - **Display:** Not used on dashboard; reserved for hero.
  - **Headline:** `font-size: 1.5rem` (24px), `font-weight: 700` — dashboard title (`h1`).
  - **Title (large):** `1.25rem` (20px), `500` — section titles (`h2.section-title`).
  - **Title (medium):** `1rem` (16px), `500` — sidebar card titles (`h3.section-title`).
  - **Body (large):** `1rem` (16px), `400` — body text.
  - **Body (medium):** `0.875rem` (14px), `400` — tables, meta.
  - **Label (large):** `0.875rem`, `500` — buttons, labels.
  - **Label (small):** `0.75rem` (12px), `500` — summary labels, table headers, chips.
- **Line height:** 1.25–1.5 for titles, 1.4–1.5 for body.
- **Letter spacing:** Default (0) or slight positive for labels (e.g. 0.5px).

---

## 4. Component mapping

| Current class / element | M3 role | Implementation |
|------------------------|--------|----------------|
| `body` | Background | `background: var(--md-sys-color-surface-dim)`; `color: var(--md-sys-color-on-surface)` |
| `.dashboard-header` | Surface container | Use surface + elevation 1 (shadow) |
| `#dashboard-title` (h1) | Headline | M3 headline type; `color: var(--md-sys-color-on-surface)` |
| `.swagger-link` | Primary link | `color: var(--md-sys-color-primary)`; hover: state layer (opacity) |
| `#btn-refresh` | Filled or tonal button | M3 filled tertiary or tonal; 8px radius; hover/focus state |
| `.summary-card` | Filled card (tonal) | `background: var(--md-sys-color-surface-container)`; 12px radius; level 1 elevation; accent = 4px top border using primary/error/success/etc. |
| `.summary-value` | Title large | Large type; color = accent (primary, success, error, tertiary) |
| `.summary-label` | Label small | `color: var(--md-sys-color-on-surface-variant)`; uppercase optional |
| `.dashboard-card`, `.chart-container` | Elevated surface | `background: var(--md-sys-color-surface-container)`; 12px radius; elevation 1 (shadow) |
| `.section-title` | Title large/medium | M3 title type |
| `.data-table` | Surface variant header | `thead` bg `surface-variant`; borders `outline-variant` |
| `.action-btn.approve` | Filled button (success) | Green/success color; M3 shape |
| `.action-btn.reject` | Filled button (error) | Error color; M3 shape |
| `.log-block` | Surface container | Same as card; `.log-block-title` = surface-variant |
| `.sidebar-card` | Surface container high | Slightly elevated; 12px radius |
| `.empty-message` | On surface variant | `color: var(--md-sys-color-on-surface-variant)` |
| Status classes (`.td-status.status-healthy`, etc.) | Semantic colors | success / warning / error / primary from tokens |

---

## 5. Shape (border radius)

- **Cards, panels, log blocks:** `12px` (M3 medium shape).
- **Buttons:** `8px` (full radius or 8px).
- **Chips / tags:** `8px`.
- **Tables:** Optional `4px` on first/last cell or full table container radius `12px`.

---

## 6. Elevation (shadows and surface tint)

- **Level 0:** No shadow (background).
- **Level 1:** Cards, header — `box-shadow: 0 1px 2px rgba(0,0,0,0.3)` (dark); light theme: `0 1px 3px rgba(0,0,0,0.12)`.
- **Level 2:** Optional for raised buttons or dropdowns — `0 2px 6px rgba(0,0,0,0.4)` (dark).
- **Surface tint:** Optional overlay for elevation (e.g. `background: linear-gradient(...)` with primary at low opacity); not required for MVP — shadows suffice.

---

## 7. State layers (hover / focus)

- **Links:** Hover — `opacity: 0.8` or `text-decoration: underline`.
- **Buttons:** Hover — `filter: brightness(1.1)` or background lighten; focus — `outline: 2px solid var(--md-sys-color-primary)`; `outline-offset: 2px`.
- **Table rows:** Optional `:hover { background: var(--md-sys-color-surface-container-high); }`.

---

## 8. Font link

Add in `<head>` of `index.html`:

```html
<link rel="preconnect" href="https://fonts.googleapis.com" />
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
<link href="https://fonts.googleapis.com/css2?family=Roboto:wght@400;500;700&family=Roboto+Mono:wght@400&display=swap" rel="stylesheet" />
```

---

## 9. Chart.js palette (M3)

Charts use token-aligned colors so they match the theme:

- **Task status:** created (outline), pending (tertiary), queued/scheduled (primary), running (warning), completed (success), failed (error).
- **Goals:** active (warning), completed (success), failed (error), pending (tertiary).
- **Service health:** healthy = success, unhealthy = error.
- **Health donut:** Same as service health.

Exact hex values are defined in CSS and read in `app.js` via `getComputedStyle` or duplicated as a single source in JS (see `app.js` section below).

---

## 10. Files to change

| File | Changes |
|------|---------|
| `cmd/api-gateway/dashboard/index.html` | Add Google Fonts links; optional `data-theme="dark"` on `<html>` for future toggle. |
| `cmd/api-gateway/dashboard/static/style.css` | Replace `:root` with M3 tokens; add light theme block; restyle all components per this spec. |
| `cmd/api-gateway/dashboard/static/app.js` | Set Chart.js default colors and chart options (grid, ticks) to M3 token values (from CSS vars or shared hex object). |

---

## 11. Compliance

- **No new framework:** No React, Angular, or MWC; only CSS + Google Fonts.
- **No build step:** All assets remain static.
- **Accessibility:** Sufficient contrast (M3 dark palette meets WCAG for normal text); focus outlines on interactive elements.
