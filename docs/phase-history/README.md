# Development phase history

This directory holds a **human-readable history of what was built in each development phase**. It is separate from:

- **Runtime phase runs** — When Astra agents execute goals, each run is recorded in the DB (`phase_runs`, `phase_summaries`) and in audit logs (`events`). See `docs/phase-history-usage-audit-design.md`.
- **Vector DB** — Runtime phase summaries are embedded and stored in `phase_summaries` (pgvector) for semantic search. Development-phase notes can optionally be ingested into the same or a separate index for “what did we build?” queries.

Here we maintain one file per **development** phase (Phase 0, Phase 1, …) describing:

- Goals and scope
- What was implemented (components, migrations, config)
- Decisions and trade-offs
- How it ties to the PRD and audit/usage design

Use this for onboarding, handoffs, and to feed agent memory or vector search over “what was done” across phases.
