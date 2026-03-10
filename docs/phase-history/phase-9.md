# Phase 9: Agent Profile & Context Management

## Summary

Phase 9 adds agent persona definition (`system_prompt`), agent-attached documents (rules, skills, context docs, references), and end-to-end context assembly from profile + documents into the planner and execution-worker LLM prompts.

## Deliverables

### Database

- **Migration 0013**: `agents.system_prompt` column (TEXT, default ''), `agent_documents` table with `doc_type` (rule, skill, context_doc, reference), `content`/`uri`, `goal_id` for goal-scoped docs, indexes and triggers.

### Internal Packages

- **internal/agentdocs**
  - `store.go`: Store with Redis cache-aside for GetProfile (5min TTL), CreateDocument, ListDocuments, DeleteDocument, UpdateProfile. Graceful degradation when Redis is nil.
  - `context.go`: AssembleContext (profile + global + goal-scoped docs), SerializeContext for task payloads.

### API Gateway

- POST /agents: extended to accept optional `name`, `system_prompt`; updates profile after spawn when system_prompt provided.
- PATCH /agents/{id}: update system_prompt and config.
- GET /agents/{id}/profile: read profile (cache-aside from Redis).
- POST /agents/{id}/documents: create agent document.
- GET /agents/{id}/documents: list with optional ?doc_type= and ?goal_id=.
- DELETE /agents/{id}/documents/{doc_id}: delete document.

### Goal Service

- POST /goals: accepts optional `documents` array; creates goal-scoped documents, assembles AgentContext, passes to planner via PlanOptions.AgentContext.

### Planner

- PlanOptions extended with AgentContext (json.RawMessage).
- parseLLMResponse and fallbackGraph include agent_context in each task payload.
- Planning prompt prefixed with system_prompt when present.

### Execution Worker / Codegen

- TaskPayload extended with AgentContext.
- buildPrompt: prepends system_prompt, adds RULES/SKILLS/CONTEXT sections from agent context when present; backward compatible when absent.

## Backward Compatibility

- Agents without system_prompt continue to work (empty string default).
- Tasks without agent_context in payload use the original prompt format.
