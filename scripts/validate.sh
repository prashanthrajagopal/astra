#!/usr/bin/env bash
# Astra API validation script.
# Run from repo root: ./scripts/validate.sh
# Validates every documented API (docs/api/openapi.yaml) across all services.
# Requires: running services (deploy.sh), Postgres, Redis.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if [[ -f .env ]]; then
  set -a; source .env; set +a
fi

# Service base URLs (ports from openapi.yaml)
API="${API:-http://localhost:${HTTP_PORT:-8080}}"
IDENTITY_URL="${IDENTITY_URL:-http://localhost:${IDENTITY_PORT:-8085}}"
ACCESS_CONTROL_URL="${ACCESS_CONTROL_URL:-http://localhost:${ACCESS_CONTROL_PORT:-8086}}"
TOOL_RUNTIME_URL="${TOOL_RUNTIME_URL:-http://localhost:${TOOL_RUNTIME_PORT:-8083}}"
WORKER_MANAGER_URL="${WORKER_MANAGER_URL:-http://localhost:${WORKER_MANAGER_PORT:-8082}}"
GOAL_SERVICE_URL="${GOAL_SERVICE_URL:-http://localhost:${GOAL_SERVICE_PORT:-8088}}"
PROMPT_MANAGER_URL="${PROMPT_MANAGER_URL:-http://localhost:${PROMPT_MANAGER_PORT:-8084}}"
PLANNER_URL="${PLANNER_URL:-http://localhost:${PLANNER_PORT:-8087}}"
EVALUATION_URL="${EVALUATION_URL:-http://localhost:${EVALUATION_PORT:-8089}}"
COST_TRACKER_URL="${COST_TRACKER_URL:-http://localhost:${COST_TRACKER_PORT:-8090}}"

PASS=0
FAIL=0
SKIP=0
TOTAL=0

red()    { printf "\033[31m%s\033[0m" "$*"; }
green()  { printf "\033[32m%s\033[0m" "$*"; }
yellow() { printf "\033[33m%s\033[0m" "$*"; }
bold()   { printf "\033[1m%s\033[0m" "$*"; }

assert_eq() {
  TOTAL=$((TOTAL+1))
  local label="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    PASS=$((PASS+1))
    echo "  $(green "✓") $label"
  else
    FAIL=$((FAIL+1))
    echo "  $(red "✗") $label  (expected: $expected, got: $actual)"
  fi
}

assert_contains() {
  TOTAL=$((TOTAL+1))
  local label="$1" substring="$2" actual="$3"
  if [[ "$actual" == *"$substring"* ]]; then
    PASS=$((PASS+1))
    echo "  $(green "✓") $label"
  else
    FAIL=$((FAIL+1))
    echo "  $(red "✗") $label  (expected to contain: '$substring', got: '$actual')"
  fi
}

assert_not_empty() {
  TOTAL=$((TOTAL+1))
  local label="$1" actual="$2"
  if [[ -n "$actual" ]]; then
    PASS=$((PASS+1))
    echo "  $(green "✓") $label"
  else
    FAIL=$((FAIL+1))
    echo "  $(red "✗") $label  (expected non-empty)"
  fi
}

# ═══════════════════════════════════════════════
# Infrastructure & Build
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ Infrastructure & Build ═══')"

echo "Build verification:"
if go build ./... 2>/dev/null; then
  assert_eq "go build ./... passes" "0" "0"
else
  assert_eq "go build ./... passes" "0" "1"
fi

if go vet ./... 2>/dev/null; then
  assert_eq "go vet ./... passes" "0" "0"
else
  assert_eq "go vet ./... passes" "0" "1"
fi

echo "Proto generated code:"
assert_eq "kernel.pb.go exists" "true" "$(test -f proto/kernel/kernel.pb.go && echo true || echo false)"
assert_eq "task.pb.go exists" "true" "$(test -f proto/tasks/task.pb.go && echo true || echo false)"
assert_eq "kernel_grpc.pb.go exists" "true" "$(test -f proto/kernel/kernel_grpc.pb.go && echo true || echo false)"
assert_eq "task_grpc.pb.go exists" "true" "$(test -f proto/tasks/task_grpc.pb.go && echo true || echo false)"

