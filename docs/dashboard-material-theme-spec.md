# Astra Dashboard — Glassmorphism Theme Spec

**Replaces:** Material Design 3 flat surfaces
**Applied in:** `cmd/api-gateway/dashboard/static/style.css`
**Themes:** Dark (default) · Light (switchable)

---

## Design System

The dashboard uses **glassmorphism**: frosted-glass surfaces floating over a gradient background. Key properties on every card or container:

```css
background: var(--glass-bg);
backdrop-filter: var(--glass-blur);        /* blur(20px) */
-webkit-backdrop-filter: var(--glass-blur);
border: 1px solid var(--glass-border);
border-radius: 16px;
box-shadow: var(--glass-shadow-sm);
```

The gradient body background is fixed (`background-attachment: fixed`) so it shows through all frosted surfaces.

---

## CSS Token Reference

### Dark Theme (default — `[data-theme="dark"]`)

| Token | Value | Purpose |
|-------|-------|---------|
| `--glass-body-bg` | `linear-gradient(135deg, #0a0818, #130d24, #0d1a2e, #0a1520)` | Page background |
| `--glass-bg` | `rgba(255,255,255,0.06)` | Card/container fill |
| `--glass-bg-hover` | `rgba(255,255,255,0.10)` | Hover state |
| `--glass-bg-active` | `rgba(255,255,255,0.15)` | Active/selected state |
| `--glass-bg-strong` | `rgba(255,255,255,0.10)` | Modal backgrounds, table headers |
| `--glass-bg-input` | `rgba(255,255,255,0.07)` | Input fields, chat panels |
| `--glass-border` | `rgba(255,255,255,0.10)` | Default border |
| `--glass-border-strong` | `rgba(255,255,255,0.18)` | Modal borders, summary card tops |
| `--glass-blur` | `blur(20px)` | Standard backdrop blur |
| `--glass-blur-sm` | `blur(12px)` | Small elements (inputs, badges) |
| `--glass-shadow` | `0 8px 32px rgba(0,0,0,0.45), 0 2px 8px rgba(0,0,0,0.30)` | Hover shadow |
| `--glass-shadow-sm` | `0 4px 16px rgba(0,0,0,0.35)` | Default card shadow |
| `--glass-shadow-modal` | `0 20px 60px rgba(0,0,0,0.60), 0 4px 16px rgba(0,0,0,0.40)` | Modal shadow |

### Light Theme (`[data-theme="light"]`)

| Token | Value | Purpose |
|-------|-------|---------|
| `--glass-body-bg` | `linear-gradient(135deg, #e8e0f8, #dce8fc, #f0e8ff, #e4f0fa)` | Page background |
| `--glass-bg` | `rgba(255,255,255,0.55)` | Card fill |
| `--glass-bg-hover` | `rgba(255,255,255,0.72)` | Hover |
| `--glass-bg-active` | `rgba(255,255,255,0.85)` | Active/selected |
| `--glass-bg-strong` | `rgba(255,255,255,0.65)` | Modal backgrounds |
| `--glass-bg-input` | `rgba(255,255,255,0.65)` | Input fields |
| `--glass-border` | `rgba(255,255,255,0.75)` | Default border |
| `--glass-border-strong` | `rgba(255,255,255,0.95)` | Modal borders |
| `--glass-shadow` | `0 8px 32px rgba(99,102,241,0.12), 0 2px 8px rgba(0,0,0,0.08)` | Hover shadow |
| `--glass-shadow-sm` | `0 4px 16px rgba(99,102,241,0.10)` | Default card shadow |
| `--glass-shadow-modal` | `0 20px 60px rgba(99,102,241,0.20), 0 4px 16px rgba(0,0,0,0.12)` | Modal shadow |

---

## Theme Toggle

A 🌙/☀️ button (`#btn-theme-toggle`, class `theme-toggle-btn`) lives in the header nav row. It sets `data-theme` on `<html>` and persists the choice to `localStorage` under the key `astra-theme`. Default is `dark`.

```js
// Toggle logic (inline script after app.js)
var stored = localStorage.getItem('astra-theme') || 'dark';
document.documentElement.setAttribute('data-theme', stored);
// Click handler flips dark ↔ light
```

---

## M3 Color Tokens (retained for text & accent colors)

The full Material Design 3 color token set is still present on `:root` and `[data-theme="light"]`. These tokens drive all **text, accent, and status colors** — only the **surface/background** tokens were replaced by glass tokens.

| M3 Token | Dark value | Light value | Purpose |
|----------|-----------|------------|---------|
| `--md-sys-color-primary` | `#a8c7fa` | `#0061a4` | Active nav tab, focus rings, links |
| `--md-sys-color-tertiary` | `#c8b8ff` | `#6750a4` | Purple accent, super-admin badge |
| `--md-sys-color-error` | `#f2b8b5` | `#ba1a1a` | Error states, reject buttons |
| `--md-ref-success` | `#7dd87d` | `#2e7d32` | Approve buttons, healthy status |
| `--md-ref-warning` | `#e6c547` | `#f9a825` | Warning/stale/running status |
| `--md-sys-color-on-surface` | `#e6e1e5` | `#1c1b1f` | Primary text |
| `--md-sys-color-on-surface-variant` | `#c4c6d0` | `#49454f` | Secondary text, labels |

---

## Element-by-Element Rules

| Element | Glass treatment |
|---------|----------------|
| `.dashboard-header` | `glass-bg` + `glass-blur` + `glass-border` + `border-radius: 16px` |
| `.dashboard-card`, `.chart-container` | `glass-bg` + `glass-blur` + `glass-border` + `border-radius: 16px` |
| `.summary-card` | `glass-bg` + `glass-blur` + accent `border-top` (color preserved per card type) |
| `.sidebar-card` | Same as `.dashboard-card` |
| `.goal-modal-content`, `.approval-modal-content` | `glass-bg-strong` + `glass-blur` + `glass-border-strong` + `border-radius: 20px` |
| Modal backdrops | `rgba(0,0,0,0.55)` + `backdrop-filter: blur(6px)` |
| `.data-table thead` | `glass-bg-strong` + `glass-blur-sm` |
| `.data-table` borders | `glass-border` (replaces solid M3 outline) |
| Inputs (`.modal-input`, `.search-input`, `.chat-input`) | `glass-bg-input` + `glass-blur-sm` + `glass-border` |
| `.nav-tab` | `glass-border` border; active tab keeps solid `--md-sys-color-primary` fill |
| `.action-btn.approve` / `.reject` | Solid fill retained (affordance requires contrast) |
| `.action-btn.view`, `.pagination-btn`, `.agent-action-btn` | `glass-bg-input` + `glass-border` |
| `.chat-widget-panel` | `glass-bg-strong` + `glass-blur` + `glass-border-strong` |
| Chat messages | `glass-border` border + `glass-blur-sm`; user bubble keeps `primary-container` fill |
| `.log-block` | `glass-bg` + `glass-blur-sm` + `glass-border` |

---

## Browser Support

`backdrop-filter` requires a prefix on WebKit (`-webkit-backdrop-filter`). All rules include both. Fallback: browsers without support show a slightly more opaque background (still functional).
