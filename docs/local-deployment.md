# Local Deployment (Mac Mini M4 Pro)

What you need to run Astra and the agent-memory MCP on your local Mac Mini M4 (Apple Silicon).

## Prerequisites

| Requirement | Purpose | Install |
|-------------|---------|--------|
| **Docker Desktop** (or Colima) | Postgres, Redis, Memcached, MinIO | [Docker Desktop for Mac (Apple Silicon)](https://docs.docker.com/desktop/install/mac-install/) |
| **Go 1.22+** | Build and run all services | `brew install go` |
| **Node 20+** (optional) | Agent-memory MCP server | `brew install node` |
| **PostgreSQL client** | Run migrations (`psql`) | `brew install libpq` then `brew link --force libpq` or use `psql` from Postgres.app |
| **Ollama** (optional) | Embeddings for agent memory | [ollama.ai](https://ollama.ai) → `ollama pull nomic-embed-text` |
| **Redis with vector support** (optional) | Agent memory semantic search | See “Agent memory (optional)” below |

All Docker images used (`pgvector/pgvector`, `redis:7-alpine`, `memcached`, `minio`) have ARM64 builds and run natively on M4.

## 1. Infra only (Postgres, Redis, Memcached, MinIO)

```bash
cd /path/to/astra
cp .env.example .env   # edit if you need different ports/passwords
docker compose up -d
```

Wait for Postgres, then run migrations:

```bash
export PGPASSWORD=changeme
./scripts/migrate.sh
```

Or use the all-in-one script (starts compose + waits for Postgres + runs migrations):

```bash
./scripts/dev.sh
```

After this you can run Go services locally, e.g.:

```bash
go run ./cmd/api-gateway
go run ./cmd/scheduler-service   # needs POSTGRES_* and REDIS_ADDR in env or .env
```

## 2. Agent memory (optional)

The Cursor agent-memory MCP needs:

- **Redis** with vector commands (`VADD`, `VSIM`). Standard `redis:7-alpine` does **not** include these; you need **Redis Stack** (or a Redis build with the vector module).

**Option A — Redis Stack in Docker**

Replace or add a Redis Stack service in `docker-compose.yml` (e.g. use image `redis/redis-stack:latest` and expose 6379). Then point agent-memory at that Redis URL.

**Option B — Redis Stack installed on the host**

Install Redis Stack on the Mac (e.g. `brew install redis-stack` if available, or use the official Redis Stack package). Run it and set `AGENT_MEMORY_REDIS_URL` to `redis://localhost:6379/1` (or the DB you use).

Then:

```bash
cd packages/agent-memory
npm install
npm run build
node dist/bootstrap.js   # requires Ollama + nomic-embed-text
```

Cursor will start the MCP server automatically if `agent-memory` is configured in `.cursor/mcp.json` and the path `packages/agent-memory/dist/index.js` is correct.

## 3. Environment

- Copy `.env.example` to `.env` and adjust if needed.
- For Go services, either export the vars or use a tool that loads `.env` (e.g. `dotenv` or run from an env that sources `.env`).
- `scripts/dev.sh` and `scripts/migrate.sh` use `POSTGRES_*` and `PGPASSWORD`; `dev.sh` passes `PGPASSWORD=changeme` when calling the migrate script.

## 4. Quick checklist (Mac Mini M4)

1. Install Docker Desktop (or Colima), Go 1.22+, `psql` (e.g. via `libpq`).
2. Clone repo, `cp .env.example .env`.
3. `docker compose up -d` then `./scripts/dev.sh` (or run migrations manually with `PGPASSWORD=changeme ./scripts/migrate.sh`).
4. Run any service, e.g. `go run ./cmd/api-gateway`.
5. (Optional) Install Ollama, run Redis Stack, build and bootstrap agent-memory, then use Cursor with the MCP server enabled.

No architecture-specific steps are required; the stack runs on ARM64 as-is.
