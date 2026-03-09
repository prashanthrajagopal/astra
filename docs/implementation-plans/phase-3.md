# Phase 3 — Memory & LLM Routing — Implementation Plan

**Depends on:** Phase 2 complete. Acceptance: workers execute tasks in sandbox; worker-manager tracks health.

**PRD references:** §9 Services (memory-service, llm-router, prompt-manager), §11 DB (memories, pgvector), §13 Caching & Fast-path, §17 Observability (metrics), §22 Cost/LLM, §25 Phase 3, §26 Build Order. Hot-path: p99 ≤10ms (PRD §24).

---

## 1. Phase goal

Deliver memory-service with pgvector (write, search by embedding), LLM router with model selection and response caching, and prompt-manager; bring hot-path API reads into compliance with 10ms SLA using Redis and Memcached so that agent memory search and LLM calls are fast and cacheable.

---

## 2. Dependencies

- **Phase 2** complete: execution-worker and tool-runtime operational.
- **DB:** `memories` table (migrations/0003_memories.sql) with embedding vector; indexes per 0007.
- **Infra:** Memcached available (docker-compose); Redis already in use.
- **Build order:** internal/memory and internal/llm depend on pkg/* and optionally internal/events for audit later.

---

## 3. Work packages

### WP3.1 — Memory store and pgvector (internal/memory)

**Description:** Implement `internal/memory`: write memory (agent_id, memory_type, content, embedding optional); search by embedding (similarity search on memories.embedding for agent_id, topK). Use migrations/0003_memories.sql; embedding dimension 1536. If embedding not provided on write, either require caller to provide it or integrate with embedding pipeline (WP3.3). PRD §9: memory-service for episodic/semantic memory.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Implement Store: Write(ctx, agentID, memoryType, content, embedding []float32) — INSERT into memories. | Go Engineer | internal/memory/memory.go |
| 2 | Implement Search(ctx, agentID, queryEmbedding []float32, topK int) — vector similarity search (ORDER BY embedding <=> query_embedding LIMIT topK). | Go Engineer | Same |
| 3 | GetByID(ctx, id) for single memory fetch. | Go Engineer | Same |
| 4 | Use pkg/db; support read replicas for Search if configured (optional Phase 3). | Go Engineer | Same |
| 5 | Unit/integration tests: write memory with embedding, search returns correct ordering. | QA / Go Engineer | Tests |

**Deliverables:** internal/memory package; tests.

**Acceptance criteria:** Write persists to memories; Search returns semantically similar items by embedding; GetByID works.

---

### WP3.2 — Embedding pipeline (internal/memory or internal/llm)

**Description:** Provide embedding generation for content (e.g. for memory write and phase_summaries). May call external embedding API or local model; output 1536-dim vector. Cache embeddings in Memcached per PRD §13: `embed:{content_hash}`. Pipeline used by memory-service when content is written without precomputed embedding.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define Embedder interface: Embed(ctx, content string) ([]float32, error). | Go Engineer | internal/memory/embedding.go or internal/llm |
| 2 | Implement one backend: external API (e.g. OpenAI-compatible) or local model; config-driven. | Go Engineer | Same |
| 3 | Cache: key embed:{hash(content)}, TTL 7–30 days; on cache hit return cached vector. | Go Engineer | Memcached client in pkg or internal |
| 4 | Wire into memory Write: if embedding nil, call Embedder.Embed then Write. | Go Engineer | Same |
| 5 | Unit test: mock embedder; cache hit returns same vector. | Go Engineer | Tests |

**Deliverables:** Embedding pipeline with cache; tests.

**Acceptance criteria:** Content can be embedded and cached; memory write can use pipeline when embedding not supplied.

---

### WP3.3 — LLM router (internal/llm)

**Description:** Implement `internal/llm`: route request to model (local vs premium by task type or config), call LLM API, return response. Cache responses in Memcached: key `llm:resp:{model}:{prompt_hash}` per PRD §13. Record token usage (tokens_in, tokens_out, model, latency) for response metadata and async audit (Phase 4 or phase-history design). PRD §22: cost management; §17 metrics.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define Router interface: Complete(ctx, modelHint, prompt, options) (response, usage, error). | Go Engineer | internal/llm/router.go |
| 2 | Model selection: config or task type → model (e.g. local for dev, premium for production); document tier (TierLocal, TierPremium). | Go Engineer | Same |
| 3 | On request: check Memcached for llm:resp:{model}:{hash(prompt)}; on hit return cached response and optional cached usage. | Go Engineer | Same |
| 4 | On miss: call LLM API, record tokens_in, tokens_out, latency; cache response with TTL (e.g. 24h); return response and usage. | Go Engineer | Same |
| 5 | Usage struct returned in-memory; do not write to DB in request path (async persistence in Phase 4 or per phase-history-usage-audit-design). | Go Engineer | Same |
| 6 | Register metrics: astra_llm_token_usage_total, astra_llm_cost_dollars (if cost available). | Go Engineer | pkg/metrics |
| 7 | Unit tests: mock LLM; cache hit returns without calling LLM. | Go Engineer | Tests |

**Deliverables:** internal/llm package; tests.

**Acceptance criteria:** Router returns completion; cache hit avoids LLM call; usage available in response; no sync DB write on hot path.

---

### WP3.4 — Memory service API (cmd/memory-service)

**Description:** Expose memory write and search via gRPC (and optionally REST). Read path: serve from Redis/Memcached when possible so p99 ≤10ms. Write path: persist to Postgres, then async or sync cache update (e.g. cache recent memory IDs or search results). PRD §13: all API read endpoints serve from cache.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define memory service proto: WriteMemory, SearchMemories, GetMemory. | Architect / Go Engineer | proto/memory or existing |
| 2 | Implement memory-service: gRPC handlers call internal/memory.Store; Search and GetByID try Redis/Memcached first (e.g. cache key memory:agent:{id}:search:{query_hash} or memory:{id}). | Go Engineer | cmd/memory-service/main.go |
| 3 | Cache-aside: on miss read from Postgres, populate cache, return. Set TTL (e.g. 5m for hot data). | Go Engineer | Same |
| 4 | Write: persist to Postgres; invalidate or update cache for affected agent. | Go Engineer | Same |
| 5 | Integration test: write then search; second search served from cache (measure latency). | QA / Go Engineer | Tests |

**Deliverables:** cmd/memory-service; proto if new; tests.

**Acceptance criteria:** Memory write and search work via API; repeated reads served from cache; p99 read latency ≤10ms in load test.

---

### WP3.5 — LLM router service (cmd/llm-router)

**Description:** Expose LLM router as a service (gRPC): Complete(request) returns response and usage. Used by planner-service, execution-worker, memory-service (for embedding model), etc. Memcached for response cache; no Postgres on read path.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | Define llm-router proto: Complete(CompletionRequest) returns CompletionResponse (content, usage). | Architect / Go Engineer | proto/llm or existing |
| 2 | cmd/llm-router: gRPC server calling internal/llm.Router.Complete. | Go Engineer | cmd/llm-router/main.go |
| 3 | Config: LLM endpoint(s), model list, cache TTL, Memcached addr. | Go Engineer | Same |
| 4 | Integration test: two identical requests; second is cache hit. | QA / Go Engineer | Tests |

**Deliverables:** cmd/llm-router; tests.

**Acceptance criteria:** LLM router service returns completions; cache hit avoids backend call; p99 latency within SLA when cached.

---

### WP3.6 — Prompt manager (cmd/prompt-manager)

**Description:** Implement prompt-manager: store and retrieve prompt templates (name, version, body, variables). Used by planner and agents to get prompt text. Optional: A/B experiments (Phase 4). Phase 3: CRUD for prompts, get by name/version. Cache hot prompts in Redis/Memcached for 10ms read path.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | DB: prompts table if not in migrations (id, name, version, body, variables schema, created_at). Migration idempotent. | DB Architect | migrations/0010_prompts.sql or similar |
| 2 | internal/prompt or internal/llm: GetPrompt(ctx, name, version), SavePrompt(ctx, prompt). | Go Engineer | internal package |
| 3 | cmd/prompt-manager: gRPC GetPrompt, SavePrompt; cache get in Redis/Memcached. | Go Engineer | cmd/prompt-manager/main.go |
| 4 | Unit tests. | Go Engineer | Tests |

**Deliverables:** prompts table; prompt-manager service; tests.

**Acceptance criteria:** Prompts can be stored and retrieved; read path cacheable; p99 ≤10ms for GetPrompt.

---

### WP3.7 — Redis/Memcached cache-aside for task and agent state

**Description:** To meet 10ms read SLA globally, task-service and agent-service read paths must use cache. Task: GetTask, GetGraph — cache key task:{id}, graph:{id} with TTL. Agent: GetAgent — cache key agent:{id}. Write path: update Postgres then set/invalidate cache. PRD §13: actor:state:<id>, task lookups from cache.

| # | Task | Owner | Deliverable |
|---|------|--------|-------------|
| 1 | task-service: on GetTask/GetGraph check Redis first; on miss query Postgres, set cache, return. | Go Engineer | cmd/task-service or internal/tasks |
| 2 | agent-service: on GetAgent (or equivalent) check Redis first; on miss Postgres, set cache. | Go Engineer | cmd/agent-service |
| 3 | On task transition and agent update: write Postgres then update or invalidate cache. | Go Engineer | Same |
| 4 | Load test: p99 read latency for GetTask and GetAgent ≤10ms. | QA | Benchmarks |

**Deliverables:** Cache-aside in task-service and agent-service; load test results.

**Acceptance criteria:** Hot-path reads for task and agent state served from Redis; p99 ≤10ms.

---

## 4. Delegation hints

| Work package | Primary owner | Hand-off |
|--------------|---------------|----------|
| WP3.1 memory store | Go Engineer | Hand off to memory-service and embedding pipeline |
| WP3.2 embedding | Go Engineer | Depends on Memcached; hand off to memory write path |
| WP3.3 llm router | Go Engineer | Hand off to cmd/llm-router |
| WP3.4 memory-service | Go Engineer | QA for latency test |
| WP3.5 llm-router cmd | Go Engineer | QA for cache and latency |
| WP3.6 prompt-manager | Go Engineer + DB Architect | Migration + service |
| WP3.7 cache-aside | Go Engineer | QA for 10ms load test |

---

## 5. Ordering within Phase 3

1. **First:** WP3.1 (memory store) and WP3.2 (embedding pipeline) can run in parallel; WP3.2 may depend on external embedding API config.
2. **Then:** WP3.3 (LLM router) — can start once Memcached and config are ready.
3. **Then:** WP3.4 (memory-service) after WP3.1, WP3.2; WP3.5 (llm-router) after WP3.3.
4. **Parallel:** WP3.6 (prompt-manager) and WP3.7 (cache-aside for task/agent) — WP3.6 needs migration first (DB Architect).
5. **Last:** Load tests and 10ms verification (QA).

---

## 6. Risks / open decisions

- **Embedding model:** Which embedding API or model to use (e.g. OpenAI, local sentence-transformers). Config-driven; document default.
- **Memcached vs Redis for LLM cache:** PRD §13 uses Memcached for llm:resp and embed:. Use Memcached for large values if preferred; Redis for smaller keys. Confirm infra: both available in docker-compose.
- **Cost in Phase 3:** astra_llm_cost_dollars may require a pricing table (model → $/token). Can be stub (0 or null) until Phase 4/5.
- **Read replica:** If Search is heavy, Postgres read replica for memory search; optional for Phase 3.

---

## Sign-off (Phase 3 complete)

- [ ] Agent can write memory and search by semantic similarity; memory-service uses pgvector and cache.
- [ ] LLM router selects model and caches responses; repeated prompts served from cache.
- [ ] Prompt-manager stores and retrieves templates; read path cached.
- [ ] Task and agent state reads served from Redis; p99 ≤10ms in load test.
- [ ] All tests pass; CI green.
