# Review Code

Perform a structured code review on recent changes to Astra. Optionally specify a branch, commit range, or file path. If nothing specified, review uncommitted changes.

## Steps

### Step 1 — Identify Changed Files

Run `git diff --name-only` and classify each file:
- `internal/actors/**`, `internal/tasks/**`, `internal/scheduler/**`, `internal/messaging/**`, `internal/events/**` → Kernel scope
- `internal/agent/**`, `internal/planner/**`, `internal/memory/**`, `internal/workers/**`, `internal/tools/**`, `internal/evaluation/**` → Service scope
- `cmd/**` → Entrypoint scope
- `pkg/**` → Shared library scope
- `proto/**` → API contract scope
- `migrations/**` → DB schema scope
- `deployments/**` → Infrastructure scope

### Step 2 — Architecture Review

For each changed file, check:
- **Microkernel boundary**: kernel vs user-space separation respected?
- **S1 (mTLS)**: All inter-service calls over mTLS?
- **S2 (JWT)**: External APIs requiring auth?
- **S3 (RBAC/OPA)**: Policy checks in place?
- **S4 (sandbox)**: Tool executions properly isolated?
- **S5 (secrets)**: No secrets in code or logs?
- **S6 (approval gates)**: Dangerous ops gated?
- **Performance**: No Postgres on hot path? Reads from cache?

### Step 3 — Implementation Quality Review

For each Go file, check:
- Proper error wrapping (`fmt.Errorf("context: %w", err)`)
- No swallowed errors
- `context.Context` as first parameter on all I/O functions
- Structured logging (`slog`), no `fmt.Println`
- No `panic` in library code
- Actor mailbox sends non-blocking
- Task state transitions transactional
- Redis consumer groups acknowledge messages
- New exported functions have unit tests (table-driven)

### Step 4 — Proto & Migration Review

If `.proto` files changed: backward compatible? `buf lint` pass?
If `migrations/` changed: idempotent? No destructive changes? Proper indexes?

## Output

Group findings by severity:

### Critical (must fix before merge)
- Security violations (S1-S6), microkernel boundary violations, data loss risk, missing error handling, proto breaks

### Warning (should fix)
- Missing tests, Postgres on read path, missing logging, non-blocking send not used, missing context propagation

### Suggestion (nice to have)
- Style improvements, documentation gaps, refactoring opportunities
