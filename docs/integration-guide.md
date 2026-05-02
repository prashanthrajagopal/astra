# Connecting External Applications to Astra

Astra is a generic autonomous agent platform. Any external application can dispatch goals to Astra agents, inject context, and receive structured results without knowing anything about the internal task graph or LLM execution.

---

## How it works

```
External Application
  POST /agents/{id}/goals  ->  Astra (goal-service)
                                   -> Planner -> Task DAG
                                        -> execution-worker
                                             -> http_fetch tool -> External APIs
                                             -> task result (JSON execution plan)
                                   -> Goal completed -> result_payload stored

External Application polls:
  GET /goals/{id}/result   <-  { status, result_payload, completed_at }
```

**Key principle:** Astra agents never act on external systems directly. They query external data sources to gather context, then produce a structured **execution plan** as their result. Your application reads the plan and executes the actions.

---

## 1. Register an agent

Create an agent with a system prompt that instructs it to produce an execution plan:

```bash
curl -X POST http://localhost:8080/agents \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Integration Agent",
    "system_prompt": "You are an automation agent. When given a goal, query the provided data sources, decide what actions to take, and return ONLY a JSON execution plan:\n\n{\"version\":\"1\",\"summary\":\"what you decided and why\",\"actions\":[{\"type\":\"<action_type>\",\"payload\":{...}}]}\n\nDo not return any other text.",
    "allowed_tools": ["http_fetch", "shell_exec"]
  }'
```

Save the returned `agent_id`.

---

## 2. Dispatch a goal

```bash
curl -X POST http://localhost:8080/agents/{agent_id}/goals \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{
    "goal_text": "Notify stakeholders of the schedule change for Event X",
    "workspace": "my-application",
    "priority": 100,
    "documents": [
      {
        "doc_type": "context_doc",
        "name": "data-source",
        "content": "Stakeholder API: https://api.example.com/v1\nEndpoints: GET /stakeholders?event=X"
      },
      {
        "doc_type": "context_doc",
        "name": "trigger-context",
        "content": "Trigger: schedule.update | Event: X | Old time: 14:00 | New time: 16:30"
      }
    ]
  }'
```

**Response:**
```json
{ "goal_id": "550e8400-e29b-41d4-a716-446655440000", "status": "pending" }
```

### Goal request fields

| Field | Required | Description |
|-------|----------|-------------|
| `goal_text` | Yes | Natural language description of the task |
| `workspace` | No | Tag to group goals by application (e.g. `"my-app"`) |
| `priority` | No | Integer priority; default 100 |
| `documents` | No | Context documents injected into the agent's context window |
| `auto_approve` | No | Skip human approval gate for this goal |
| `cascade_id` | No | Link to a parent cascade for grouping related goals |
| `system_prompt` | No | Override the agent's stored system prompt for this goal |

### Document types

| `doc_type` | Purpose |
|------------|---------|
| `context_doc` | General context: API specs, trigger data, reference info |
| `rule` | Constraints the agent must follow |
| `skill` | Reusable instructions the agent can invoke |
| `reference` | Links to external resources |

---

## 3. Poll for results

```bash
curl http://localhost:8080/goals/{goal_id}/result \
  -H "Authorization: Bearer $TOKEN"
```

**Response while running:**
```json
{ "goal_id": "...", "status": "in_progress" }
```

**Response when complete:**
```json
{
  "goal_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "workspace_tag": "my-application",
  "result_payload": {
    "version": "1",
    "summary": "Identified 3 stakeholders for Event X; scheduled notifications.",
    "actions": [
      {
        "type": "send_email",
        "payload": {
          "to": "ops@example.com",
          "subject": "Schedule change: Event X moved to 16:30",
          "body": "..."
        }
      },
      {
        "type": "webhook",
        "payload": {
          "url": "https://partner.example.com/events",
          "method": "POST",
          "body": { "event": "schedule.update", "new_time": "16:30" }
        }
      }
    ]
  },
  "completed_at": "2026-05-02T17:45:00Z"
}
```

Poll every few seconds until `status` is `"completed"` or `"failed"`.

---

## 4. List goals for an agent

```bash
curl "http://localhost:8080/agents/{agent_id}/goals?status=completed" \
  -H "Authorization: Bearer $TOKEN"
```

Optional `?status=` filter: `pending`, `in_progress`, `completed`, `failed`.

---

## 5. Execution plan format

Agents must return their result as a JSON execution plan. Astra validates the envelope; your application defines the action types.

