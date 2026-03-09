# Phase 7 — Security Compliance & Production Auth

**Status:** Complete  
**Date:** 2026-03-09

## What was built

### WP7.1 / WP7.2 — gRPC transport security
- Added `pkg/grpc/tls.go` with config-driven TLS/mTLS support for gRPC servers and clients.
- Updated gRPC services to initialize servers through `grpc.NewServerFromConfig(cfg)`:
  - `cmd/agent-service/main.go`
  - `cmd/task-service/main.go`
  - `cmd/memory-service/main.go`
  - `cmd/llm-router/main.go`
- Updated API gateway gRPC client dials to use shared TLS-aware helper.

### WP7.3 — HTTP transport security
- Added `pkg/httpx/httpx.go` with TLS-aware `ListenAndServe` and shared HTTP client construction.
- Updated HTTP services to use TLS-aware listener path when `ASTRA_TLS_ENABLED=true`.
- Updated internal HTTP callers (auth middleware and approval gate) to use shared TLS-aware HTTP clients.

### WP7.4 / WP7.5 — Vault + config abstraction
- Added `pkg/secrets/vault.go` for Vault KV loading.
- Extended `pkg/config/config.go` with:
  - TLS fields (`TLSEnabled`, cert/key/CA paths, server name, skip-verify)
  - Vault fields (`VaultAddr`, `VaultToken`, `VaultPath`)
  - Vault overlay logic for DB/Redis/Memcached/JWT/TLS settings.

### WP7.7 — Validation and runbooks
- Added runbooks:
  - `docs/runbooks/tls-rotation.md`
  - `docs/runbooks/vault-setup.md`
- Updated `docs/runbooks/README.md` index.
- Added Phase 7 checks to `scripts/validate.sh` for TLS/Vault implementation presence.

## Verification

- `go build ./...` passes.
- `scripts/validate.sh` now includes Phase 7 checks.
- `docs/PRD.md` Phase 7 marked complete.
