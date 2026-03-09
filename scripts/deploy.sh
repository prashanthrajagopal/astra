#!/usr/bin/env bash
# Astra local deployment: native-first (use existing Postgres/Redis/Memcached), Docker fallback.
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

# --- Memcached ---
if memcached_ok; then
  MEMCACHED_SOURCE="native"
  echo "Memcached: native (already running)"
else
  if ! command -v docker &>/dev/null || ! docker info &>/dev/null; then
    echo "Memcached not running and Docker unavailable. Start Memcached or Docker and re-run."
    exit 1
  fi
  echo "Memcached: not found, starting with Docker..."
  docker compose up -d memcached
  until tcp_ok "localhost" "11211"; do sleep 1; done
  MEMCACHED_SOURCE="Docker"
  echo "Memcached: Docker (ready)"
fi

echo ""
echo "Migrations..."

run_migration() {
  local f="$1"
  if [[ "$POSTGRES_SOURCE" == "Docker" ]]; then
    docker compose exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f - -v ON_ERROR_STOP=1 < "$f"
  else
    if command -v psql &>/dev/null; then
      psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f "$f" -v ON_ERROR_STOP=1
    elif command -v docker &>/dev/null && docker info &>/dev/null; then
      docker run --rm -i --add-host=host.docker.internal:host-gateway \
        -e PGPASSWORD="$PGPASSWORD" \
        postgres:17-alpine psql -h host.docker.internal -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f - -v ON_ERROR_STOP=1 < "$f"
    else
      echo "Native Postgres in use but psql not in PATH and Docker unavailable. Install PostgreSQL client (psql) or Docker."
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
mkdir -p bin logs
echo "Building..."
go build -o bin/api-gateway        ./cmd/api-gateway
go build -o bin/scheduler-service  ./cmd/scheduler-service
go build -o bin/task-service       ./cmd/task-service
go build -o bin/agent-service      ./cmd/agent-service
go build -o bin/execution-worker   ./cmd/execution-worker
echo "Build done."

echo ""
echo "Starting services..."
set -a; source .env 2>/dev/null || true; set +a

SERVICES="task-service agent-service scheduler-service execution-worker api-gateway"
for svc in $SERVICES; do
  if [[ -f "logs/${svc}.pid" ]]; then
    kill "$(cat "logs/${svc}.pid")" 2>/dev/null || true
    rm -f "logs/${svc}.pid"
  fi
done
sleep 1

./bin/task-service       > logs/task-service.log 2>&1 &
echo $! > logs/task-service.pid
./bin/agent-service      > logs/agent-service.log 2>&1 &
echo $! > logs/agent-service.pid
./bin/scheduler-service  > logs/scheduler-service.log 2>&1 &
echo $! > logs/scheduler-service.pid
./bin/execution-worker   > logs/execution-worker.log 2>&1 &
echo $! > logs/execution-worker.pid
sleep 1
./bin/api-gateway        > logs/api-gateway.log 2>&1 &
echo $! > logs/api-gateway.pid

echo ""
echo "=== Deploy complete ==="
echo "Infra: Postgres=$POSTGRES_SOURCE  Redis=$REDIS_SOURCE  Memcached=$MEMCACHED_SOURCE"
echo "Services: task-service, agent-service, scheduler-service, execution-worker, api-gateway"
echo "Logs:  logs/*.log"
echo "PIDs:  logs/*.pid"
echo "Stop:  for f in logs/*.pid; do kill \$(cat \$f) 2>/dev/null; done"