```json
{
  "version": "1",
  "summary": "Human-readable explanation of the decision",
  "actions": [
    {
      "type": "your_action_type",
      "payload": { }
    }
  ],
  "meta": { }
}
```

### Required fields

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Schema version -- use `"1"` |
| `actions` | array | List of actions to dispatch; may be empty |
| `actions[].type` | string | Action type identifier (your application defines these) |

### Optional fields

| Field | Type | Description |
|-------|------|-------------|
| `summary` | string | Human-readable explanation |
| `actions[].payload` | object | Arbitrary action data |
| `meta` | object | Application-defined metadata |

Astra stores the plan in `goals.result_payload` and returns it verbatim via the polling endpoint. It does **not** dispatch the actions -- that is your application's responsibility.

---

## 6. Giving agents access to external APIs

Use the `http_fetch` tool to let agents query external APIs from an allow-list.

### Configure the allow-list

```bash
# In .env or environment
HTTP_FETCH_ALLOWLIST=api.example.com,catalog.internal:8100
HTTP_FETCH_BEARER_TOKEN=your-api-token   # injected into Authorization header
HTTP_FETCH_TIMEOUT_SECONDS=30
```

If `HTTP_FETCH_ALLOWLIST` is empty, all HTTPS/HTTP hosts are allowed (not recommended for production). Cloud metadata endpoints (`169.254.x.x`, `metadata.google.internal`) are always blocked.

### Agent tool call

The agent calls `http_fetch` with:

```json
{
  "url": "https://api.example.com/v1/stakeholders?event=X",
  "method": "GET",
  "headers": { "Accept": "application/json" }
}
```

The tool returns:

```json
{
  "status_code": 200,
  "body": "[{\"name\":\"Alice\",...}]",
  "headers": { "Content-Type": "application/json" }
}
```

---

## 7. Mock mode for integration testing

Set `ASTRA_MOCK_EXECUTION=true` on the execution-worker to skip LLM execution and return a static mock plan immediately. Useful for testing the full dispatch -> poll pipeline without a real LLM.

```bash
ASTRA_MOCK_EXECUTION=true go run ./cmd/execution-worker
```

The mock `result_payload` will be:

```json
{
  "version": "1",
  "summary": "mock execution -- ASTRA_MOCK_EXECUTION=true",
  "actions": [
    { "type": "mock_log", "payload": { "note": "mock mode active", "goal_id": "..." } }
  ]
}
```

---

## 8. Authentication

All endpoints require a Bearer token from the Astra identity service.

```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"..."}'
```

Pass the token as `Authorization: Bearer <token>` on every request.

---

## 9. End-to-end example

```bash
# 1. Get auth token
TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"admin"}' | jq -r .token)

# 2. Get an agent ID
AGENT_ID=$(curl -s http://localhost:8080/agents \
  -H "Authorization: Bearer $TOKEN" | jq -r '.[0].id')

# 3. Dispatch a goal
GOAL_ID=$(curl -s -X POST http://localhost:8080/agents/$AGENT_ID/goals \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "goal_text": "Summarise the top 3 risks and produce a notification plan",
    "workspace": "my-app",
    "documents": [
      {"doc_type":"context_doc","name":"risks","content":"Risk 1: ...\nRisk 2: ...\nRisk 3: ..."}
    ]
  }' | jq -r .goal_id)

echo "Goal: $GOAL_ID"

# 4. Poll until complete
while true; do
  RESULT=$(curl -s http://localhost:8080/goals/$GOAL_ID/result \
    -H "Authorization: Bearer $TOKEN")
  STATUS=$(echo $RESULT | jq -r .status)
  echo "Status: $STATUS"
  if [ "$STATUS" = "completed" ] || [ "$STATUS" = "failed" ]; then
    echo $RESULT | jq .result_payload
    break
  fi
  sleep 3
done
```

---

## 10. API reference

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/agents` | Register a new agent |
| `POST` | `/agents/{id}/goals` | Dispatch a goal to an agent |
| `GET` | `/agents/{id}/goals` | List goals for an agent (`?status=` filter) |
| `GET` | `/goals/{id}/result` | Poll for goal status and execution plan result |
| `GET` | `/goals/{id}` | Full goal details |
| `GET` | `/graphs/{id}` | Task DAG for a goal |
| `GET` | `/tasks/{id}` | Individual task result |

All endpoints are served by the api-gateway at `http://localhost:8080` by default.
See [docs/api/openapi.yaml](api/openapi.yaml) for the full OpenAPI spec.
