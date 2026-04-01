# Open Questions

## astra-olympus-gap-closure - 2026-04-01

- [ ] Should the slack-worker be a separate service (cmd/slack-worker) or a consumer mode within cmd/slack-adapter? -- Affects service count in Section 9 header and deploy.sh updates
- [ ] Should Phase 9 roadmap checkboxes be updated to [x] in this same PR, or left as-is since the PRD rule is "additive only"? -- Phase 9 items are currently unchecked despite being implemented; strict "additive only" means not touching them, but it creates a misleading roadmap
- [ ] The spec lists cmd/dtec-adapter as a new service but notes it is "Olympus-owned, not Astra core" -- Should the olympus_adapters table DDL be included in the PRD or excluded since it belongs to Olympus?
- [ ] Goal state machine currently has: created, pending, queued, scheduled, running, completed, failed. Gap 4 introduces a "blocked" state for goals waiting on dependencies -- Should this be documented in Section 7 (Task Graph Engine) as a goal-level state addition?
- [ ] The spec references `internal/goals/deps.go` but the current codebase has goal logic in `cmd/goal-service/` -- Confirm whether `internal/goals/` already exists as a package or needs to be created
