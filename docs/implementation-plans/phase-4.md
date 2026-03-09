# Phase 4 — Orchestration, Eval, Security — Implementation Plan

**Depends on:** Phase 3 complete. Acceptance: memory and LLM routing with 10ms read path; prompt-manager and caches in place.

**PRD references:** §9 Services (planner-service, goal-service, evaluation-service, identity, access-control), §15 SDK (goal/task flow), §18 Security (JWT, OPA, approval gates), §25 Phase 4, §26 Build Order. Design: docs/phase-history-usage-audit-design.md (phase_runs, llm_usage, events).

---

## 1. Phase goal

Replace the planner stub with LLM-driven goal→DAG planning, add goal-service and evaluation-service, implement identity (JWT) and access-control (OPA), and add tool execution approval gates so that goals are ingested, planned, executed, validated, and secured end-to-end.

---

## 2. Dependencies

- **Phase 3** complete: llm-router and memory-service operational; Memcached/Redis for 10ms reads.
- **Internal packages:** internal/planner (replace stub), internal/evaluation; cmd/planner-service, cmd/goal-service, cmd/evaluation-service, cmd/identity, cmd/access-control.
- **Security:** S2 (JWT), S3 (OPA), S6 (approval gates) per security rule.

---

## 3. Work packages

### WP4.1 — Planner: goal → DAG with LLM (internal/planner)

**Description:** Replace stub planner with LLM-based planning: given goal text and agent context, call LLM (via llm-router) to produce a structured task DAG (task types, dependencies, payloads). Use prompt-manager for planning prompt template. Output must be valid for internal/tasks (CreateTask, dependencies). PRD §9: planner-service produces TaskGraphs from goals.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define planning prompt template: input (goal text, agent id, optional context), output (list of tasks with type, payload, depends_on). | Go Engineer | prompt template + internal/planner |
| 2 | Implement Plan(ctx, goalID, agentID, goalText) using llm-router.Complete with planning prompt; parse LLM response into Graph ([]Task, []Dependency). | Go Engineer | internal/planner/planner.go |
| 3 | Validation: ensure DAG is acyclic, task types and payloads valid. | Go Engineer | Same |
| 4 | Store or reference prompt in prompt-manager; support versioning. | Go Engineer | Same |
| 5 | Unit tests: mock LLM returning valid JSON DAG; assert Graph structure. | Go Engineer | Tests |
| 6 | Integration test: real LLM (or fixture) produces DAG; task-service creates tasks. | QA / Go Engineer | Tests |

**Deliverables:** internal/planner with LLM; tests.

**Acceptance criteria:** Planner produces multi-task DAGs from goal text; DAG is acyclic and persistable via task-service.

---

### WP4.2 — Planner service API (cmd/planner-service)

**Description:** Expose planner as gRPC service: Plan(PlanRequest) returns PlanResponse (graph or task IDs). Called by goal-service or agent-service when a goal is accepted. Uses internal/planner and llm-router.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define Plan RPC: request (goal_id, agent_id, goal_text), response (graph or list of task_ids). | Architect / Go Engineer | proto/planner or existing |
| 2 | cmd/planner-service: gRPC server, call internal/planner.Plan, return graph. | Go Engineer | cmd/planner-service/main.go |
| 3 | After planning, caller (goal-service) creates tasks via task-service; planner may return graph only or also create tasks (design choice). | Tech Lead / Go Engineer | Design doc or implementation |
| 4 | Integration test: goal-service submits goal, planner-service returns DAG, tasks created. | QA | Tests |

**Deliverables:** cmd/planner-service; tests.

**Acceptance criteria:** Planner-service returns valid DAG for a goal; goal-service can drive full flow.

---

### WP4.3 — Goal service (cmd/goal-service)

**Description:** Ingest goals (CreateGoal), validate, and route to planner-service. Create goal row (goals table); optionally create phase_runs row (PhaseStarted) per docs/phase-history-usage-audit-design.md; trigger planner; create tasks via task-service; update goal status. Emit PhaseCompleted/PhaseFailed when run finishes.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define CreateGoal, GetGoal, ListGoals (by agent_id). Persist to goals table. | Go Engineer | cmd/goal-service + proto |
| 2 | On CreateGoal or StartGoal: create phase_runs row (status running), emit PhaseStarted event to events table. | Go Engineer | Same |
| 3 | Call planner-service.Plan; call task-service to create tasks and dependencies; trigger scheduler (tasks in pending). | Go Engineer | Same |
| 4 | When all tasks terminal (completed/failed) or timeout: update phase_runs (status, ended_at, summary, timeline), emit PhaseCompleted/PhaseFailed; optional: async write phase log file per design doc. | Go Engineer | Same |
| 5 | Timeline: build from task events (TaskCompleted, TaskFailed) or query tasks at end. | Go Engineer | Same |
| 6 | Integration test: create goal, run to completion, assert phase_runs and events. | QA | Tests |

