#!/usr/bin/env bash
# Astra local deployment: native-first (use existing Postgres/Redis/Memcached), Docker fallback.
# GCP: use ./scripts/gcp-deploy.sh (GCS workspace bucket; no MinIO on GCP).
# Only the DevOps agent should run this. Run from repo root: ./scripts/deploy.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if [[ ! -f .env ]] && [[ -f .env.example ]]; then
  cp .env.example .env
  echo "Created .env from .env.example"
fi
if [[ -f .env ]]; then
  set -a
  source .env
  set +a
fi

# --- LLM provider auto-detection ---
export MLX_HOST="${MLX_HOST:-http://localhost:8888}"
export MLX_MODEL="${MLX_MODEL:-Qwen2.5-7B-Instruct-4bit}"
export LLM_DEFAULT_PROVIDER="${LLM_DEFAULT_PROVIDER:-ollama}"

export POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
export POSTGRES_PORT="${POSTGRES_PORT:-5432}"
export POSTGRES_DB="${POSTGRES_DB:-astra}"
export POSTGRES_USER="${POSTGRES_USER:-astra}"
export PGPASSWORD="${PGPASSWORD:-${POSTGRES_PASSWORD:-changeme}}"

# Parse REDIS_ADDR and MEMCACHED_ADDR (host:port)
REDIS_HOST="${REDIS_HOST:-localhost}"
REDIS_PORT="${REDIS_PORT:-6379}"
if [[ -n "${REDIS_ADDR:-}" ]]; then
  REDIS_HOST="${REDIS_ADDR%%:*}"
  REDIS_PORT="${REDIS_ADDR##*:}"
  [[ "$REDIS_PORT" == "$REDIS_ADDR" ]] && REDIS_PORT="6379"
fi
MEMCACHED_HOST="${MEMCACHED_HOST:-localhost}"
MEMCACHED_PORT="${MEMCACHED_PORT:-11211}"
if [[ -n "${MEMCACHED_ADDR:-}" ]]; then
  MEMCACHED_HOST="${MEMCACHED_ADDR%%:*}"
  MEMCACHED_PORT="${MEMCACHED_ADDR##*:}"
  [[ "$MEMCACHED_PORT" == "$MEMCACHED_ADDR" ]] && MEMCACHED_PORT="11211"
fi

# Resolve psql (PATH or common install locations so deploy works outside interactive shell)
PSQL=""
if command -v psql &>/dev/null; then
  PSQL="psql"
elif [[ -x /opt/homebrew/bin/psql ]]; then
  PSQL="/opt/homebrew/bin/psql"
elif [[ -x /usr/local/bin/psql ]]; then
  PSQL="/usr/local/bin/psql"
fi

# --- Detection helpers (portable: nc or /dev/tcp) ---
tcp_ok() {
  if command -v nc &>/dev/null; then
    nc -z "$1" "$2" 2>/dev/null
  else
    (echo >/dev/tcp/"$1"/"$2") 2>/dev/null
  fi
}
pg_ready() {
  if command -v pg_isready &>/dev/null; then
    pg_isready -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" &>/dev/null
  else
    tcp_ok "$POSTGRES_HOST" "$POSTGRES_PORT"
  fi
}
redis_ok() {
  if command -v redis-cli &>/dev/null; then
    redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" ping 2>/dev/null | grep -q PONG
  else
    tcp_ok "$REDIS_HOST" "$REDIS_PORT"
  fi
}
memcached_ok() { tcp_ok "$MEMCACHED_HOST" "$MEMCACHED_PORT"; }

# On macOS, prefer MLX-LM if available (unless cloud keys are set)
DETECTED_OS="$(uname -s)"
if [[ "$DETECTED_OS" == "Darwin" ]]; then
  HAS_CLOUD_KEYS=false
  if [[ -n "${OPENAI_API_KEY:-}" ]] || [[ -n "${ANTHROPIC_API_KEY:-}" ]] || [[ -n "${GEMINI_API_KEY:-}" ]]; then
    HAS_CLOUD_KEYS=true
  fi

  if [[ "$HAS_CLOUD_KEYS" == "false" ]]; then
    MLX_CHECK_HOST="${MLX_HOST#http://}"
    MLX_CHECK_HOST="${MLX_CHECK_HOST#https://}"
    MLX_CHECK_PORT="${MLX_CHECK_HOST##*:}"
    MLX_CHECK_HOST="${MLX_CHECK_HOST%%:*}"
    [[ "$MLX_CHECK_PORT" == "$MLX_CHECK_HOST" ]] && MLX_CHECK_PORT="8888"

    if tcp_ok "$MLX_CHECK_HOST" "$MLX_CHECK_PORT"; then
      export LLM_DEFAULT_PROVIDER="mlx"
      echo "MLX-LM: detected on $MLX_HOST (macOS) — using as default LLM provider"
    else
      echo "MLX-LM: not reachable on $MLX_HOST (macOS) — using Ollama"
    fi
  else
    echo "Cloud API keys detected — using cloud LLM providers"
  fi
