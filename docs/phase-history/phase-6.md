# Phase 6 — SDK & Applications

**Status:** Complete  
**Date:** 2026-03-09

## What was built

### WP6.1 — SDK context and core client shape
- Added `pkg/sdk/context.go` with public `AgentContext`, `TaskSpec`, `Event`, and tool result types.
- Added `pkg/sdk/client.go` with `Config`, defaults, and `NewAgentContext` wiring to kernel/task/memory services.
- Added actor bootstrap behavior so SDK can spawn an actor when `AgentID` is not provided.

### WP6.2 — Memory and tool clients
- Added `pkg/sdk/memory.go` with `MemoryClient` (`Write`, `Search`, `GetByID`) backed by memory-service gRPC.
- Added float32 embedding encode/decode helpers for memory payload transport.
- Added `pkg/sdk/tool.go` with `ToolClient` (`Execute`) backed by tool-runtime HTTP API.

### WP6.3 — SimpleAgent reference application
- Added `examples/simple-agent/main.go` implementing a minimal plan/execute/reflect loop.
- Added `examples/simple-agent/README.md` with run instructions.

### WP6.4 — SDK docs and second example
- Added `pkg/sdk/README.md` documenting interfaces and usage.
- Added `examples/echo-agent/main.go` as a minimal second runnable SDK sample.
- Added `examples/README.md` for consolidated examples guidance.

### WP6.5 — Goal helper utilities
- Added `pkg/sdk/goal.go` with optional HTTP goal-service helper (`CreateGoal`, `WaitForCompletion`).

## Operational updates

- Updated `scripts/validate.sh` Phase 6 section with concrete assertions for:
  - SDK files/interfaces/docs present
  - `go build ./pkg/sdk/...`
  - no `astra/internal/*` dependency from SDK (`go list -deps` check)
  - examples presence
- Updated `docs/PRD.md` to mark Phase 6 complete.

## Verification

- `go build ./pkg/sdk/...` passes.
- `go test ./pkg/sdk/...` passes.
- `scripts/validate.sh` includes Phase 6 checks and no longer has placeholders.
