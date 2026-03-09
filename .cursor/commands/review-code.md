# Review Code

Perform a structured code review on recent changes to Astra. The user may specify a branch, commit range, or file path. If nothing specified, review uncommitted changes.

## Pre-requisites — Read Before Acting

1. Read `.cursor/rules/GLOBAL-ENGINEERING-STANDARD.mdc` (production standards)
2. Read `.cursor/rules/SECURITY-RULE.mdc` (security constraints S1-S6)
3. Read `.cursor/rules/PERFORMANCE-RULE.mdc` (performance targets)
4. Read `.cursor/rules/GO-ENGINEER-RULE.mdc` for Go code changes

## Delegation Chain

```
User → Architect   (architecture + security + performance review)
     → Tech Lead   (implementation quality review)
```

Neither agent writes code. They produce findings only.

## Steps

### Step 1 — Identify Changed Files

Run `git diff --name-only` to get the list of changed files.

Classify each file:
- `internal/actors/**`, `internal/tasks/**`, `internal/scheduler/**`, `internal/messaging/**`, `internal/events/**` → Kernel scope
- `internal/agent/**`, `internal/planner/**`, `internal/memory/**`, `internal/workers/**`, `internal/tools/**`, `internal/evaluation/**` → Service scope
- `cmd/**` → Entrypoint scope
- `pkg/**` → Shared library scope
- `proto/**` → API contract scope
- `migrations/**` → DB schema scope
- `deployments/**` → Infrastructure scope

### Step 2 — Architecture Review (Architect)

For each changed file, check:
- **Microkernel boundary**: Does the change respect kernel vs user-space separation?
- **S1 (mTLS)**: Are all inter-service calls over mTLS?
- **S2 (JWT)**: Are all external APIs requiring auth?
- **S3 (RBAC/OPA)**: Are policy checks in place?
- **S4 (sandbox)**: Tool executions properly isolated?
- **S5 (secrets)**: No secrets in code or logs?
- **S6 (approval gates)**: Dangerous ops gated?
- **Performance**: No Postgres on hot path? Reads from cache?

### Step 3 — Implementation Quality Review (Tech Lead)

For each Go file, check:

**Code Quality:**
- Proper error wrapping (`fmt.Errorf("context: %w", err)`)
- No swallowed errors
- `context.Context` as first parameter on all I/O functions
- Structured logging (`slog`), no `fmt.Println` or bare `log.Println`
- No `panic` in library code (only in `main()` for unrecoverable init)

**Correctness:**
- Actor mailbox sends are non-blocking (select with default)
- Task state transitions are transactional (BEGIN/COMMIT)
- Redis consumer groups acknowledge messages
- gRPC methods propagate context cancellation

**Testing:**
- New exported functions have unit tests
- Table-driven tests used
- Edge cases covered (nil, empty, context cancelled, channel full)

**Linting:**
- `go vet ./...` clean
- `golangci-lint run` clean

### Step 4 — Proto Contract Review

If `.proto` files changed:
- Are all existing RPCs backward compatible?
- Are new fields added (not renamed/removed)?
- Does `buf lint` pass?

### Step 5 — Migration Review

If `/migrations/` files changed:
- Are migrations idempotent (`IF NOT EXISTS`)?
- No destructive changes without explicit approval?
- Proper indexes added?
- Foreign key constraints correct?

## Output

Group findings by severity:

### Critical (must fix before merge)
- Security violations (S1-S6)
- Microkernel boundary violations
- Data loss risk in migrations
- Missing error handling on hot paths
- Proto backward compatibility breaks

### Warning (should fix)
- Missing tests for new code
- Postgres queries on read path
- Missing structured logging
- Non-blocking mailbox send not used
- Missing context propagation

### Suggestion (nice to have)
- Code style improvements
- Documentation gaps
- Refactoring opportunities
- Benchmark suggestions
