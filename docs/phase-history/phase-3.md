# Phase 3 — Memory & LLM Routing

**Status:** Complete  
**Date:** 2026-03-09

## What was built

### WP3.1 — Memory store and pgvector (`internal/memory/`)
- Store now supports `Write(ctx, agentID, memType, content, embedding)` with optional embedding.
- Vector search implemented using `embedding <=> query::vector` with 1536-dim validation fallback to created_at ordering.
- Added `GetByID` and vector conversion helpers for bytes/float32 conversions.

### WP3.2 — Embedding pipeline (`internal/memory/embedding.go`)
- Added `Embedder` interface.
- Added deterministic `StubEmbedder` for local/dev and tests.
- Added `CachedEmbedder` backed by Memcached with keys `embed:{sha256(content)}` and TTL support.
- Memory writes can auto-generate embeddings when embedder is configured.

### WP3.3 — LLM router (`internal/llm/`)
- Added `Complete()` flow with model resolution, usage metadata, and response caching.
- Added `LLMBackend` abstraction and `StubBackend` fallback.
- Added cache key strategy `llm:resp:{model}:{sha256(prompt)}`.
- Added token/cost metrics (`astra_llm_token_usage_total`, `astra_llm_cost_dollars`).

### WP3.4 — memory-service (`cmd/memory-service`)
- Implemented gRPC MemoryService (`WriteMemory`, `SearchMemories`, `GetMemory`).
- Added proto definitions in `proto/memory/memory.proto` and generated stubs.
- Service runs on `MEMORY_GRPC_PORT` (default 9092).

### WP3.5 — llm-router service (`cmd/llm-router`)
- Implemented gRPC LLMRouter `Complete` endpoint.
- Added proto definitions in `proto/llm/llm.proto` and generated stubs.
- Service runs on `LLM_GRPC_PORT` (default 9093).

### WP3.6 — prompt-manager (`cmd/prompt-manager`, `internal/prompt/`)
- Implemented prompt store (`GetPrompt`, `SavePrompt`, `ListByName`) using migration `0011_prompts.sql`.
- Implemented HTTP service with cache-aside prompt reads and write-through updates.
- Service runs on `PROMPT_MANAGER_PORT` (default 8084).

### WP3.7 — Cache-aside for task reads (`internal/tasks/cache.go`, `cmd/task-service`)
- Added `CachedStore` with Redis-backed cache keys `task:{id}` and `graph:{id}`.
- `GetTask` and `GetGraph` now support cache-hit fast path.
- Write transitions invalidate relevant keys to keep cache coherence.

## Operational updates

- `scripts/deploy.sh` updated to build/start Phase 3 services.
- `scripts/validate.sh` updated to validate Phase 3 service availability and core structural checks.

## Verification

- `go build ./...` passes.
- `go test ./... -short` passes.
- Phase 3 checks in `scripts/validate.sh` are now real tests (no placeholders).
