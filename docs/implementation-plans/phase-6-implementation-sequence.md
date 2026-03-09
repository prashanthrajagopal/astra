# Phase 6 — Concrete Implementation Sequence

**Scope:** WP6.1–WP6.4 (minimum); WP6.5 optional if small.  
**Constraint:** SDK under `pkg/sdk`, no imports from `internal/*`.  
**Reference:** PRD §15, §25; `docs/implementation-plans/phase-6.md`.

---

## 1. Implementation Order

| Step | Work Package | Deliverables |
|------|--------------|--------------|
| 1 | WP6.1 | `pkg/sdk` structure, AgentContext interface, DefaultAgentContext |
| 2 | WP6.2 | MemoryClient, ToolClient interfaces and implementations |
| 3 | WP6.3 | SimpleAgent example (runnable) |
| 4 | WP6.4 | SDK docs + second example |
| 5 | WP6.5 (optional) | Goal helpers: CreateGoal, WaitForCompletion |
| 6 | — | validate.sh Phase 6 checks, PRD §25 update |

---

## 2. Checklist with Target File Paths

### WP6.1 — SDK package structure and AgentContext

| # | Task | Target Path | Acceptance |
|---|------|--------------|------------|
| 1 | Create `pkg/sdk`; define AgentContext interface with ID(), Memory(), CreateTask(), PublishEvent(), CallTool(); define Goal, Task, Event, ToolResult types | `pkg/sdk/context.go` | Interface matches PRD §15; types in SDK package |
| 2 | Implement DefaultAgentContext: agent ID, gRPC clients (task-service, kernel), HTTP client for tool-runtime; constructor NewAgentContext(opts) | `pkg/sdk/client.go` | Constructor accepts agentID, endpoints (env or opts); ID() returns UUID |
| 3 | CreateTask: call task-service CreateTask gRPC (proto/tasks) | `pkg/sdk/client.go` | Creates task via gRPC; no internal import |
| 4 | PublishEvent: call kernel PublishEvent gRPC (proto/kernel) | `pkg/sdk/client.go` | Publishes event via gRPC |
| 5 | CallTool: HTTP POST to tool-runtime /execute (name, input base64, timeout) | `pkg/sdk/client.go` | Calls tool-runtime; returns ToolResult |
| 6 | Config: document ASTRA_API_ADDR, ASTRA_GRPC_*, ASTRA_TOOL_RUNTIME, JWT; support TLS opts | `pkg/sdk/client.go` + README | Env vars documented |
| 7 | Unit tests: mock gRPC/HTTP; verify methods call correct RPCs | `pkg/sdk/client_test.go` | Tests pass; no internal import |

**Acceptance (WP6.1):** `go build ./pkg/sdk/...` passes; `go list -deps ./pkg/sdk` has no `internal/` path; AgentContext usable without kernel imports.

---

### WP6.2 — MemoryClient and ToolClient

| # | Task | Target Path | Acceptance |
|---|------|--------------|------------|
| 1 | Define MemoryClient interface: Write(ctx, agentID, memType, content, embedding []float32) (uuid.UUID, error); Search(ctx, agentID, queryEmbedding []byte, topK int) ([]Memory, error); GetByID(ctx, id uuid.UUID) (*Memory, error) | `pkg/sdk/memory.go` | Interface matches PRD §15; nil embedding = optional |
| 2 | Implement DefaultMemoryClient: gRPC client to memory-service (WriteMemory, SearchMemories, GetMemory) | `pkg/sdk/memory.go` | Calls proto/memory; embedding bytes = 1536*4 float32 LE |
| 3 | Define ToolClient interface: Execute(ctx, name string, input json.RawMessage, opts *ToolOpts) (ToolResult, error) | `pkg/sdk/tool.go` | Interface defined |
| 4 | Implement DefaultToolClient: HTTP POST to tool-runtime /execute | `pkg/sdk/tool.go` | Matches tool-runtime JSON: name, input (base64), timeout_seconds |
| 5 | Wire Memory() and ToolClient into DefaultAgentContext; CallTool delegates to ToolClient | `pkg/sdk/client.go` | AgentContext.Memory() returns MemoryClient; CallTool uses ToolClient |
| 6 | Unit tests with in-memory mock or test server | `pkg/sdk/memory_test.go`, `pkg/sdk/tool_test.go` | Tests pass |

**Note:** MemoryClient.Search accepts `queryEmbedding []byte`; memory-service requires 1536-dim vector. For query-string semantic search, SDK can expose `SearchWithEmbedding(ctx, agentID, query string, topK int, embed func(string)([]float32,error))` where caller provides embedder, or document that nil embedding yields created_at-ordered fallback.

**Acceptance (WP6.2):** SDK user can Write/Search/GetByID memory and Execute tools without using internal packages.

---

### WP6.3 — SimpleAgent example

| # | Task | Target Path | Acceptance |
|---|------|--------------|------------|
| 1 | SimpleAgent struct with Plan(ctx, goal) ([]Task, error), Execute(ctx, task) (Result, error), Reflect(ctx, outcome) error | `examples/simple-agent/main.go` or `cmd/simple-agent/main.go` | Struct and methods exist |
| 2 | Plan: return single stub task or call planner via goal-service (POST /goals) | Same | At least one task produced |
| 3 | Execute: for task type "tool", CallTool; for "memory", Memory().Write; else no-op | Same | Executes tool and/or memory tasks |
| 4 | Reflect: Memory().Write summary of outcome | Same | Writes to memory |
| 5 | Main: connect via NewAgentContext, spawn agent (POST /agents), create goal, run Plan→Execute→Reflect loop | Same | Runnable with `go run` against local Astra |
| 6 | README: run Astra (scripts/deploy.sh), set env (API, JWT), run SimpleAgent | `examples/simple-agent/README.md` | Clear run instructions |

