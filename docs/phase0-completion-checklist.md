# Phase 0 Completion Checklist — Astra Prep

**Audience:** Tech Lead (assign to Go Engineer, DevOps Engineer, QA Engineer as needed).  
**Source:** PRD Section 25 (Phase 0), Section 4 (monorepo layout).  
**Acceptance (PRD):** `docker compose up` starts all infra; `go build ./...` succeeds; migrations applied.

---

## 1. Current State Assessment

### 1.1 Already in place

| Item | Status | Notes |
|------|--------|--------|
| **Go module** | Done | `go.mod` (module `astra`, Go 1.25.0), `go.sum` present |
| **Directory layout (Section 4)** | Done | `cmd/` (all 16 services), `internal/` (actors, agent, kernel, planner, scheduler, tasks, memory, workers, tools, evaluation, events, messaging, llm), `pkg/` (db, config, logger, metrics, grpc, models, otel), `proto/`, `migrations/`, `deployments/`, `scripts/`, `docs/` |
| **docker-compose** | Done | Postgres (pgvector/pg17), Redis, Memcached, MinIO; volumes for pgdata, miniodata |
| **Migrations** | Done | `0000_extensions.sql` through `0008_constraints.sql`; use `IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS` (idempotent) |
| **Proto definitions** | Done | `proto/kernel/kernel.proto`, `proto/tasks/task.proto` with `go_package`; `buf.yaml`, `buf.gen.yaml`; generated `*.pb.go` in `proto/kernel/`, `proto/tasks/` |
| **Deploy script** | Done | `scripts/deploy.sh`: native-first infra, runs all migrations, builds api-gateway + scheduler-service, starts them |
| **Deployments** | Done | `deployments/helm/astra/` (Chart.yaml, values.yaml, deployment.yaml, service.yaml) |
| **Dockerfile** | Done | Multi-stage build; ARG SERVICE; copies migrations |
| **.cursor/** | Done | Agents (10), rules (6), skills (7), commands (6), devops-deployment skill, deploy command |

### 1.2 Missing or incomplete

| Item | Gap |
|------|-----|
| **Proto-generated Go code** | No `buf.yaml`, `buf.gen.yaml`, or generated `.pb.go` files. Proto stubs are not generated; no package under `proto/` or `gen/` is importable for gRPC. |
| **CI pipeline** | No `.github/workflows` at repo root. PRD requires: go vet, lint, test (e.g. GitHub Actions or similar). |
| **Full-tree build verification** | `scripts/deploy.sh` only builds `cmd/api-gateway` and `cmd/scheduler-service`. Phase 0 acceptance requires `go build ./...` to pass for the entire module. |
| **Optional: .env.example** | `deploy.sh` references `.env.example` for creating `.env`; if not present, that branch is skipped (no blocker). |

---

## 2. Phase 0 Completion Checklist (for Tech Lead)

Execute in dependency order. Assign each to the appropriate role (Go Engineer, DevOps, QA).

### 2.1 Proto stubs and generated code

- [ ] **Add buf config (Go Engineer)**  
  - Add `buf.yaml` at repo root (or in `proto/`) per [buf docs](https://buf.build/docs/configuration/v1/buf-yaml): `version: v1`, `modules: [ { path: proto } ]` (or equivalent so `proto/` is the module root).  
  - Add `buf.gen.yaml` defining the Go plugin (and optionally grpc-go) so that `buf generate` emits Go code under a path that matches the proto `go_package` (e.g. `astra/proto/kernel`, `astra/proto/tasks` or a single `astra/proto/gen` with a suitable mapping).  
  - Ensure generated code lives inside the repo (e.g. `proto/gen/go` or per-package directories) so `go build ./...` can compile it.

- [ ] **Generate and commit proto Go stubs (Go Engineer)**  
  - Run `buf generate` (and `buf lint` if available); fix any lint errors.  
  - Commit generated `.pb.go` (and if used, `_grpc.pb.go`) files so that CI and `go build ./...` succeed without requiring buf in CI for the initial Phase 0 acceptance.  
  - Document in `docs/` or README: “To regenerate proto: run `buf generate` from repo root.”

- [ ] **CI: buf lint and generate (DevOps / Go Engineer)**  
  - Optional for Phase 0: add a CI step that runs `buf lint` and `buf generate` and fails if generated code is out of date (e.g. diff check). If not in Phase 0, add in a follow-up.

### 2.2 Go build and vet

- [ ] **Verify `go build ./...` (Go Engineer)**  
  - From repo root, run `go build ./...`. Fix any broken packages or missing imports.  
  - Ensure all `cmd/*` and any packages that depend on generated proto (once added) build.  
  - If proto is generated under a path that is not yet imported by any `cmd/`, the build should still pass; once services start using gRPC, keep `go build ./...` green.

- [ ] **Add `go vet` to CI (DevOps / Go Engineer)**  
  - CI workflow must run `go vet ./...` (or equivalent) for the whole module.

### 2.3 Lint

- [ ] **Add golangci-lint (or equivalent) to CI (DevOps / Go Engineer)**  
  - Run `golangci-lint run ./...` (or the linter specified in GLOBAL-ENGINEERING-STANDARD: `golangci-lint`, `staticcheck`).  
  - Add a config file if needed (e.g. `.golangci.yml`) and commit it; ensure CI uses it so PRs are checked.

### 2.4 Tests

- [ ] **Add `go test ./...` to CI (DevOps / QA)**  
  - CI must run `go test ./...` (or `go test -race ./...` if desired). Tests can be stubs initially; the pipeline must be in place and passing.

### 2.5 CI workflow location and content

- [ ] **Create GitHub Actions workflow (DevOps Engineer)**  
  - Create `.github/workflows/` at repo root (not under `packages/` or other subprojects).  
  - Single workflow (e.g. `ci.yml`) is sufficient for Phase 0: on push/PR to main (or appropriate branch), run:  
    - `go vet ./...`  
    - `golangci-lint run ./...` (and/or `staticcheck`)  
    - `go test ./...`  
  - Optionally: `go build ./...` as an explicit step.  
  - Use a Go version consistent with `go.mod` (e.g. 1.25).  
  - Cache Go module download (e.g. `actions/setup-go` with cache) to keep runs fast.

### 2.6 Infra and migrations (acceptance)

- [ ] **Confirm infra and migrations (DevOps / QA)**  
  - From repo root, `docker compose up -d` brings up Postgres, Redis, Memcached, MinIO.  
  - Run `scripts/deploy.sh` (or equivalent migration path): all migrations in `migrations/*.sql` apply in order without error (idempotent).  
  - No need to change migration content for Phase 0 if already idempotent.

### 2.7 Optional / nice-to-have

- [ ] **`.env.example`**  
  - If `scripts/deploy.sh` is the standard path for local runs, add a minimal `.env.example` (e.g. `POSTGRES_*`, `REDIS_ADDR`, `MEMCACHED_ADDR`, `HTTP_PORT`, `GRPC_PORT`, `LOG_LEVEL`) so new contributors can `cp .env.example .env` and run deploy.

- [ ] **Dockerfile and Go version**  
  - Align Dockerfile Go version with `go.mod` (e.g. 1.25) if it currently references an older version (e.g. 1.22).

---

## 3. Dependency order summary

1. **Proto:** Add `buf.yaml` + `buf.gen.yaml` → run `buf generate` → commit generated Go.  
2. **Build:** Run `go build ./...` and fix any failures (including after proto is added).  
3. **CI:** Add `.github/workflows/ci.yml` (or equivalent) with vet, lint, test, and optionally build.  
4. **Acceptance:** Verify `docker compose up`, migrations applied, `go build ./...` passes (and CI green).

---

## 4. Sign-off

When all items above are done:

- [ ] Infra: `docker compose up` starts Postgres, Redis, Memcached, MinIO.  
- [ ] Migrations: All `migrations/*.sql` apply in order (e.g. via `scripts/deploy.sh`).  
- [ ] Build: `go build ./...` passes from repo root.  
- [ ] CI: One workflow runs `go vet`, linter, and `go test` (and optionally `go build`) and is green on main (or target branch).

Phase 0 is then complete per PRD Section 25.
