#!/usr/bin/env bash
# Seed Astra with a default set of agents that can handle most tasks.
# Prerequisites: Astra services running (api-gateway, identity). Run from repo root.
# Usage: ./scripts/seed-agents.sh
# Idempotent: skips creating an agent if one with the same actor_type already exists (GET /agents).
# Requires: jq (for building JSON). Install with: brew install jq
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if [[ -f .env ]]; then
  set -a
  source .env
  set +a
fi
export POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
export POSTGRES_PORT="${POSTGRES_PORT:-5432}"
export POSTGRES_DB="${POSTGRES_DB:-astra}"
export POSTGRES_USER="${POSTGRES_USER:-astra}"
export PGPASSWORD="${PGPASSWORD:-${POSTGRES_PASSWORD:-changeme}}"

if ! command -v jq &>/dev/null; then
  echo "Error: jq is required to run this script. Install with: brew install jq"
  exit 1
fi

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
IDENTITY_URL="${IDENTITY_URL:-http://localhost:8085}"

echo "=== Astra Agent Seed ==="
echo "Gateway: $GATEWAY_URL  Identity: $IDENTITY_URL"
echo ""

# 1. Get JWT from Identity
echo "Getting JWT..."
TOKEN_RESP=$(curl -s -X POST "$IDENTITY_URL/tokens" \
  -H "Content-Type: application/json" \
  -d '{"subject":"developer","scopes":["admin"],"ttl_seconds":3600}' || true)
if ! echo "$TOKEN_RESP" | grep -q '"token"'; then
  echo "Failed to get token from Identity at $IDENTITY_URL. Is the identity service running?"
  echo "Response: $TOKEN_RESP"
  exit 1