**Deliverables:** cmd/goal-service; phase lifecycle; tests.

**Acceptance criteria:** Goal is created and planned; tasks run; phase_runs and events reflect lifecycle; PhaseCompleted emitted when done.

---

### WP4.4 — Evaluation service (internal/evaluation, cmd/evaluation-service)

**Description:** Implement evaluators: result validators (e.g. task output meets criteria), auto-evaluators (script or LLM-based). evaluation-service receives task result and evaluator config, returns pass/fail and optional feedback. PRD §9: evaluation-service for result validators and test harnesses.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define Evaluator interface: Evaluate(ctx, taskID, result, criteria) (passed bool, feedback string, error). | Go Engineer | internal/evaluation/eval.go |
| 2 | Implement at least one evaluator: e.g. regex or JSON schema check; or LLM-based “does result satisfy goal?”. | Go Engineer | Same |
| 3 | cmd/evaluation-service: gRPC Evaluate(request) returns EvaluateResponse (passed, feedback). | Go Engineer | cmd/evaluation-service/main.go |
| 4 | Optional: publish to astra:evaluation stream for downstream; append to events. | Go Engineer | Same |
| 5 | Unit tests: evaluator returns pass/fail for known inputs. | Go Engineer | Tests |

**Deliverables:** internal/evaluation; cmd/evaluation-service; tests.

**Acceptance criteria:** Evaluation-service can validate a task result and return pass/fail; integrable with task completion flow (optional hook in Phase 4).

---

### WP4.5 — Identity service (cmd/identity)

**Description:** Implement identity service: issue JWT tokens for users and service accounts. Short-lived tokens; signed with secret from config/Vault. PRD §18: identity handles user/service auth; S2: all external API calls require JWT.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define IssueToken(request) RPC: subject (user/service id), scopes, TTL; return JWT. | Architect / Go Engineer | proto/identity or existing |
| 2 | Implement JWT signing (HS256 or RS256); store secret in config or Vault (no secret in code). | Go Engineer | cmd/identity/main.go |
| 3 | Token payload: sub, exp, iat, scopes (or roles). | Go Engineer | Same |
| 4 | ValidateToken RPC or middleware: verify signature and exp. | Go Engineer | Same |
| 5 | Integration test: issue token, validate; expired token rejected. | QA | Tests |

**Deliverables:** cmd/identity; JWT issue and validate; tests.

**Acceptance criteria:** Identity service issues and validates JWTs; api-gateway can validate tokens (WP4.7).

---

### WP4.6 — Access-control and OPA (cmd/access-control)

**Description:** Integrate OPA for policy enforcement. access-control service: Check(request) returns allow/deny based on policy (RBAC, per-agent scopes). Policies stored in Git or config; loaded at startup or on change. PRD §18: access-control uses OPA; S3: all operations pass OPA checks.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define Check RPC: subject, action, resource, context → allowed bool, reason. | Architect / Go Engineer | proto/access-control |
| 2 | Integrate OPA: load Rego policies; query OPA with input (subject, action, resource). | Go Engineer | cmd/access-control/main.go |
| 3 | Document policy format and example policies (e.g. agent can create tasks for self; admin can do X). | Go Engineer | docs/ or policies/ |
| 4 | Unit test: policy allows/denies as expected. | Go Engineer | Tests |

**Deliverables:** cmd/access-control; OPA integration; tests.

**Acceptance criteria:** access-control returns allow/deny from OPA; api-gateway or services call it before sensitive operations.

---

### WP4.7 — API gateway: JWT and OPA (cmd/api-gateway)

**Description:** Replace placeholder auth with JWT validation and OPA. On each request: extract JWT, call identity.ValidateToken (or local verify); extract subject and scopes; call access-control.Check(action, resource); if denied return 403. S2 and S3 compliance.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Middleware: extract Bearer token, validate via identity service or local JWT verify. | Go Engineer | cmd/api-gateway |
| 2 | Middleware: build OPA input (subject, action, resource from path/method); call access-control.Check; 403 if denied. | Go Engineer | Same |
| 3 | Health and readiness endpoints remain unauthenticated or with optional bypass for k8s. | Go Engineer | Same |
| 4 | Integration test: request with valid JWT and allowed policy returns 200; invalid or denied returns 401/403. | QA | Tests |

**Deliverables:** api-gateway auth middleware; tests.

**Acceptance criteria:** All protected routes require valid JWT and OPA allow; S2 and S3 enforced at gateway.

---

### WP4.8 — Tool execution approval gates (S6)