**Acceptance (WP6.3):** `go run ./examples/simple-agent` (or cmd/simple-agent) runs against local Astra; creates goal, executes ≥1 task, writes memory.

---

### WP6.4 — SDK documentation and examples

| # | Task | Target Path | Acceptance |
|---|------|--------------|------------|
| 1 | SDK README: quick start, AgentContext, MemoryClient, ToolClient, config (env, TLS), error handling | `pkg/sdk/README.md` | New developer can follow and use SDK |
| 2 | Godoc on all public types and methods | `pkg/sdk/*.go` | `go doc ./pkg/sdk` shows docs |
| 3 | Second example: e.g. echo agent (receives message, writes memory, returns response) or minimal research stub | `examples/echo-agent/main.go` or `examples/research-agent/` | Second runnable example |
| 4 | Document how to run examples against docker-compose / deploy.sh Astra | `examples/README.md` or in each example | Run instructions for both examples |

**Acceptance (WP6.4):** README accurate; two examples with run instructions.

---

### WP6.5 (optional) — Goal helpers

| # | Task | Target Path | Acceptance |
|---|------|--------------|------------|
| 1 | CreateGoal(ctx, goalText) (goalID uuid.UUID, error) | `pkg/sdk/goal.go` | Calls goal-service |
| 2 | WaitForCompletion(ctx, goalID, timeout) error | Same | Polls or subscribes until done |
| 3 | Document in README; SimpleAgent or example uses CreateGoal | README + example | Optional sign-off |

---

### validate.sh and PRD

| # | Task | Target Path | Acceptance |
|---|------|--------------|------------|
| 1 | Replace Phase 6 `skip_test` with real checks | `scripts/validate.sh` | See §3 below |
| 2 | Update PRD §25 Phase 6 checklist | `docs/PRD.md` | SDK, SimpleAgent, docs marked complete |

---

## 3. validate.sh Phase 6 Checks (Exact)

Replace the current Phase 6 block:

```bash
# ═══════════════════════════════════════════════
# PHASE 6 — SDK & Apps (placeholder)
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ PHASE 6: SDK & Apps ═══')"
skip_test "AgentContext SDK"
skip_test "SimpleAgent example runs"
```

With:

```bash
# ═══════════════════════════════════════════════
# PHASE 6 — SDK & Apps
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ PHASE 6: SDK & Apps ═══')"

echo "SDK package:"
assert_eq "pkg/sdk exists" "true" "$(test -d pkg/sdk && echo true || echo false)"
SDK_DEPS=$(go list -deps ./pkg/sdk 2>/dev/null | grep -c 'astra/internal' || echo "0")
assert_eq "pkg/sdk has no internal imports" "0" "$SDK_DEPS"

echo "SDK builds:"
if go build ./pkg/sdk/... 2>/dev/null; then
  assert_eq "go build ./pkg/sdk passes" "0" "0"
else
  assert_eq "go build ./pkg/sdk passes" "0" "1"
fi

echo "SDK types:"
assert_eq "AgentContext defined" "true" "$(grep -q 'type AgentContext interface' pkg/sdk/context.go 2>/dev/null && echo true || echo false)"
assert_eq "MemoryClient defined" "true" "$(grep -q 'type MemoryClient interface' pkg/sdk/memory.go 2>/dev/null && echo true || echo false)"
assert_eq "ToolClient defined" "true" "$(grep -q 'type ToolClient interface' pkg/sdk/tool.go 2>/dev/null && echo true || echo false)"

echo "SimpleAgent example:"
assert_eq "simple-agent example exists" "true" "$(test -f examples/simple-agent/main.go -o -f cmd/simple-agent/main.go && echo true || echo false)"
assert_eq "simple-agent README exists" "true" "$(test -f examples/simple-agent/README.md -o -f cmd/simple-agent/README.md && echo true || echo false)"

echo "SDK docs:"
assert_eq "pkg/sdk README exists" "true" "$(test -f pkg/sdk/README.md && echo true || echo false)"

echo "Second example:"
assert_eq "second example exists" "true" "$(test -f examples/echo-agent/main.go -o -f examples/research-agent/main.go -o -d examples/echo-agent -o -d examples/research-agent && echo true || echo false)"
```

---

## 4. Dependency Summary

- **SDK imports:** `pkg/*`, `proto/*`, `github.com/google/uuid`, `encoding/json`, `google.golang.org/grpc`, stdlib. **No** `internal/*`.
- **Backend services (must be running for examples):** api-gateway, identity, access-control, agent-service, task-service, memory-service, tool-runtime, goal-service. Use `scripts/deploy.sh` per Phase 5.

---

## 5. Sign-Off (Phase 6 complete)

- [ ] SDK package provides AgentContext, MemoryClient, ToolClient; no internal imports.
- [ ] SimpleAgent example runs against Astra; demonstrates Plan/Execute/Reflect.
- [ ] SDK README and godoc complete; at least two examples with run instructions.
- [ ] validate.sh Phase 6 checks pass.
- [ ] PRD §25 Phase 6 items checked off.