echo "CI:"
assert_eq "ci.yml exists" "true" "$(test -f .github/workflows/ci.yml && echo true || echo false)"
assert_eq ".golangci.yml exists" "true" "$(test -f .golangci.yml && echo true || echo false)"

echo "Migrations:"
MIGRATION_COUNT=$(ls migrations/*.sql 2>/dev/null | wc -l | tr -d ' ')
assert_eq "at least 10 migration files" "true" "$([ "$MIGRATION_COUNT" -ge 10 ] && echo true || echo false)"

echo "Unit tests (short mode):"
TEST_OUTPUT=$(go test ./... -count=1 -short 2>&1)
TEST_EXIT=$?
assert_eq "go test ./... -short passes" "0" "$TEST_EXIT"

PKGS_TESTED=$(echo "$TEST_OUTPUT" | grep "^ok" | wc -l | tr -d ' ')
assert_eq "at least 7 packages tested" "true" "$([ "$PKGS_TESTED" -ge 7 ] && echo true || echo false)"

echo "Infra:"
PG_OK=$(nc -z "${POSTGRES_HOST:-localhost}" "${POSTGRES_PORT:-5432}" 2>/dev/null && echo true || echo false)
assert_eq "Postgres reachable" "true" "$PG_OK"

REDIS_OK=$(nc -z "${REDIS_HOST:-localhost}" "${REDIS_PORT:-6379}" 2>/dev/null && echo true || echo false)
assert_eq "Redis reachable" "true" "$REDIS_OK"

# ═══════════════════════════════════════════════
# Identity Service — JWT (needed for gateway)
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ Identity Service ═══')"

IDENTITY_HEALTH=$(curl -sf "$IDENTITY_URL/health" 2>/dev/null || echo "FAIL")
assert_eq "GET /health" "ok" "$IDENTITY_HEALTH"

TOKEN_RESP=$(curl -sf -X POST "$IDENTITY_URL/tokens" -H "Content-Type: application/json" -d '{"subject":"validator","scopes":["admin"],"ttl_seconds":600}' 2>/dev/null || echo '{}')
JWT_TOKEN=$(echo "$TOKEN_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || echo "")
assert_not_empty "POST /tokens returns token" "$JWT_TOKEN"

VALID_VAL="false"
if [[ -n "$JWT_TOKEN" ]]; then
  VALIDATE_RESP=$(curl -sf -X POST "$IDENTITY_URL/validate" -H "Content-Type: application/json" -d "{\"token\":\"$JWT_TOKEN\"}" 2>/dev/null || echo '{}')
  VALID_VAL=$(echo "$VALIDATE_RESP" | python3 -c "import sys,json; print(str(json.load(sys.stdin).get('valid',False)).lower())" 2>/dev/null || echo "false")
fi
assert_eq "POST /validate returns valid true" "true" "$VALID_VAL"

AUTH_HEADER=()
[[ -n "$JWT_TOKEN" ]] && AUTH_HEADER=(-H "Authorization: Bearer $JWT_TOKEN")

# ═══════════════════════════════════════════════
# API Gateway — Health & unauthenticated
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ API Gateway ═══')"

GATEWAY_HEALTH=$(curl -sf "$API/health" 2>/dev/null || echo "FAIL")
assert_eq "GET /health" "ok" "$GATEWAY_HEALTH"

NOAUTH_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$API/agents" -H "Content-Type: application/json" -d '{"actor_type":"test-agent","config":"{}"}' 2>/dev/null || echo "000")
assert_eq "POST /agents without JWT returns 401" "401" "$NOAUTH_CODE"

# API Gateway — Agents (JWT required; all must pass)
echo "Agents:"
POST_AGENTS_RESP=$(curl -sf -X POST "$API/agents" "${AUTH_HEADER[@]}" -H "Content-Type: application/json" -d '{"actor_type":"validate-agent","name":"Validate Agent","config":"{}"}' 2>/dev/null || echo '{"error":"failed"}')
ACTOR_ID=$(echo "$POST_AGENTS_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('actor_id',''))" 2>/dev/null || echo "")
assert_not_empty "POST /agents returns actor_id" "$ACTOR_ID"

AGENTS_RESP=$(curl -sf "$API/agents" "${AUTH_HEADER[@]}" 2>/dev/null || echo '{}')
AGENTS_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$API/agents" "${AUTH_HEADER[@]}" 2>/dev/null || echo "000")
assert_eq "GET /agents returns 200" "200" "$AGENTS_CODE"
assert_contains "GET /agents response has agents" "agents" "$AGENTS_RESP"

if [[ -n "$ACTOR_ID" ]]; then
  PATCH_RESP=$(curl -sf -X PATCH "$API/agents/$ACTOR_ID" "${AUTH_HEADER[@]}" -H "Content-Type: application/json" -d '{"system_prompt":"Updated"}' 2>/dev/null || echo '{}')
  PATCH_STATUS=$(echo "$PATCH_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
  assert_eq "PATCH /agents/{id} returns status ok" "ok" "$PATCH_STATUS"

  PROFILE_RESP=$(curl -sf "$API/agents/$ACTOR_ID/profile" "${AUTH_HEADER[@]}" 2>/dev/null || echo '{}')
  PROFILE_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$API/agents/$ACTOR_ID/profile" "${AUTH_HEADER[@]}" 2>/dev/null || echo "000")
  assert_eq "GET /agents/{id}/profile returns 200" "200" "$PROFILE_CODE"
  assert_contains "GET /agents/{id}/profile has id" "id" "$PROFILE_RESP"

  # Documents
  DOC_BODY=$(mktemp)
  DOC_CODE=$(curl -s -o "$DOC_BODY" -w "%{http_code}" -X POST "$API/agents/$ACTOR_ID/documents" "${AUTH_HEADER[@]}" -H "Content-Type: application/json" -d '{"doc_type":"rule","name":"test-rule","content":"test"}' 2>/dev/null || echo "000")
  DOC_RESP=$(cat "$DOC_BODY" 2>/dev/null); rm -f "$DOC_BODY"
  DOC_ID=$(echo "$DOC_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")
  assert_eq "POST /agents/{id}/documents returns 201" "201" "$DOC_CODE"
  assert_not_empty "POST /agents/{id}/documents returns id" "$DOC_ID"

  LIST_DOCS=$(curl -sf "$API/agents/$ACTOR_ID/documents" "${AUTH_HEADER[@]}" 2>/dev/null || echo '{}')
  assert_contains "GET /agents/{id}/documents has documents" "documents" "$LIST_DOCS"

  DEL_DOC_CODE="000"
  [[ -n "$DOC_ID" ]] && DEL_DOC_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$API/agents/$ACTOR_ID/documents/$DOC_ID" "${AUTH_HEADER[@]}" 2>/dev/null || echo "000")
  assert_eq "DELETE /agents/{id}/documents/{docId} returns 200" "200" "$DEL_DOC_CODE"

  GOAL_RESP=$(curl -sf -X POST "$API/agents/$ACTOR_ID/goals" "${AUTH_HEADER[@]}" -H "Content-Type: application/json" -d '{"goal_text":"validate api"}' 2>/dev/null || echo '{}')
  GOAL_STATUS=$(echo "$GOAL_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
  assert_eq "POST /agents/{id}/goals returns status ok" "ok" "$GOAL_STATUS"
fi

# Tasks & Graphs — use a non-existent UUID; expect 404 or 500 (endpoint exists)
FAKE_UUID="00000000-0000-0000-0000-000000000001"
TASK_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$API/tasks/$FAKE_UUID" "${AUTH_HEADER[@]}" 2>/dev/null || echo "000")
assert_eq "GET /tasks/{taskId} returns 404 or 500 for unknown id" "true" "$([ "$TASK_CODE" = "404" ] || [ "$TASK_CODE" = "500" ] && echo true || echo false)"

GRAPH_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$API/graphs/$FAKE_UUID" "${AUTH_HEADER[@]}" 2>/dev/null || echo "000")
assert_eq "GET /graphs/{graphId} returns 404 or 500 for unknown id" "true" "$([ "$GRAPH_CODE" = "404" ] || [ "$GRAPH_CODE" = "500" ] && echo true || echo false)"

# API Gateway — Dashboard
echo "Dashboard:"
SNAPSHOT=$(curl -sf "$API/superadmin/api/dashboard/snapshot" 2>/dev/null || echo "{}")
assert_not_empty "GET /superadmin/api/dashboard/snapshot returns data" "$SNAPSHOT"
SNAPSHOT_KEYS=$(echo "$SNAPSHOT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(','.join(sorted(d.keys())))" 2>/dev/null || echo "")
assert_contains "snapshot has services key" "services" "$SNAPSHOT_KEYS"
assert_contains "snapshot has workers key" "workers" "$SNAPSHOT_KEYS"
assert_contains "snapshot has approvals key" "approvals" "$SNAPSHOT_KEYS"
assert_contains "snapshot has cost key" "cost" "$SNAPSHOT_KEYS"
assert_contains "snapshot has logs key" "logs" "$SNAPSHOT_KEYS"
assert_contains "snapshot has pids key" "pids" "$SNAPSHOT_KEYS"

DASH_HTML=$(curl -sf "$API/superadmin/dashboard/" 2>/dev/null || curl -sf "$API/superadmin/dashboard/index.html" 2>/dev/null || echo "")
assert_contains "GET /superadmin/dashboard/ serves HTML" "Astra Platform Dashboard" "$DASH_HTML"

# Dashboard approval flow: create pending approval then approve via gateway
APPROVAL_SEED=$(curl -s -X POST "$TOOL_RUNTIME_URL/execute" -H "Content-Type: application/json" -d '{"name":"terraform apply","timeout_seconds":5}' 2>/dev/null || echo '{}')
APPROVAL_ID=$(echo "$APPROVAL_SEED" | python3 -c "import sys,json; print(json.load(sys.stdin).get('approval_request_id',''))" 2>/dev/null || echo "")
assert_not_empty "Tool runtime returns approval_request_id for dangerous tool (dashboard approve)" "$APPROVAL_ID"
APPROVE_RESP=$(curl -sf -X POST "$API/superadmin/api/dashboard/approvals/$APPROVAL_ID/approve" -H "Content-Type: application/json" -d '{"decided_by":"validate"}' 2>/dev/null || echo '{}')
APPROVE_STATUS=$(echo "$APPROVE_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
assert_eq "POST /superadmin/api/dashboard/approvals/{id}/approve" "ok" "$APPROVE_STATUS"

# ═══════════════════════════════════════════════
# Access Control Service
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ Access Control Service ═══')"

ACCESS_HEALTH=$(curl -sf "$ACCESS_CONTROL_URL/health" 2>/dev/null || echo "FAIL")
assert_eq "GET /health" "ok" "$ACCESS_HEALTH"

CHECK_RESP=$(curl -sf -X POST "$ACCESS_CONTROL_URL/check" -H "Content-Type: application/json" -d '{"subject":"validator","action":"execute","resource":"tool-runtime","tool_name":"echo"}' 2>/dev/null || echo '{}')
assert_contains "POST /check returns allowed or reason" "allowed" "$CHECK_RESP"

PENDING_JSON=$(curl -sf "$ACCESS_CONTROL_URL/approvals/pending" 2>/dev/null || echo "[]")
PENDING_IS_ARRAY=$(echo "$PENDING_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); print('true' if isinstance(d, list) else 'false')" 2>/dev/null || echo "false")
assert_eq "GET /approvals/pending returns array" "true" "$PENDING_IS_ARRAY"

# ═══════════════════════════════════════════════
# Tool Runtime
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ Tool Runtime ═══')"

TOOL_HEALTH=$(curl -sf "$TOOL_RUNTIME_URL/health" 2>/dev/null || echo "FAIL")
assert_eq "GET /health" "ok" "$TOOL_HEALTH"

EXEC_RESP=$(curl -sf -X POST "$TOOL_RUNTIME_URL/execute" -H "Content-Type: application/json" -d '{"name":"echo test","timeout_seconds":5}' 2>/dev/null || echo '{"error":"failed"}')
EXEC_CODE=$(echo "$EXEC_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('exit_code',-1))" 2>/dev/null || echo "-1")
assert_eq "POST /execute (safe tool) returns exit_code 0" "0" "$EXEC_CODE"

DANGEROUS_RESP=$(curl -s -X POST "$TOOL_RUNTIME_URL/execute" -H "Content-Type: application/json" -d '{"name":"terraform plan","timeout_seconds":5}' 2>/dev/null || echo '{}')
DANGEROUS_STATUS=$(echo "$DANGEROUS_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
assert_eq "POST /execute (dangerous) returns pending_approval" "pending_approval" "$DANGEROUS_STATUS"

# ═══════════════════════════════════════════════
# Worker Manager
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ Worker Manager ═══')"

WORKER_MGR_HEALTH=$(curl -sf "$WORKER_MANAGER_URL/health" 2>/dev/null || echo "FAIL")
assert_eq "GET /health" "ok" "$WORKER_MGR_HEALTH"

WORKERS_RESP=$(curl -sf "$WORKER_MANAGER_URL/workers" 2>/dev/null || echo "[]")
assert_not_empty "GET /workers returns data" "$WORKERS_RESP"
# Response may be {"workers": [...]} or raw array [...]
assert_eq "GET /workers returns JSON array or object" "true" "$(echo "$WORKERS_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print('true' if isinstance(d, (list, dict)) else 'false')" 2>/dev/null || echo "false")"

# ═══════════════════════════════════════════════
# Goal Service
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ Goal Service ═══')"

GOAL_HEALTH=$(curl -sf "$GOAL_SERVICE_URL/health" 2>/dev/null || echo "FAIL")
assert_eq "GET /health" "ok" "$GOAL_HEALTH"

assert_not_empty "ACTOR_ID required for goal service (from POST /agents)" "$ACTOR_ID"
CREATE_GOAL_RESP=$(curl -sf -X POST "$GOAL_SERVICE_URL/goals" -H "Content-Type: application/json" -d "{\"agent_id\":\"$ACTOR_ID\",\"goal_text\":\"validate goal api\",\"priority\":50}" 2>/dev/null || echo '{}')
GOAL_ID=$(echo "$CREATE_GOAL_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('goal_id',''))" 2>/dev/null || echo "")
assert_not_empty "POST /goals returns goal_id" "$GOAL_ID"

LIST_GOALS=$(curl -sf "$GOAL_SERVICE_URL/goals?agent_id=$ACTOR_ID" 2>/dev/null || echo '{}')
assert_contains "GET /goals?agent_id= has goals" "goals" "$LIST_GOALS"

GOAL_DETAIL=$(curl -sf "$GOAL_SERVICE_URL/goals/$GOAL_ID" 2>/dev/null || echo '{}')
assert_contains "GET /goals/{id} returns goal" "id" "$GOAL_DETAIL"

FINALIZE_RESP=$(curl -sf -X POST "$GOAL_SERVICE_URL/goals/$GOAL_ID/finalize" 2>/dev/null || echo '{}')
assert_contains "POST /goals/{id}/finalize returns status" "status" "$FINALIZE_RESP"

STATS_RESP=$(curl -sf "$GOAL_SERVICE_URL/stats" 2>/dev/null || echo '{}')
assert_contains "GET /stats has goals/tasks" "goals" "$STATS_RESP"

# ═══════════════════════════════════════════════
# Prompt Manager
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ Prompt Manager ═══')"

PROMPT_HEALTH=$(curl -sf "$PROMPT_MANAGER_URL/health" 2>/dev/null || echo "FAIL")
assert_eq "GET /health" "ok" "$PROMPT_HEALTH"

PROMPT_NAME="validate-test-prompt"
PROMPT_VER=1
POST_PROMPT_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$PROMPT_MANAGER_URL/prompts" -H "Content-Type: application/json" -d "{\"name\":\"$PROMPT_NAME\",\"version\":\"$PROMPT_VER\",\"body\":\"Test {{var}}\",\"variables_schema\":\"{}\"}" 2>/dev/null || echo "000")
assert_eq "POST /prompts returns 2xx" "true" "$([ "$POST_PROMPT_CODE" = "200" ] || [ "$POST_PROMPT_CODE" = "201" ] && echo true || echo false)"

GET_PROMPT=$(curl -sf "$PROMPT_MANAGER_URL/prompts/$PROMPT_NAME/$PROMPT_VER" 2>/dev/null || echo '{}')
assert_contains "GET /prompts/{name}/{version} returns body" "body" "$GET_PROMPT"

# ═══════════════════════════════════════════════
# Planner Service
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ Planner Service ═══')"

PLANNER_HEALTH=$(curl -sf "$PLANNER_URL/health" 2>/dev/null || echo "FAIL")
assert_eq "GET /health" "ok" "$PLANNER_HEALTH"

PLAN_RESP=$(curl -sf -X POST "$PLANNER_URL/plan" -H "Content-Type: application/json" -d '{"goal_id":"00000000-0000-0000-0000-000000000001","agent_id":"00000000-0000-0000-0000-000000000002","goal_text":"Build a hello world page"}' 2>/dev/null || echo '{}')
assert_contains "POST /plan returns tasks or graph" "tasks" "$PLAN_RESP"

# ═══════════════════════════════════════════════
# Evaluation Service
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ Evaluation Service ═══')"

EVAL_HEALTH=$(curl -sf "$EVALUATION_URL/health" 2>/dev/null || echo "FAIL")
assert_eq "GET /health" "ok" "$EVAL_HEALTH"

EVAL_RESP=$(curl -sf -X POST "$EVALUATION_URL/evaluate" -H "Content-Type: application/json" -d '{"task_id":"validate-eval","result":"hello world","criteria":"hello"}' 2>/dev/null || echo '{}')
EVAL_PASSED=$(echo "$EVAL_RESP" | python3 -c "import sys,json; print(str(json.load(sys.stdin).get('passed',False)).lower())" 2>/dev/null || echo "false")
assert_eq "POST /evaluate returns passed true for matching criteria" "true" "$EVAL_PASSED"

# ═══════════════════════════════════════════════
# Cost Tracker
# ═══════════════════════════════════════════════
echo ""
echo "$(bold '═══ Cost Tracker ═══')"

COST_HEALTH=$(curl -sf "$COST_TRACKER_URL/health" 2>/dev/null || echo "FAIL")
assert_eq "GET /health" "ok" "$COST_HEALTH"

COST_RESP=$(curl -sf "$COST_TRACKER_URL/cost/daily?days=7" 2>/dev/null || echo '{}')
assert_contains "GET /cost/daily returns rows or empty" "rows" "$COST_RESP"

# ── Chat (sessions and WebSocket) ──
# WebSocket streaming tests require a WebSocket client; manual or integration test.
echo ""
echo "$(bold '═══ Chat (sessions and WebSocket) ═══')"

if [[ "${CHAT_ENABLED:-false}" != "true" ]] && [[ "${CHAT_ENABLED:-0}" != "1" ]]; then
  echo "  $(yellow "⊘") Chat tests skipped (CHAT_ENABLED not set)"
  SKIP=$((SKIP + 5))
  TOTAL=$((TOTAL + 5))
else
  echo "Chat sessions:"
  CHAT_AGENT_ID=$(echo "$AGENTS_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); a=d.get('agents',[]); a=a[0] if isinstance(a,list) and a else {}; print(a.get('id',''))" 2>/dev/null || echo "")
  [[ -z "$CHAT_AGENT_ID" ]] && CHAT_AGENT_ID="$ACTOR_ID"
  assert_not_empty "agent_id available for chat session" "$CHAT_AGENT_ID"

  SESSION_RESP=$(curl -s -w "\n%{http_code}" -X POST "$API/superadmin/api/dashboard/chat/sessions" -H "Content-Type: application/json" -d "{\"agent_id\":\"$CHAT_AGENT_ID\",\"title\":\"\"}" 2>/dev/null || echo -e "\n000")
  SESSION_BODY=$(echo "$SESSION_RESP" | sed '$d')
  SESSION_CODE=$(echo "$SESSION_RESP" | tail -1)
  assert_eq "POST /superadmin/api/dashboard/chat/sessions returns 201" "201" "$SESSION_CODE"

  SESSION_ID=$(echo "$SESSION_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || echo "")
  assert_not_empty "session create returns session_id (id)" "$SESSION_ID"

  LIST_SESSIONS_RESP=$(curl -s -w "\n%{http_code}" "$API/superadmin/api/dashboard/chat/sessions" 2>/dev/null || echo -e "\n000")
  LIST_SESSIONS_CODE=$(echo "$LIST_SESSIONS_RESP" | tail -1)
  assert_eq "GET /superadmin/api/dashboard/chat/sessions returns 200" "200" "$LIST_SESSIONS_CODE"

  if [[ -n "$SESSION_ID" ]]; then
    GET_SESSION_RESP=$(curl -s -w "\n%{http_code}" "$API/superadmin/api/dashboard/chat/sessions/$SESSION_ID" 2>/dev/null || echo -e "\n000")
    GET_SESSION_CODE=$(echo "$GET_SESSION_RESP" | tail -1)
    assert_eq "GET /superadmin/api/dashboard/chat/sessions/{id} returns 200" "200" "$GET_SESSION_CODE"

    SEND_MSG_RESP=$(curl -s -w "\n%{http_code}" -X POST "$API/superadmin/api/dashboard/chat/sessions/$SESSION_ID/messages" -H "Content-Type: application/json" -d '{"content":"hello"}' 2>/dev/null || echo -e "\n000")
    SEND_MSG_CODE=$(echo "$SEND_MSG_RESP" | tail -1)
    assert_eq "POST /superadmin/api/dashboard/chat/sessions/{id}/messages returns 200 or 201" "true" "$([ "$SEND_MSG_CODE" = "200" ] || [ "$SEND_MSG_CODE" = "201" ] && echo true || echo false)"

    MSGS_RESP=$(curl -s -w "\n%{http_code}" "$API/superadmin/api/dashboard/chat/sessions/$SESSION_ID/messages" 2>/dev/null || echo -e "\n000")
    MSGS_CODE=$(echo "$MSGS_RESP" | tail -1)
    assert_eq "GET /superadmin/api/dashboard/chat/sessions/{id}/messages returns 200" "200" "$MSGS_CODE"
  fi
fi

# ═══════════════════════════════════════════════
# Summary
# ═══════════════════════════════════════════════
# ═══════════════════════════════════════════════════════════════
# Phase 11: Multi-Tenancy
# ═══════════════════════════════════════════════════════════════

echo ""; echo "=== Phase 11: Multi-Tenancy ==="; echo ""

assert_eq "migration 0018 exists" "true" "$(test -f migrations/0018_multi_tenant.sql && echo true || echo false)"
assert_eq "internal/identity/store.go exists" "true" "$(test -f internal/identity/store.go && echo true || echo false)"
assert_eq "internal/identity/jwt.go exists" "true" "$(test -f internal/identity/jwt.go && echo true || echo false)"
assert_eq "internal/rbac/rbac.go exists" "true" "$(test -f internal/rbac/rbac.go && echo true || echo false)"
assert_eq "internal/rbac/visibility.go exists" "true" "$(test -f internal/rbac/visibility.go && echo true || echo false)"
assert_eq "internal/rbac/middleware.go exists" "true" "$(test -f internal/rbac/middleware.go && echo true || echo false)"
assert_eq "internal/orgs/store.go exists" "true" "$(test -f internal/orgs/store.go && echo true || echo false)"
assert_eq "identity builds" "true" "$(go build ./internal/identity/... 2>/dev/null && echo true || echo false)"
assert_eq "rbac builds" "true" "$(go build ./internal/rbac/... 2>/dev/null && echo true || echo false)"
assert_eq "orgs builds" "true" "$(go build ./internal/orgs/... 2>/dev/null && echo true || echo false)"
assert_eq "identity service builds" "true" "$(go build ./cmd/identity/... 2>/dev/null && echo true || echo false)"
assert_eq "api-gateway builds" "true" "$(go build ./cmd/api-gateway/... 2>/dev/null && echo true || echo false)"
assert_eq "PRD has multi-tenancy section" "true" "$(grep -q 'Multi-Tenancy Architecture' docs/PRD.md && echo true || echo false)"
assert_eq ".env.example has ASTRA_SUPER_ADMIN_EMAIL" "true" "$(grep -q 'ASTRA_SUPER_ADMIN_EMAIL' .env.example && echo true || echo false)"

echo ""
echo "$(bold '═══════════════════════════════════════')"
echo "$(bold '         VALIDATION SUMMARY')"
echo "$(bold '═══════════════════════════════════════')"
echo ""
echo "  $(green "✓ Passed:  $PASS")"
echo "  $(red "✗ Failed:  $FAIL")"
echo "  $(yellow "⊘ Skipped: $SKIP")"
echo "  Total:   $TOTAL"
echo ""

if [[ $FAIL -gt 0 ]]; then
  echo "$(red 'RESULT: FAIL')"
  exit 1
else
  echo "$(green 'RESULT: PASS')"
  exit 0
fi
