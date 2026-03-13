# Create Agent Dashboard Feature — Architectural Design

**Status:** Design  
**Author:** Principal Architect  
**Date:** 2026-03-13

---

## 1. Overview

Add a **Create Agent** button to the superadmin dashboard’s agents section (Overview tab) with a modal that supports:

1. Agent creation (name, actor_type, system_prompt, config)
2. Optional markdown document: inline editor or file upload
3. Optional visibility and chat_capable (backend permitting)
4. Flow: create agent → attach document(s) → refresh list

Uses existing APIs only. No new backend endpoints unless noted.

---

## 2. API Flow

### 2.1 Endpoints

| Step | Method | Endpoint | Purpose |
|------|--------|----------|---------|
| 1 | POST | `/agents` | Create agent; returns `actor_id` |
| 2 | POST | `/agents/{id}/documents` | Attach document(s) (0..n) |

**Note:** The dashboard uses relative URLs. From `/superadmin/dashboard/`, `authFetch('/agents', ...)` resolves to the same origin and hits the gateway’s `POST /agents`. No base path change.

### 2.2 Sequence

```
User fills form → clicks Create Agent
  → POST /agents { actor_type, name, system_prompt, config }
  → Response: { actor_id: "uuid" }
  → If document provided:
       POST /agents/{actor_id}/documents { doc_type, name, content }
       (one call per document; can be run in sequence)
  → Close modal, refresh snapshot (loadDashboardSnapshot → renderAgents)
  → Show success toast or inline confirmation
```

### 2.3 Error Handling

- **Agent create fails:** Show error in `modal-error`, keep modal open, don’t call documents API.
- **Agent succeeds, document fails:** Agent remains created; show error like "Agent created but document upload failed: …". Consider retry or manual upload later.
- **Partial document success:** Log which docs failed; show warning if any failed.

---

## 3. Modal Layout

### 3.1 Structure

Mirror existing Create Org / Create User modals:

```
┌─────────────────────────────────────────────────────────┐
│ Create Agent                                        [×] │
├─────────────────────────────────────────────────────────┤
│ [Agent Fields]                                          │
│  • name (required)                                      │
│  • actor_type (required)                                │
│  • system_prompt (optional, textarea)                   │
│  • config (optional, JSON string)                       │
│  • visibility (optional, select) — if API supports      │
│  • chat_capable (optional, checkbox) — if API supports  │
├─────────────────────────────────────────────────────────┤
│ [Optional Document]                                      │
│  • doc_type (rule|skill|context_doc|reference)          │
│  • doc_name (if document provided)                      │
│  • Tab: [Write inline] | [Upload .md file]               │
│    - Inline: <textarea>                                 │
│    - Upload: <input type=file accept=.md> + filename     │
├─────────────────────────────────────────────────────────┤
│ [modal-error]                                           │
│ [Create Agent]                                          │
└─────────────────────────────────────────────────────────┘
```

### 3.2 Field Order and Types

1. **name** — `input type="text"`, placeholder e.g. "E-Commerce Builder", required.
2. **actor_type** — `input type="text"` or `select` with common types (e.g. `ecommerce-builder`, `python-expert`, `chat-assistant`) + "Other" free text. Required.
3. **system_prompt** — `textarea`, placeholder "You are a senior full-stack developer...", optional.
4. **config** — `input type="text"` or `textarea`, placeholder `{"model_preference":"code"}`, optional.
5. **visibility** — `select` (global/public/team/private), optional. Only if backend accepts it in POST /agents (currently it does not).
6. **chat_capable** — `checkbox`, optional. Only if backend supports it (currently it does not).
7. **Document section** — collapsible or always visible:
   - doc_type: `select` (rule, skill, context_doc, reference), default `context_doc`.
   - doc_name: `input`, placeholder e.g. "agent-instructions", required when document is provided.
   - Content: tabs "Write inline" | "Upload file".

### 3.3 Document Content UI

**Tab 1 — Write inline**

- Single `<textarea>` with class `modal-input`, font monospace, ~8 rows.
- Optional placeholder: "# Agent instructions\n\n..."
- No syntax highlighting or preview for MVP; keep it simple.

**Tab 2 — Upload .md file**

- `<input type="file" accept=".md">`
- On change: use `FileReader.readAsText()` to read content.
- Show chosen filename and allow clear.
- Validate: only `.md` (or `.markdown`) allowed; optional max size check (e.g. 1MB).

---

## 4. File Upload Approach

### 4.1 Client-Side Only (Recommended)

- Use `FileReader.readAsText(file, 'UTF-8')`.
- Content is read locally; no multipart upload to server.
- Send document via existing JSON API: `POST /agents/{id}/documents` with `content` in the body.

