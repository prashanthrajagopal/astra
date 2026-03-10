#!/usr/bin/env bash
# Create a Python-expert agent in Astra via the public APIs.
# Prerequisites: Astra services running (api-gateway, identity). Run from repo root.
# Usage: ./scripts/create-python-expert-agent.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
IDENTITY_URL="${IDENTITY_URL:-http://localhost:8085}"

echo "=== Create Python Expert Agent (Astra API) ==="
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
if [[ -z "$TOKEN" ]]; then
  if command -v jq &>/dev/null; then
    TOKEN=$(echo "$TOKEN_RESP" | jq -r '.token')
  fi
fi
if [[ -z "$TOKEN" ]]; then
  echo "Could not parse token from Identity response."
  exit 1
fi
echo "Token obtained."
echo ""

# 2. Create Python Expert agent via API Gateway
echo "Creating Python Expert agent..."
AGENT_RESP=$(curl -s -X POST "$GATEWAY_URL/agents" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "actor_type": "python-expert",
    "name": "Python Expert",
    "system_prompt": "You are a senior Python expert. You write clean, idiomatic Python (3.10+), follow PEP 8, use type hints, and prefer the standard library. You produce production-ready code with tests when appropriate. You do not write in other languages unless explicitly asked.",
    "config": "{\"model_preference\":\"code\"}"
  }' || true)
if ! echo "$AGENT_RESP" | grep -q 'actor_id'; then
  echo "Failed to create agent. Response: $AGENT_RESP"
  exit 1
fi
AGENT_ID=$(echo "$AGENT_RESP" | sed -n 's/.*"actor_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
if [[ -z "$AGENT_ID" ]] && command -v jq &>/dev/null; then
  AGENT_ID=$(echo "$AGENT_RESP" | jq -r '.actor_id')
fi
if [[ -z "$AGENT_ID" ]]; then
  echo "Could not parse actor_id from response."
  exit 1
fi
echo "Agent created: $AGENT_ID"
echo ""

# 3. Attach a Python-only rule document (optional)
echo "Attaching Python expert rule document..."
DOC_RESP=$(curl -s -w "\n%{http_code}" -X POST "$GATEWAY_URL/agents/$AGENT_ID/documents" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "doc_type": "rule",
    "name": "python-only",
    "content": "Only write code in Python. Use type hints, docstrings, and prefer pathlib, dataclasses, and asyncio where appropriate. No JavaScript, Go, or other languages unless the user explicitly requests them.",
    "priority": 50
  }' || true)
HTTP_CODE=$(echo "$DOC_RESP" | tail -n1)
if [[ "$HTTP_CODE" != "201" ]] && [[ "$HTTP_CODE" != "200" ]]; then
  echo "Document attach returned HTTP $HTTP_CODE (non-fatal)."
else
  echo "Rule document attached."
fi
echo ""

echo "=== Python Expert Agent Ready ==="
echo "Agent ID: $AGENT_ID"
echo ""
echo "Submit a goal:"
echo "  curl -X POST $GATEWAY_URL/agents/$AGENT_ID/goals \\"
echo "    -H \"Authorization: Bearer \$TOKEN\" \\"
echo "    -H \"Content-Type: application/json\" \\"
echo "    -d '{\"goal_text\": \"Write a Python script that lists all CSV files in a directory and prints their row counts\"}'"
echo ""
echo "Get tasks/graphs via dashboard or: GET $GATEWAY_URL/tasks/{taskId}, GET $GATEWAY_URL/graphs/{graphId}"
