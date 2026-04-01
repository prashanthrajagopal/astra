# Performance Check

Audit the Astra codebase for compliance with performance targets. Optionally specify a service or package to narrow scope.

## Performance Targets

| Metric | Target |
|--------|--------|
| API read response time (p99) | ≤ 10ms |
| Task scheduling latency (median) | ≤ 50ms |
| Task scheduling latency (P95) | ≤ 500ms |
| Worker failure detection | ≤ 30s |
| Event persistence to Postgres | ≤ 1s |

## Audit Scope

### API Gateway Read Path (`cmd/api-gateway/`)
- Every read endpoint must serve from Redis or Memcached
- No `db.Query` or `db.QueryRow` on synchronous read path
- No external HTTP calls, no file I/O, no synchronous LLM calls

### Scheduler Hot Path (`internal/scheduler/`)
- Ready-task detection uses proper indexes (`idx_tasks_status`, `FOR UPDATE SKIP LOCKED`)
- Push to Redis stream immediately after marking ready
- No unbounded loops or full-table scans

### Actor Runtime (`internal/actors/`)
- Mailbox sends non-blocking (select with default)
- No unbounded goroutine creation
- Supervision restart limited by circuit breaker
- Actor state snapshots async (not blocking message loop)

### Redis/Memcached Client Configuration
- Connection timeout configured, command timeout bounded
- Connection pool sized for concurrency
- Pipeline used for batch operations

### Task State Machine (`internal/tasks/`)
- State transitions use `FOR UPDATE SKIP LOCKED`
- Events appended in same transaction
- No N+1 queries for dependency checking

### Worker Heartbeat (`internal/workers/`)
- Heartbeat interval ≤ 10s
- Heartbeat check is lightweight (Redis key expiry, not Postgres poll)
- Task requeue on heartbeat loss is bounded (max retries)

### Memcached Cache
- LLM responses cached (`llm:resp:{model}:{hash}`, TTL ~24h)
- Embeddings cached (`embed:{hash}`, TTL 7-30d)
- Tool results cached (`tool:cache:{tool}:{hash}`)

## Output

| Component | Status | Issue |
|-----------|--------|-------|
| API read endpoints | PASS/FAIL | {description if fail} |
| Scheduler ready-task detection | PASS/FAIL | ... |
| Actor mailbox (non-blocking) | PASS/FAIL | ... |
| Redis client config | PASS/FAIL | ... |
| Task state transitions | PASS/FAIL | ... |
| Worker heartbeat | PASS/FAIL | ... |
| Memcached caching | PASS/FAIL | ... |

For each FAIL, recommend the fix.