**Pros:** No backend changes, reuse existing documents API, simpler UX.  
**Cons:** Large files held in memory; mitigated by size cap (e.g. 1MB).

### 4.2 Constraints

- Max content size: **1 MB** (align with OpenAPI “docs under 1MB”).
- Show warning if file exceeds limit: "File too large (max 1MB). Use a shorter file or paste content inline."
- Reject non-.md by `accept` and/or extension check.

---

## 5. Inline Markdown Editor

**MVP:** Plain `<textarea>` with `font-family: monospace`.

**Future (out of scope):**

- Syntax highlighting (e.g. lightweight library).
- Live preview pane.
- Markdown toolbar (bold, list, etc.).

For this phase, a basic textarea is enough.

---

## 6. Document Type Selection

User must choose `doc_type` when attaching a document. Default: `context_doc`.

| Value       | Description                         |
|------------|-------------------------------------|
| rule       | Constraints / rules                 |
| skill      | How-to / procedural knowledge       |
| context_doc| Context (PRD, specs, instructions)   |
| reference  | Reference material                  |

**UI:** `select` with labels, e.g. "Context document", "Rule", "Skill", "Reference".

---

## 7. Files to Change

| File | Changes |
|------|---------|
| `cmd/api-gateway/dashboard/index.html` | Add Create Agent button to `section-agents` header; add agent modal markup |
| `cmd/api-gateway/dashboard/static/app.js` | Modal open/close, form handling, authFetch for POST /agents + POST /agents/{id}/documents, file reader, refresh |
| `cmd/api-gateway/dashboard/static/style.css` | Styles for modal and document tabs (optional) |

### 7.1 HTML Changes

**Agents section header**

```html
<!-- Before -->
<section id="section-agents" class="dashboard-card card-agents">
  <h2 class="section-title">Agents</h2>

<!-- After -->
<section id="section-agents" class="dashboard-card card-agents">
  <div class="section-header-row">
    <h2 class="section-title">Agents</h2>
    <button type="button" id="btn-create-agent" class="action-btn approve">Create Agent</button>
  </div>
```

**New modal** — Add `#agent-modal` after `#user-modal`, using `goal-modal` structure.

### 7.2 JavaScript Changes

- Bind `btn-create-agent` → show modal, reset form.
- Bind close (X, backdrop) → hide modal.
- Bind `agent-modal-save` → validate → POST /agents → optionally POST documents → close → refresh.
- `authFetch` base: same as rest of dashboard (relative URLs).
- After success: `loadDashboardSnapshot()` so agents list refreshes.

### 7.3 Implementation Owner

**Tech Lead** coordinates; **UI/UX Expert** or **Go Engineer** implements the dashboard changes (HTML/JS/CSS). No backend implementation unless we add optional `visibility` / `chat_capable` to POST /agents.

---

## 8. Edge Cases & Validation

### 8.1 Agent Creation

- **astra-global-*** prefix: only for super admins (handled by backend).
- **name** required; **actor_type** required.
- **config**: warn if invalid JSON; still send string as-is (backend may accept or reject).

### 8.2 Documents

- Document optional: user can create agent without any document.
- If document provided: `doc_type` and `doc_name` required; `content` required (inline or from file).
- Inline empty after trimming → treat as "no document".
- Both inline and file: prefer file if user uploaded; otherwise use inline (no simultaneous source).

### 8.3 File Upload

- Empty file → show error.
- Non-UTF-8: `FileReader` may fail; catch and show generic error.
- Very large file → enforce 1MB limit before sending.

### 8.4 Network

- Disable submit button during requests; show loading state.
- Timeout / network error → show error, re-enable submit.

### 8.5 Multi-Tenant

- Superadmin dashboard creates agents via `POST /agents` (no org). `org_id` stays null; visibility is global for superadmin context.
- Org context is out of scope for this superadmin feature; future org dashboard would use `POST /org/api/agents`.

---

## 9. Security Compliance

| Constraint | Status |
|-----------|--------|
| S1 (mTLS) | N/A — browser → gateway |
| S2 (JWT) | PASS — authFetch includes Bearer token |
| S3 (RBAC) | PASS — superadmin only; astra-global-* enforced by backend |
| S4 (sandbox) | N/A |
| S5 (secrets) | PASS — no secrets in form or documents |
| S6 (approval) | N/A |

---

## 10. Summary

- **Create Agent** button in agents section header.
- Modal with agent fields (name, actor_type, system_prompt, config) and optional document (inline or .md upload).
- Two-step API: POST /agents, then POST /agents/{id}/documents.
- File upload via FileReader, content sent as JSON `content`.
- Optional visibility and chat_capable only if backend is extended.
- Vanilla JS, `goal-modal` pattern, `authFetch`; no new backend endpoints required for core flow.