fi

POSTGRES_SOURCE=""
REDIS_SOURCE=""
MEMCACHED_SOURCE=""

echo "=== Astra local deploy (native-first) ==="
echo "Repo: $REPO_ROOT"
echo ""

# --- Postgres ---
if pg_ready; then
  POSTGRES_SOURCE="native"
  echo "Postgres: native (already running)"
else
  if ! command -v docker &>/dev/null || ! docker info &>/dev/null; then
    echo "Postgres not running and Docker unavailable. Start Postgres or Docker and re-run."
    exit 1
  fi
  echo "Postgres: not found, starting with Docker..."
  docker compose up -d postgres
  until docker compose exec -T postgres pg_isready -U "$POSTGRES_USER" 2>/dev/null; do sleep 1; done
  POSTGRES_SOURCE="Docker"
  echo "Postgres: Docker (ready)"
fi

# --- Bootstrap Postgres role & database (native only) ---
if [[ "$POSTGRES_SOURCE" == "native" ]]; then
  # Determine which superuser to connect as for bootstrapping.
  # On macOS Homebrew, the default superuser is often the current OS user.
  # On Linux, it's typically "postgres".
  PG_SUPERUSER="${PG_SUPERUSER:-}"
  if [[ -z "$PG_SUPERUSER" ]]; then
    if [[ "$DETECTED_OS" == "Darwin" ]]; then
      PG_SUPERUSER="$(whoami)"
    else
      PG_SUPERUSER="postgres"
    fi
  fi

  if [[ -n "$PSQL" ]]; then
    # Check/create role
    ROLE_EXISTS=$("$PSQL" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$PG_SUPERUSER" -tAc \
      "SELECT 1 FROM pg_roles WHERE rolname = '$POSTGRES_USER'" postgres 2>/dev/null || true)
    if [[ "$ROLE_EXISTS" != "1" ]]; then
      echo "Creating Postgres role '$POSTGRES_USER'..."
      "$PSQL" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$PG_SUPERUSER" -c \
        "CREATE ROLE $POSTGRES_USER WITH LOGIN SUPERUSER PASSWORD '$PGPASSWORD';" postgres 2>/dev/null \
        && echo "  Role '$POSTGRES_USER' created (superuser)." \
        || echo "  WARNING: Could not create role '$POSTGRES_USER'. Create it manually: CREATE ROLE $POSTGRES_USER WITH LOGIN SUPERUSER PASSWORD '\$PGPASSWORD';"
    else
      # Ensure existing role has superuser (required for CREATE EXTENSION vector)
      "$PSQL" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$PG_SUPERUSER" -c \
        "ALTER ROLE $POSTGRES_USER WITH SUPERUSER;" postgres 2>/dev/null \
        && echo "  Role '$POSTGRES_USER' granted superuser." \
        || true
    fi

    # Check/create database
    DB_EXISTS=$("$PSQL" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$PG_SUPERUSER" -tAc \
      "SELECT 1 FROM pg_database WHERE datname = '$POSTGRES_DB'" postgres 2>/dev/null || true)
    if [[ "$DB_EXISTS" != "1" ]]; then
      echo "Creating Postgres database '$POSTGRES_DB'..."
      "$PSQL" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$PG_SUPERUSER" -c \
        "CREATE DATABASE $POSTGRES_DB OWNER $POSTGRES_USER;" postgres 2>/dev/null \
        && echo "  Database '$POSTGRES_DB' created." \
        || echo "  WARNING: Could not create database '$POSTGRES_DB'. Create it manually: CREATE DATABASE $POSTGRES_DB OWNER $POSTGRES_USER;"
    fi
  else
    echo "  psql not found (checked PATH, /opt/homebrew/bin/psql, /usr/local/bin/psql). Cannot verify role/database. If migrations fail, create them manually:"
    echo "    CREATE ROLE $POSTGRES_USER WITH LOGIN PASSWORD '\$PGPASSWORD';"
    echo "    CREATE DATABASE $POSTGRES_DB OWNER $POSTGRES_USER;"
  fi
fi

# --- Ensure pgvector extension is available (native only) ---
if [[ "$POSTGRES_SOURCE" == "native" ]] && [[ -n "$PSQL" ]]; then
  VECTOR_AVAILABLE=$("$PSQL" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc \
    "SELECT 1 FROM pg_available_extensions WHERE name = 'vector'" 2>/dev/null || true)
  if [[ "$VECTOR_AVAILABLE" != "1" ]]; then
    echo "pgvector extension not found — attempting install..."
    if [[ "$DETECTED_OS" == "Darwin" ]]; then
      if command -v brew &>/dev/null; then
        brew install pgvector 2>&1 | tail -3
      else
        echo "  WARNING: Homebrew not found. Install pgvector manually: brew install pgvector"
      fi
    else
      if command -v apt-get &>/dev/null; then
        sudo apt-get install -y postgresql-14-pgvector 2>/dev/null \
          || sudo apt-get install -y postgresql-pgvector 2>/dev/null \
          || echo "  WARNING: Could not install pgvector via apt. Install manually."
      elif command -v dnf &>/dev/null; then
        sudo dnf install -y pgvector 2>/dev/null \
          || echo "  WARNING: Could not install pgvector via dnf. Install manually."
      else
        echo "  WARNING: No supported package manager found. Install pgvector manually."
        echo "  See: https://github.com/pgvector/pgvector#installation"
      fi
    fi
    # Re-check after install attempt
    VECTOR_AVAILABLE=$("$PSQL" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc \
      "SELECT 1 FROM pg_available_extensions WHERE name = 'vector'" 2>/dev/null || true)
    if [[ "$VECTOR_AVAILABLE" == "1" ]]; then
      echo "  pgvector: installed and available."
    else
      echo "  WARNING: pgvector still not available. Migrations requiring 'CREATE EXTENSION vector' will fail."
      echo "  Install manually and re-run: https://github.com/pgvector/pgvector#installation"
    fi
  fi
fi

# --- Redis ---
if redis_ok; then
  REDIS_SOURCE="native"
  echo "Redis: native (already running)"
else
  if ! command -v docker &>/dev/null || ! docker info &>/dev/null; then
    echo "Redis not running and Docker unavailable. Start Redis or Docker and re-run."
    exit 1
  fi
  echo "Redis: not found, starting with Docker..."
  docker compose up -d redis
  until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done
  REDIS_SOURCE="Docker"
  echo "Redis: Docker (ready)"
fi

# --- Memcached (prefer local binary if installed; otherwise Docker) ---
if memcached_ok; then
  MEMCACHED_SOURCE="native"
  echo "Memcached: native (already running)"
else
  if command -v memcached &>/dev/null; then
    echo "Memcached: starting local memcached..."
    memcached -d -p "${MEMCACHED_PORT:-11211}" -l "${MEMCACHED_HOST:-127.0.0.1}" 2>/dev/null || true
    sleep 1
    if memcached_ok; then
      MEMCACHED_SOURCE="native"
      echo "Memcached: native (started locally)"
    else
      if ! command -v docker &>/dev/null || ! docker info &>/dev/null; then
        echo "Memcached: local start failed and Docker unavailable. Start memcached manually or run Docker."
        exit 1
      fi
      echo "Memcached: local start failed, starting with Docker..."
      docker compose up -d memcached
      until tcp_ok "localhost" "${MEMCACHED_PORT:-11211}"; do sleep 1; done
      MEMCACHED_SOURCE="Docker"
      echo "Memcached: Docker (ready)"
    fi
  else
    if ! command -v docker &>/dev/null || ! docker info &>/dev/null; then
      echo "Memcached not installed and Docker unavailable. Install memcached (e.g. brew install memcached) or Docker and re-run."
      exit 1
    fi
    echo "Memcached: not found, starting with Docker..."
    docker compose up -d memcached
    until tcp_ok "localhost" "${MEMCACHED_PORT:-11211}"; do sleep 1; done
    MEMCACHED_SOURCE="Docker"
    echo "Memcached: Docker (ready)"
  fi
fi

echo ""
echo "Migrations..."

run_migration() {
  local f="$1"
  if [[ "$POSTGRES_SOURCE" == "Docker" ]]; then
    docker compose exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f - -v ON_ERROR_STOP=1 < "$f"
  else
    if [[ -n "$PSQL" ]]; then
      "$PSQL" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f "$f" -v ON_ERROR_STOP=1
    elif command -v docker &>/dev/null && docker info &>/dev/null; then
      docker run --rm -i --add-host=host.docker.internal:host-gateway \
        -e PGPASSWORD="$PGPASSWORD" \
        postgres:17-alpine psql -h host.docker.internal -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f - -v ON_ERROR_STOP=1 < "$f"
    else
      echo "Native Postgres in use but psql not found (PATH, /opt/homebrew/bin/psql, /usr/local/bin/psql) and Docker unavailable. Install PostgreSQL client (psql) or Docker."
      exit 1
    fi
  fi
}

for f in migrations/*.sql; do
  [[ -f "$f" ]] || continue
  echo "  $(basename "$f")"
  run_migration "$f"
done
echo "Migrations done."

echo ""
if ! command -v go &>/dev/null; then
  echo "Error: go not in PATH. Install Go and re-run."
  exit 1
fi
echo "Go mod tidy..."
go mod tidy
mkdir -p bin logs "${WORKSPACE_ROOT:-workspace}"

# Keep dashboard Swagger in sync with canonical OpenAPI spec (embedded at build time)
if [[ -f docs/api/openapi.yaml ]]; then
  cp docs/api/openapi.yaml cmd/api-gateway/dashboard/static/openapi.yaml
  echo "Synced OpenAPI spec to dashboard (Swagger)."
fi

echo "Building..."
ASTRA_VERSION="$(cat VERSION 2>/dev/null || echo dev)"
GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-X astra/pkg/version.Version=${ASTRA_VERSION} -X astra/pkg/version.GitCommit=${GIT_COMMIT} -X astra/pkg/version.BuildDate=${BUILD_DATE}"
go build -ldflags "$LDFLAGS" -o bin/api-gateway        ./cmd/api-gateway
go build -ldflags "$LDFLAGS" -o bin/scheduler-service  ./cmd/scheduler-service
go build -ldflags "$LDFLAGS" -o bin/task-service       ./cmd/task-service
go build -ldflags "$LDFLAGS" -o bin/agent-service      ./cmd/agent-service
go build -ldflags "$LDFLAGS" -o bin/execution-worker   ./cmd/execution-worker
go build -ldflags "$LDFLAGS" -o bin/worker-manager     ./cmd/worker-manager
go build -ldflags "$LDFLAGS" -o bin/tool-runtime       ./cmd/tool-runtime
go build -ldflags "$LDFLAGS" -o bin/browser-worker     ./cmd/browser-worker
go build -ldflags "$LDFLAGS" -o bin/memory-service     ./cmd/memory-service
go build -ldflags "$LDFLAGS" -o bin/llm-router         ./cmd/llm-router
go build -ldflags "$LDFLAGS" -o bin/prompt-manager     ./cmd/prompt-manager
go build -ldflags "$LDFLAGS" -o bin/identity           ./cmd/identity
go build -ldflags "$LDFLAGS" -o bin/access-control     ./cmd/access-control
go build -ldflags "$LDFLAGS" -o bin/planner-service    ./cmd/planner-service
go build -ldflags "$LDFLAGS" -o bin/goal-service       ./cmd/goal-service
go build -ldflags "$LDFLAGS" -o bin/evaluation-service ./cmd/evaluation-service
go build -ldflags "$LDFLAGS" -o bin/cost-tracker       ./cmd/cost-tracker
echo "Build done."

echo ""
echo "Restarting all services..."
set -a; source .env 2>/dev/null || true; set +a

SERVICES="task-service agent-service scheduler-service execution-worker worker-manager tool-runtime browser-worker memory-service llm-router prompt-manager identity access-control planner-service goal-service evaluation-service cost-tracker api-gateway"

# Stop all running Astra services (by PID file)
echo "  Stopping existing services..."
for svc in $SERVICES; do
  if [[ -f "logs/${svc}.pid" ]]; then
    pid=$(cat "logs/${svc}.pid")
    kill "$pid" 2>/dev/null || true
    rm -f "logs/${svc}.pid"
  fi
done
sleep 2

# Start all services
echo "  Starting services..."

./bin/task-service       > logs/task-service.log 2>&1 &
echo $! > logs/task-service.pid
./bin/agent-service      > logs/agent-service.log 2>&1 &
echo $! > logs/agent-service.pid
./bin/scheduler-service  > logs/scheduler-service.log 2>&1 &
echo $! > logs/scheduler-service.pid
./bin/execution-worker   > logs/execution-worker.log 2>&1 &
echo $! > logs/execution-worker.pid
./bin/worker-manager     > logs/worker-manager.log 2>&1 &
echo $! > logs/worker-manager.pid
./bin/tool-runtime       > logs/tool-runtime.log 2>&1 &
echo $! > logs/tool-runtime.pid
./bin/browser-worker     > logs/browser-worker.log 2>&1 &
echo $! > logs/browser-worker.pid
./bin/memory-service     > logs/memory-service.log 2>&1 &
echo $! > logs/memory-service.pid
./bin/llm-router         > logs/llm-router.log 2>&1 &
echo $! > logs/llm-router.pid
./bin/prompt-manager     > logs/prompt-manager.log 2>&1 &
echo $! > logs/prompt-manager.pid
./bin/identity           > logs/identity.log 2>&1 &
echo $! > logs/identity.pid
./bin/access-control     > logs/access-control.log 2>&1 &
echo $! > logs/access-control.pid
./bin/planner-service    > logs/planner-service.log 2>&1 &
echo $! > logs/planner-service.pid
./bin/goal-service       > logs/goal-service.log 2>&1 &
echo $! > logs/goal-service.pid
./bin/evaluation-service > logs/evaluation-service.log 2>&1 &
echo $! > logs/evaluation-service.pid
./bin/cost-tracker       > logs/cost-tracker.log 2>&1 &
echo $! > logs/cost-tracker.pid
sleep 1
./bin/api-gateway        > logs/api-gateway.log 2>&1 &
echo $! > logs/api-gateway.pid

echo "Chat WebSocket: ${CHAT_ENABLED:-false} (set CHAT_ENABLED=true to enable)"

# Seed super-admin user (idempotent)
SA_EMAIL="${ASTRA_SUPER_ADMIN_EMAIL:-admin@astra.local}"
SA_PASS="${ASTRA_SUPER_ADMIN_PASSWORD:-changeme-admin}"
echo "Seeding super-admin user ($SA_EMAIL)..."

# Wait for identity service
for i in 1 2 3 4 5; do
  if curl -sf "http://localhost:${IDENTITY_PORT:-8085}/health" >/dev/null 2>&1; then break; fi
  sleep 2
done

# Try to create via identity service API (idempotent — will fail if already exists)
SEED_RESP=$(curl -s -w "\n%{http_code}" -X POST "http://localhost:${IDENTITY_PORT:-8085}/users" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$SA_EMAIL\",\"name\":\"Super Admin\",\"password\":\"$SA_PASS\",\"is_super_admin\":true}" 2>/dev/null)
SEED_CODE=$(echo "$SEED_RESP" | tail -1)
if [[ "$SEED_CODE" == "201" || "$SEED_CODE" == "200" ]]; then
  echo "  Super-admin created: $SA_EMAIL"
elif echo "$SEED_RESP" | grep -qi "already\|duplicate\|exists\|unique"; then
  echo "  Super-admin already exists: $SA_EMAIL"
else
  echo "  Super-admin seed: HTTP $SEED_CODE (may already exist)"
fi

echo ""
echo "Seeding default agents (idempotent; skips existing)..."
# Wait for api-gateway to be ready so seed can call GET /agents (avoids duplicate agents)
echo "Waiting for api-gateway before seeding agents..."
sleep 5
if [[ -f "$REPO_ROOT/scripts/seed-agents.sh" ]]; then
  "$REPO_ROOT/scripts/seed-agents.sh" || true
else
  echo "  (seed-agents.sh not found; skip)"
fi

echo ""
echo "=== Deploy complete ==="
echo "Infra: Postgres=$POSTGRES_SOURCE  Redis=$REDIS_SOURCE  Memcached=$MEMCACHED_SOURCE"
if [[ "$LLM_DEFAULT_PROVIDER" == "mlx" ]]; then
  echo "LLM:    MLX-LM on $MLX_HOST (model: $MLX_MODEL)"
elif [[ -n "${OPENAI_API_KEY:-}" ]] || [[ -n "${ANTHROPIC_API_KEY:-}" ]] || [[ -n "${GEMINI_API_KEY:-}" ]]; then
  echo "LLM:    Cloud providers configured"
else
  echo "LLM:    Ollama on ${OLLAMA_HOST:-http://localhost:11434}"
fi
echo "Services: task-service, agent-service, scheduler-service, execution-worker, worker-manager, tool-runtime, browser-worker, memory-service, llm-router, prompt-manager, identity, access-control, planner-service, goal-service, evaluation-service, cost-tracker, api-gateway"
echo "Logs:  logs/*.log"
echo "PIDs:  logs/*.pid"
echo "Stop:  for f in logs/*.pid; do kill \$(cat \$f) 2>/dev/null; done"
