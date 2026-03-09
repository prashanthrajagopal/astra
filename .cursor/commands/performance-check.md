# Performance Check

Audit the Astra codebase for compliance with performance targets. The user may optionally specify a service or package to narrow scope.

## Pre-requisites — Read Before Acting

1. Read `.cursor/skills/api-contract-reference/SKILL.md`
2. Read `.cursor/skills/go-patterns/SKILL.md`
3. Read `.cursor/skills/kernel-reference/SKILL.md`
4. Read `.cursor/rules/PERFORMANCE-RULE.mdc`

## Delegation

Delegate to **Architect** for the audit. The Architect reports findings and delegates fixes through **Tech Lead**.

## Performance Targets

| Metric | Target |
|--------|--------|
| API read response time (p99) | ≤ 10ms |
| Task scheduling latency (median) | ≤ 50ms |
| Task scheduling latency (P95) | ≤ 500ms |
| Worker failure detection | ≤ 30s |
| Event persistence to Postgres | ≤ 1s |
| Control plane API availability | 99.9% |

## Audit Scope

### API Gateway Read Path

Scan `cmd/api-gateway/` and related handlers:
- Every read endpoint must serve from Redis or Memcached
- No `db.Query` or `db.QueryRow` on synchronous read path
- No external HTTP calls, no file I/O
- No synchronous LLM calls

### Scheduler Hot Path

Scan `internal/scheduler/`:
- Ready-task detection uses proper indexes (`idx_tasks_status`, `FOR UPDATE SKIP LOCKED`)
- Push to Redis stream immediately after marking ready
- No unbounded loops or full-table scans
- Shard coordination overhead bounded

### Actor Runtime

Scan `internal/actors/`:
- Mailbox sends are non-blocking (select with default)
- No unbounded goroutine creation
- Supervision restart limited by circuit breaker
- Actor state snapshots are async (not blocking message loop)

### Redis Client Configuration

Check Redis/Memcached client setup:
- Connection timeout configured
- Command timeout bounded
- Connection pool sized for concurrency
- Pipeline used for batch operations

### Task State Machine

Scan `internal/tasks/`:
- State transitions use `FOR UPDATE SKIP LOCKED`
- Events appended in same transaction (no separate round-trip)
- No N+1 queries for dependency checking

### Worker Heartbeat

Scan `internal/workers/`:
- Heartbeat interval ≤ 10s (to detect failure within 30s)
- Heartbeat check is lightweight (Redis key expiry, not Postgres poll)
- Task requeue on heartbeat loss is bounded (max retries)

### Memcached Cache

Verify caching strategy:
- LLM responses cached (`llm:resp:{model}:{hash}`, TTL ~24h)
- Embeddings cached (`embed:{hash}`, TTL 7-30d)
- Tool results cached (`tool:cache:{tool}:{hash}`)
- Cache-miss fallback doesn't hit hot path

## Output

| Component | Status | Issue |
|-----------|--------|-------|
| API read endpoints | PASS/FAIL | {description if fail} |
| Scheduler ready-task detection | PASS/FAIL | {description if fail} |
| Actor mailbox (non-blocking) | PASS/FAIL | {description if fail} |
| Redis client config | PASS/FAIL | {description if fail} |
| Task state transitions | PASS/FAIL | {description if fail} |
| Worker heartbeat | PASS/FAIL | {description if fail} |
| Memcached caching | PASS/FAIL | {description if fail} |
| Connection pools | PASS/FAIL | {description if fail} |

For each FAIL, recommend the fix and which agent should implement it.
