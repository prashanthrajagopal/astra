# Phase 6 — SDK & Applications — Implementation Plan

**Depends on:** Phase 5 complete (or Phase 4 minimum for initial SDK). Acceptance: production hardening, dashboards, runbooks, and scaling in place.

**PRD references:** §15 Astra SDK — Agent API, §16 Agent Taxonomy & Workflows, §25 Phase 6.

---

## 1. Phase goal

Deliver a public Astra SDK (Go) with AgentContext, MemoryClient, ToolClient, and a minimum viable sample application (SimpleAgent) so that application developers can build agent apps on Astra without touching kernel internals. Include SDK documentation and examples.

---

## 2. Dependencies

- **Phase 4 minimum:** goal-service, planner, task-service, memory-service, llm-router available via gRPC or REST.
- **Phase 5** recommended: stable APIs and production hardening.
- **Repo:** New package for SDK (e.g. `pkg/sdk` or `sdk/go/`) that applications import.

---

## 3. Work packages

### WP6.1 — SDK package structure and AgentContext

**Description:** Create SDK package (e.g. `pkg/sdk` or `sdk/go`) with AgentContext interface per PRD §15: ID() uuid.UUID, Memory() MemoryClient, CreateTask(t Task) error, PublishEvent(ev Event) error, CallTool(name string, input json.RawMessage) (ToolResult, error). Implement default AgentContext that talks to api-gateway or direct gRPC to agent-service, task-service, memory-service, tool-runtime. SDK must not import internal/*.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Create package directory; define AgentContext interface and supporting types (Goal, Task, Event, ToolResult). | Go Engineer | pkg/sdk/context.go or agent.go |
| 2 | Implement DefaultAgentContext: holds agent ID, gRPC/client connections to backend services; ID() returns agent UUID. | Go Engineer | pkg/sdk/client.go |
| 3 | CreateTask: call task-service CreateTask (or goal-service CreateGoal) with task spec. | Go Engineer | Same |
| 4 | PublishEvent: call kernel/events PublishEvent or equivalent. | Go Engineer | Same |
| 5 | CallTool: call tool-runtime Execute (or execution path that runs tool). | Go Engineer | Same |
| 6 | Constructor: NewAgentContext(agentID, apiAddr or grpcOpts); document config (env, TLS). | Go Engineer | Same |
| 7 | Unit tests: mock backends; AgentContext methods call correct RPCs. | Go Engineer | Tests |

**Deliverables:** pkg/sdk (or sdk/go) with AgentContext; tests.

**Acceptance criteria:** Application can construct AgentContext and call CreateTask, PublishEvent, CallTool; no internal imports.

---

### WP6.2 — MemoryClient and ToolClient interfaces

**Description:** Implement MemoryClient interface: Write(agentID, memoryType, content, embedding), Search(agentID, query, topK), GetByID(id). ToolClient: Execute(toolName, input) (ToolResult, error). These may be part of AgentContext or separate clients used by AgentContext. PRD §15.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define MemoryClient interface; implement client that calls memory-service gRPC. | Go Engineer | pkg/sdk/memory.go |
| 2 | MemoryClient.Write: support optional embedding; if nil, service may compute (or require). | Go Engineer | Same |
| 3 | MemoryClient.Search: pass query string; service returns semantic search results. | Go Engineer | Same |
| 4 | Define ToolClient or use CallTool on AgentContext; document tool name and input format. | Go Engineer | pkg/sdk/tool.go or context |
| 5 | Unit tests with mock or in-process test server. | Go Engineer | Tests |

**Deliverables:** MemoryClient and ToolClient (or equivalent); tests.

**Acceptance criteria:** SDK user can write and search memory, and call tools, without using internal packages.

---

### WP6.3 — SimpleAgent example

**Description:** Implement SimpleAgent per PRD §15 skeleton: Plan(ctx, goal) returns tasks (stub or using LLM via context), Execute(ctx, task) runs task (e.g. CallTool or simple logic), Reflect(ctx, outcome) updates memory. Package in examples/ or cmd/simple-agent; runnable against local Astra stack.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Create SimpleAgent struct: Plan, Execute, Reflect methods; accept AgentContext. | Go Engineer | examples/simple-agent/ or cmd/simple-agent |
| 2 | Plan: return single task or call planner via context (if SDK exposes PlanGoal). | Go Engineer | Same |
| 3 | Execute: for task type “tool”, CallTool; for “memory”, write memory; else no-op or log. | Go Engineer | Same |
| 4 | Reflect: Memory().Write summary of outcome. | Go Engineer | Same |
| 5 | Main: connect to Astra (env or flags), create AgentContext, create goal, run Plan → Execute → Reflect loop (or delegate to goal-service). | Go Engineer | Same |
| 6 | README: how to run Astra locally and run SimpleAgent. | Go Engineer | README |

**Deliverables:** SimpleAgent example; README.

**Acceptance criteria:** SimpleAgent runs against local Astra; creates goal, executes at least one task, writes memory.

---

### WP6.4 — SDK documentation and examples

**Description:** Document SDK: installation (go get), configuration (endpoints, auth), AgentContext usage, MemoryClient, CreateTask/CallTool, error handling. Add at least one more example (e.g. “echo agent” or “research agent” stub). PRD §25: SDK documentation and examples.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | SDK README: quick start, AgentContext, MemoryClient, ToolClient, config (env vars, TLS). | Go Engineer | pkg/sdk/README.md or docs/sdk.md |
| 2 | Godoc or doc comments on all public types and methods. | Go Engineer | Code |
| 3 | Second example: e.g. echo agent (receives message, writes to memory, returns response) or minimal “research” flow. | Go Engineer | examples/ |
| 4 | Document how to run examples against docker-compose Astra. | Go Engineer | docs/ or examples/README |

**Deliverables:** SDK README; godoc; two examples; run instructions.

**Acceptance criteria:** New developer can follow README to use SDK and run an example; docs are accurate.

---

### WP6.5 — Optional: Goal and planner helpers in SDK

**Description:** Expose goal creation and plan retrieval in SDK so apps can CreateGoal(goalText) and wait for or poll task completion. May wrap goal-service and task-service calls. Simplifies “submit goal and wait” flow for SDK users.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | SDK method: CreateGoal(ctx, goalText) (goalID, error); optionally WaitForCompletion(ctx, goalID, timeout). | Go Engineer | pkg/sdk/goal.go |
| 2 | Document usage in README. | Go Engineer | Same |
| 3 | SimpleAgent or example uses CreateGoal. | Go Engineer | Example update |

**Deliverables:** Goal helpers in SDK; optional for Phase 6 minimum.

**Acceptance criteria:** SDK user can create goal and optionally wait for completion without calling gRPC directly.

---

## 4. Delegation hints

| Work package | Primary owner | Hand-off |
|--------------|---------------|----------|
| WP6.1 AgentContext | Go Engineer | Foundation for all SDK usage |
| WP6.2 MemoryClient/ToolClient | Go Engineer | Used by AgentContext and examples |
| WP6.3 SimpleAgent | Go Engineer | Example for docs |
| WP6.4 Documentation | Go Engineer | Review by Tech Lead |
| WP6.5 Goal helpers | Go Engineer | Optional |

---

## 5. Ordering within Phase 6

1. **First:** WP6.1 (AgentContext) and WP6.2 (MemoryClient, ToolClient).
2. **Then:** WP6.3 (SimpleAgent) using WP6.1 and WP6.2.
3. **Then:** WP6.4 (documentation and second example).
4. **Optional:** WP6.5 (goal helpers) in parallel or after WP6.3.

---

## 6. Risks / open decisions

- **SDK location:** `pkg/sdk` (inside Astra repo) vs separate repo. PRD implies single monorepo; SDK in pkg/ keeps versioning aligned. Confirm with product.
- **Auth in SDK:** SDK must pass JWT or API key to backend; document how (env, config, or explicit token).
- **Python/TS bindings:** PRD §25 says “Python/TS bindings” as ongoing work; Phase 6 minimum is Go SDK only unless scoped otherwise.
- **Versioning:** SDK package versioning (v0.1.0, semver) and compatibility with backend; document in README.

---

## Sign-off (Phase 6 complete)

- [ ] SDK package provides AgentContext, MemoryClient, and tool execution; no internal imports.
- [ ] SimpleAgent example runs against Astra and demonstrates Plan/Execute/Reflect.
- [ ] SDK README and godoc complete; at least two examples with run instructions.
- [ ] Optional: CreateGoal and wait-for-completion helpers in SDK.
- [ ] All SDK tests pass; CI includes SDK package.
