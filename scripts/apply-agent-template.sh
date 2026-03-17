#!/usr/bin/env bash
# Apply a JSON agent template via api-gateway (requires ASTRA_TOKEN and API base).
set -euo pipefail
TEMPLATE="${1:?usage: apply-agent-template.sh path/to/pack.json}"
API_BASE="${ASTRA_API_BASE:-http://localhost:8080}"
TOKEN="${ASTRA_TOKEN:?set ASTRA_TOKEN}"
jq -e .name "$TEMPLATE" >/dev/null
BODY=$(jq -c '{name, actor_type, system_prompt, chat_capable, allowed_tools} | with_entries(select(.value != null))' "$TEMPLATE")
curl -sS -X POST "$API_BASE/agents" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "$BODY" | jq .
