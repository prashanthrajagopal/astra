# Changelog

All notable changes to Astra are documented in this file.

## [Unreleased] — 2026-04-02

### Added — Olympus (LA28) Platform Features

#### Goal-Level Dependencies & Cascades
- Migration `0026_goal_dependencies.sql`: `cascade_id`, `depends_on_goal_ids UUID[]`, `completed_at`, `source_agent_id` on goals table
- Goal dependency engine (`internal/goals/deps.go`): activates blocked goals when all dependencies complete
- `GoalCompleted` events published to `astra:goals:completed` Redis stream from scheduler and goal-service
- `POST /goals` accepts `cascade_id`, `depends_on_goal_ids`, `source_agent_id` fields
- `GET /goals/{id}` returns cascade and dependency fields

#### External Agent Adapter Framework
- Adapter interface (`internal/adapters/adapter.go`): `DispatchGoal`, `PollStatus`, `HandleCallback`, `ListCapabilities`, `HealthCheck`
- Thread-safe adapter registry (`internal/adapters/registry.go`)
- Base adapter with HTTP client and retry (`internal/adapters/base.go`)
- D.TEC adapter service (`cmd/dtec-adapter`) on port 8098
- Execution worker delegates to adapters when task has `provider_type != "astra_agent"`

#### Webhook Ingest Service
- New service `cmd/webhook-ingest` on port 8099
- `POST /webhooks/{source_id}` with HMAC-SHA256 signature validation
- `webhook_sources` table for per-source configuration
- Publishes normalized events to `olympus:triggers:raw` Redis stream
- Proxied through API gateway

#### Agent-to-Agent Goal Posting
- `POST /internal/goals` endpoint for service-to-service goal creation
- Redis rate limiting (100 goals/minute per source agent)
- `source_agent_id` tracking in goals table
- SDK client (`pkg/sdk/goals.go`): `PostGoal`, `GetGoal` methods

#### Dual-Approval (Two-Person Rule)
- Migration `0028_dual_approval.sql`: `required_approvals`, `approvals JSONB` on approval_requests
- `POST /approvals/{id}/decide` endpoint with individual approver tracking
- Quorum logic: promotes to approved when `approvals.count >= required_approvals`
- Duplicate voter prevention
- Plan application triggered on quorum approval

#### Trust Score Infrastructure
- Migration `0027_agent_trust_and_tags.sql`: `trust_score FLOAT DEFAULT 0.5`, `tags TEXT[]`, `metadata JSONB` on agents
- GIN index on tags for fast filtering
- Policy gating in access-control: low trust score (`< 0.3`) requires approval

#### Agent Tags & Metadata
- `GET /agents?tag=` filter parameter (hits GIN index)
- `PATCH /agents/{id}` supports `tags` and `metadata` fields

#### Tool Definitions Registry
- Full CRUD store (`internal/toolregistry/store.go`): Create, Get, List, Update, Delete, GetRiskTier
- Risk tier lookup across tool versions (returns most restrictive)

#### Chat Message Injection
- `POST /chat/sessions/{id}/inject` for programmatic message insertion from external services

#### Goal Priority in Scheduler
- Priority field included in task shard stream messages for ordering

### Added — E2E Test Suite

- Playwright test infrastructure (`tests/e2e/`) with 236 tests across 13 spec files
- Chromium + WebKit browser testing, dedicated API test project
- Test coverage: authentication, dashboard UI (stats, charts, navigation, theme toggle, sidebar), agent/goal/approval management, services/workers/cost/PIDs tables, Slack config, chat widget, REST API endpoints (health, agents CRUD, goals, approvals, webhooks, tool registry, chat injection)
- Shared helpers with JWT auth, agent lifecycle, dashboard navigation utilities

### Changed — UI Theme

- Replaced Material Design 3 theme with Apple/macOS glassmorphism for both light and dark modes
- `backdrop-filter: blur(20px) saturate(180%)` on all cards, nav, modals, sidebar, tables
- Apple system font stack (`-apple-system, BlinkMacSystemFont, 'SF Pro Display'...`)
- Apple color palette: `#0A84FF` (blue), `#30D158` (green), `#FF453A` (red), `#007AFF` (light blue)
- Translucent `rgba()` backgrounds with 0.5px borders
- Larger border radii: 16px cards, 20px modals, 12px buttons
- Gradient orbs (blue + purple radial gradients) for depth
- Login page updated with frosted glass card and Apple styling
- Fixed light mode text contrast: solid colors instead of low-opacity rgba

### Changed — Documentation

- Consolidated 61 markdown files into single `docs/PRD.md` (2,640 lines)
- Added Olympus Gap Analysis section with implementation status for all 28 features
- Added Appendix A (Phase History 0-10), B (Design Specs), C (Deployment & Ops), D (Codegen)
- Removed all individual spec, design, phase history, implementation plan, and runbook files
- Updated Phase 12 (Slack) with per-deliverable completion status

### Changed — Data Models

- `Goal` struct: added `CascadeID`, `DependsOnGoalIDs`, `CompletedAt`, `SourceAgentID`
- `Agent` struct: added `TrustScore`, `Tags`, `Metadata`, `SystemPrompt`

## [0.7.5] — 2026-03-31

- UI updates

## [0.7.4] — 2026-03-31

- General updates

## [0.7.3] — 2026-03-30

- Dashboard fixes

## [0.7.2] — 2026-03-29

- Route goal creation through goal-service, fix agent delete

## [0.7.1] — 2026-03-28

- Fix approval list null fields, dashboard chart/status colors, approval button layout

## [0.7.0] — 2026-03-27

- Updated gitignore

## [0.6.0] — 2026-03-26

- Initial release with chat agents, voice support, Slack integration design