**Description:** For dangerous actions (e.g. delete, production change), policy engine must require human-in-the-loop approval. access-control or tool-runtime: before executing tool, check if action is “dangerous”; if so, return “approval required” and enqueue for human approval; after approval, allow execution. PRD §18: dangerous actions require approval gates.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define “dangerous” tool/action list or policy (e.g. tag in tool registry, or OPA rule). | Architect / Tech Lead | Design |
| 2 | tool-runtime or execution-worker: before Execute, call access-control with action=tool.Execute, resource=tool_name; if policy says approval_required, persist approval request and return pending. | Go Engineer | internal/tools or cmd/tool-runtime |
| 3 | Implement approval API or workflow: list pending, approve(approval_id); then re-run or release task. | Go Engineer | cmd/access-control or api-gateway |
| 4 | Test: dangerous tool without approval is blocked; after approval, tool runs. | QA | Tests |

**Deliverables:** Approval gate check; approval API; tests.

**Acceptance criteria:** Dangerous tool execution requires approval; S6 satisfied.

---

### WP4.9 — LLM usage persistence and audit (async)

**Description:** Per docs/phase-history-usage-audit-design.md: llm-router already returns usage in response; add async persistence. Publish usage to astra:usage stream; consumer writes to llm_usage table and appends to events (type LLMUsage). No sync DB write on LLM path (10ms preserved).

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | llm-router: after each LLM call, publish message to astra:usage (request_id, agent_id, task_id, model, tokens_in, tokens_out, latency_ms, cost_dollars). | Go Engineer | internal/llm or cmd/llm-router |
| 2 | Consumer (same process or worker): read astra:usage, INSERT into llm_usage, INSERT into events (event_type=LLMUsage, payload). | Go Engineer | cmd/llm-router or dedicated consumer |
| 3 | Migration 0009 already adds llm_usage and events; ensure consumer runs (e.g. part of llm-router or scheduler-service). | DB Architect | Verify migration |
| 4 | Test: trigger LLM call, assert row in llm_usage and event in events (eventual). | QA | Tests |

**Deliverables:** Usage stream + consumer; tests.

**Acceptance criteria:** Every LLM call is eventually persisted to llm_usage and events; hot path unchanged.

---

## 4. Delegation hints

| Work package | Primary owner | Hand-off |
|--------------|---------------|----------|
| WP4.1 planner LLM | Go Engineer | Depends on llm-router, prompt-manager |
| WP4.2 planner-service | Go Engineer | Hand off to goal-service |
| WP4.3 goal-service | Go Engineer | Integrates planner + tasks + phase_runs; QA E2E |
| WP4.4 evaluation | Go Engineer | Optional hook from task completion |
| WP4.5 identity | Go Engineer | Hand off to api-gateway |
| WP4.6 access-control | Go Engineer | Hand off to api-gateway and tool approval |
| WP4.7 api-gateway auth | Go Engineer | QA security tests |
| WP4.8 approval gates | Go Engineer | QA for dangerous-action flow |
| WP4.9 usage audit | Go Engineer | QA for persistence |

---

## 5. Ordering within Phase 4

1. **Parallel:** WP4.1 (planner LLM), WP4.4 (evaluation), WP4.5 (identity), WP4.6 (access-control).
2. **Then:** WP4.2 (planner-service) after WP4.1; WP4.3 (goal-service) after WP4.2 and phase_runs design.
3. **Then:** WP4.7 (api-gateway auth) after WP4.5 and WP4.6.
4. **Then:** WP4.8 (approval gates) after WP4.6 and tool-runtime.
5. **Then or parallel:** WP4.9 (usage persistence) once llm-router is stable.

---

## 6. Risks / open decisions

- **Evaluation scope:** “Auto-evaluators” and “test harness integration” can be minimal in Phase 4 (one LLM-based or regex evaluator) vs full harness. Confirm with product.
- **Phase timeline build:** goal-service must know when “all tasks terminal”; can poll task-service or subscribe to task events. Design: event-driven vs polling.
- **Approval UX:** Who approves (human in loop)? API only, or integrate with a dashboard later? Phase 4 can be API-only (approve by ID).
- **mTLS:** Phase 4 focuses on JWT for external API. S1 (mTLS between services) may be Phase 5; document assumption.

---

## Sign-off (Phase 4 complete)

- [ ] Goal submitted via goal-service → planner generates real DAG (LLM) → tasks executed → evaluator can validate results.
- [ ] Identity issues JWT; access-control enforces OPA; api-gateway validates JWT and OPA.
- [ ] Dangerous tool execution requires approval; approval API works.
- [ ] LLM usage persisted asynchronously to llm_usage and events.
- [ ] Phase lifecycle (phase_runs, PhaseStarted/Completed) recorded.
- [ ] All tests pass; security checklist S2, S3, S6 verified.