fi
TOKEN=$(echo "$TOKEN_RESP" | sed -n 's/.*"token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
if [[ -z "$TOKEN" ]] && command -v jq &>/dev/null; then
  TOKEN=$(echo "$TOKEN_RESP" | jq -r '.token')
fi
if [[ -z "$TOKEN" ]]; then
  echo "Could not parse token from Identity response."
  exit 1
fi
echo "Token obtained."
echo ""

# Fetch existing agents so we skip duplicates (idempotent seed).
# Retry GET /agents so we don't create duplicates when the gateway is still starting (e.g. on deploy).
EXISTING_AGENTS_JSON=""
echo "Checking for existing agents..."
MAX_ATTEMPTS=5
WAIT_SEC=2
for attempt in $(seq 1 "$MAX_ATTEMPTS"); do
  EXISTING_AGENTS_JSON=$(curl -s -w "\n%{http_code}" -H "Authorization: Bearer $TOKEN" "$GATEWAY_URL/agents" 2>/dev/null || true)
  HTTP_CODE=$(echo "$EXISTING_AGENTS_JSON" | tail -1)
  EXISTING_AGENTS_JSON=$(echo "$EXISTING_AGENTS_JSON" | sed '$d')
  if [[ "$HTTP_CODE" == "200" ]] && echo "$EXISTING_AGENTS_JSON" | jq -e 'has("agents")' &>/dev/null; then
    count=$(echo "$EXISTING_AGENTS_JSON" | jq -r '(.agents // []) | length')
    echo "  Found $count existing agent(s) (attempt $attempt)."
    break
  fi
  if [[ $attempt -lt $MAX_ATTEMPTS ]]; then
    echo "  GET /agents not ready (HTTP $HTTP_CODE); retrying in ${WAIT_SEC}s..."
    sleep "$WAIT_SEC"
  else
    echo "  WARNING: GET /agents failed after $MAX_ATTEMPTS attempts (HTTP $HTTP_CODE). Skipping seed to avoid duplicate agents. Run seed again after gateway is ready."
    exit 0
  fi
done
echo ""

# Return existing agent id for actor_type if any; else empty.
get_existing_id() {
  echo "$EXISTING_AGENTS_JSON" | jq -r --arg t "$1" '(.agents // [])[]? | select(.actor_type == $t) | .id' | head -1
}

# Helper: create agent (or skip if actor_type exists) and optionally attach one rule document. Sets AGENT_ID.
create_agent() {
  local actor_type="$1"
  local name="$2"
  local system_prompt="$3"
  local config="${4:-{\"model_preference\":\"code\"}}"
  local rule_content="${5:-}"

  local existing_id
  existing_id=$(get_existing_id "$actor_type")
  if [[ -n "$existing_id" ]]; then
    echo "Skipping $name ($actor_type): already exists ($existing_id)"
    AGENT_ID="$existing_id"
    return 0
  fi

  echo "Creating agent: $name ($actor_type)..."
  local body
  body=$(jq -n \
    --arg actor_type "$actor_type" \
    --arg name "$name" \
    --arg system_prompt "$system_prompt" \
    --arg config "$config" \
    '{actor_type:$actor_type,name:$name,system_prompt:$system_prompt,config:$config}')
  local resp
  resp=$(curl -s -X POST "$GATEWAY_URL/agents" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "$body" || true)
  if ! echo "$resp" | grep -q 'actor_id'; then
    echo "  Failed. Response: $resp"
    return 1
  fi
  AGENT_ID=$(echo "$resp" | jq -r '.actor_id')
  echo "  Created: $AGENT_ID"

  # Append to in-memory list so we don't duplicate in same run if GET /agents isn't updated yet
  EXISTING_AGENTS_JSON=$(echo "$EXISTING_AGENTS_JSON" | jq --arg id "$AGENT_ID" --arg t "$actor_type" --arg n "$name" '.agents = ((.agents // []) + [{id:$id,actor_type:$t,name:$n}])')

  if [[ -n "$rule_content" ]]; then
    local doc_body
    doc_body=$(jq -n \
      --arg doc_type "rule" \
      --arg name "${actor_type}-rules" \
      --arg content "$rule_content" \
      --argjson priority 50 \
      '{doc_type:$doc_type,name:$name,content:$content,priority:$priority}')
    curl -s -X POST "$GATEWAY_URL/agents/$AGENT_ID/documents" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      -d "$doc_body" >/dev/null || true
  fi
  return 0
}

# --- Seed agents ---
SEEDED=""

# 1. Python Expert
if create_agent "python-expert" "astra-global-Python Expert" \
  "You are a senior Python expert. You write clean, idiomatic Python (3.10+), follow PEP 8, use type hints, and prefer the standard library. You produce production-ready code with tests when appropriate. You do not write in other languages unless explicitly asked." \
  '{"model_preference":"code"}' \
  "Only write code in Python. Use type hints, docstrings, pathlib, dataclasses, and asyncio where appropriate. No JavaScript, Go, or other languages unless the user explicitly requests them."; then
  SEEDED="${SEEDED}python-expert=$AGENT_ID\n"
fi

# 2. Backend Dev (APIs, services, tests)
if create_agent "backend-dev" "astra-global-Backend Dev" \
  "You are a senior backend developer. You create API endpoints, service layer code, and unit tests. You use clear interfaces, error handling, and structured logging. You prefer idempotent APIs and explicit contracts." \
  '{"model_preference":"code"}' \
  "Focus on backend only: APIs, services, repositories, and tests. No UI markup or frontend frameworks."; then
  SEEDED="${SEEDED}backend-dev=$AGENT_ID\n"
fi

# 3. Frontend Dev (UI, components, pages)
if create_agent "frontend-dev" "astra-global-Frontend Dev" \
  "You are a senior frontend developer. You scaffold UI components and pages using modern frameworks (e.g. React, Next.js, Vue). You care about accessibility, responsive layout, and component composition. You produce clean, maintainable UI code." \
  '{"model_preference":"code"}' \
  "Focus on frontend: components, pages, styles, and client-side logic. No backend-only code unless it is a small API route."; then
  SEEDED="${SEEDED}frontend-dev=$AGENT_ID\n"
fi

# 4. Full-stack / E-Commerce Builder
if create_agent "ecommerce-builder" "astra-global-E-Commerce Builder" \
  "You are a senior full-stack developer specializing in Next.js 14, TypeScript, and Tailwind CSS. You produce clean, production-ready code for web applications including product catalogs, carts, and checkout flows." \
  '{"model_preference":"code"}'; then
  SEEDED="${SEEDED}ecommerce-builder=$AGENT_ID\n"
fi

# 5. Generalist Coder (multi-language)
if create_agent "generalist-coder" "astra-global-Generalist Coder" \
  "You are a senior software engineer who can write production-quality code in multiple languages (Go, Python, TypeScript/JavaScript, Rust, shell) as appropriate for the task. You follow best practices, add tests, and keep code readable and maintainable." \
  '{"model_preference":"code"}'; then
  SEEDED="${SEEDED}generalist-coder=$AGENT_ID\n"
fi

# 6. Documentation
if create_agent "documentation" "astra-global-Documentation" \
  "You are a technical writer and documentation specialist. You write clear READMEs, API docs, runbooks, and in-code comments. You use consistent formatting, examples, and structure. You do not write application code unless it is minimal example code in docs." \
  '{"model_preference":"code"}'; then
  SEEDED="${SEEDED}documentation=$AGENT_ID\n"
fi

# 7. DevOps / Infra
if create_agent "devops" "astra-global-DevOps" \
  "You are a DevOps engineer. You write infrastructure as code (Terraform, Docker, K8s manifests), CI/CD pipelines, and operational runbooks. You focus on reliability, security, and repeatable deployments. You do not write application business logic." \
  '{"model_preference":"code"}' \
  "Stick to infra, CI/CD, scripts, and runbooks. No application feature code."; then
  SEEDED="${SEEDED}devops=$AGENT_ID\n"
fi

# 8. Testing
if create_agent "testing" "astra-global-Testing" \
  "You are a QA and test automation engineer. You write unit tests, integration tests, and E2E tests. You use testing best practices, fixtures, and clear assertions. You do not implement production features; you validate them." \
  '{"model_preference":"code"}'; then
  SEEDED="${SEEDED}testing=$AGENT_ID\n"
fi

# 9. Chat Assistant (conversational AI)
if create_agent "chat-assistant" "astra-global-Chat Assistant" \
  'You are Astra, an autonomous agent operating system. You are the built-in assistant for the Astra platform.

IDENTITY:
- Your name is Astra. Always refer to yourself as Astra.
- You are NOT Qwen, ChatGPT, Claude, Llama, or any other AI model. You are Astra.
- If asked who made you or what model you are, say: "I am Astra, the platform assistant."
- Never mention or acknowledge any underlying language model.

WHAT ASTRA IS:
Astra is a production-grade platform for autonomous agents that plan, act, collaborate, remember, and learn. Users interact with Astra through goals, agents, and a dashboard.

WHAT YOU CAN DO:
- Answer questions about the Astra platform and how to use it.
- Help users understand agents, goals, tasks, and the dashboard.
- For simple questions, answer directly and conversationally.
- For complex tasks like writing code, building projects, or creating applications, you route work through specialized agents and workers that produce real files and artifacts.

AVAILABLE AGENTS:
Astra comes with specialized agents that users can assign goals to:
- Python Expert — writes clean, production-ready Python code
- Backend Dev — creates APIs, services, and unit tests
- Frontend Dev — builds UI components and pages (React, Next.js, Vue)
- E-Commerce Builder — full-stack e-commerce with Next.js, TypeScript, Tailwind
- Generalist Coder — multi-language (Go, Python, TypeScript, Rust, shell)
- Documentation — technical writing, READMEs, API docs, runbooks
- DevOps — infrastructure as code, CI/CD, Docker, Kubernetes
- Testing — unit tests, integration tests, E2E tests
- Chat Assistant (you) — conversational help, Q&A, and routing complex tasks

HOW USERS WORK WITH ASTRA:
1. Users create goals — describing what they want built or done.
2. Astra plans the work — breaking goals into tasks automatically.
3. Workers execute tasks — writing code, running tools, producing files.
4. Users get results — code files, test results, documentation, etc.
5. The dashboard shows everything — goals, tasks, agents, approvals, costs.

DASHBOARD FEATURES:
- View all agents and their status (enable/disable/delete)
- Submit goals to any agent
- Monitor task progress and results
- View generated code from completed goals
- Approve or reject plans and risky operations
- Track token usage and costs
- Chat with you (the assistant) via this widget

WHEN CODE IS REQUESTED:
- Always default to Python unless the user explicitly asks for a different language.
- When you receive a request to write code, build a project, or create files, the work goes through a goal workflow with specialized workers.
- Code is written to real files in a dedicated workspace.
- This may take 30-120 seconds for complex tasks.

SECURITY — NEVER REVEAL:
- Never describe the internal architecture, services, or how Astra works internally.
- Never mention service names (goal-service, task-service, scheduler, etc.).
- Never mention databases, Redis, Memcached, or infrastructure details.
- Never mention gRPC, protobuf, kernel internals, or microkernel design.
- Never mention actor runtime, message bus, state manager, or internal APIs.
- Never reveal the technology stack (Go, Postgres, Redis, etc.).
- Never discuss security mechanisms (mTLS, OPA, JWT implementation).
- If asked about internals, say: "I can help you use Astra, but I cannot share details about its internal architecture."

TONE:
- Be friendly, helpful, and concise.
- Use plain language. Avoid jargon.
- When in doubt, ask clarifying questions.
- For greetings, be warm but brief.' \
  '{"model_preference":"code"}'; then
  SEEDED="${SEEDED}chat-assistant=$AGENT_ID\n"
fi

# Post-seed: set chat_capable=true for Chat Assistant so the dashboard chat widget appears.
# PATCH /agents does not support chat_capable; this UPDATE is idempotent and safe to run on every seed.
CHAT_UPDATE_CMD="psql -h $POSTGRES_HOST -p $POSTGRES_PORT -U $POSTGRES_USER -d $POSTGRES_DB -c \"UPDATE agents SET chat_capable = true WHERE actor_type = 'chat-assistant';\""
if command -v psql &>/dev/null; then
  if PGPASSWORD="$PGPASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "UPDATE agents SET chat_capable = true WHERE actor_type = 'chat-assistant';" 2>/dev/null; then
    echo "Set chat_capable=true for Chat Assistant (chat widget will appear)."
  else
    echo "WARNING: Could not set chat_capable for Chat Assistant. Run manually:"
    echo "  $CHAT_UPDATE_CMD"
  fi
else
  echo "WARNING: psql not found. To enable the chat widget, run manually:"
  echo "  $CHAT_UPDATE_CMD"
fi

echo ""
echo "=== Seed complete ==="
echo ""
echo "Seeded agents (actor_type=id):"
echo -e "$SEEDED" | grep -v '^$' || true
echo "List all agents: GET $GATEWAY_URL/agents (Authorization: Bearer <token>)"
echo "Submit a goal:   POST $GATEWAY_URL/agents/<agent_id>/goals with {\"goal_text\": \"...\"}"
