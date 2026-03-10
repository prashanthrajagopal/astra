# Local Deployment

How to run Astra locally. **Only the DevOps agent** runs the deploy script; everyone else delegates to DevOps or runs it themselves.

## Single deploy script

- **Entry point:** `scripts/deploy.sh` (run from repo root: `./scripts/deploy.sh`).
- **Who runs it:** DevOps agent. Other agents must not run it; they delegate deployment requests to DevOps (or use the `/deploy` command).
- **Native-first:** If Postgres, Redis, and Memcached are already running on the host (at configured host:port), the script uses them and does not start Docker for those. Docker is used only for **missing** services. This avoids unnecessary containers when you have native Postgres/Redis/Memcached.

## Prerequisites

| Requirement | Purpose |
|-------------|---------|
| **Go 1.22+** | Build and run services (required) |
| **Postgres** (or Docker) | DB; script uses native if running, else starts via Docker |
| **Redis** (or Docker) | Messaging/cache; native if running, else Docker |
| **Memcached** (or Docker) | Cache; native if running, else Docker |
| **psql** (if using native Postgres) | Migrations; required only when Postgres is native |
| **Docker** (optional) | Only needed when one or more of Postgres/Redis/Memcached are not already running |

## Steps

1. Clone repo, copy env: `cp .env.example .env` (edit if needed). If using **native Postgres**, set `POSTGRES_USER`, `POSTGRES_DB`, and `PGPASSWORD` (or `POSTGRES_PASSWORD`) to a user and database that already exist (e.g. create with `createuser`/`createdb` or use your existing role).
2. Run deploy (DevOps runs this, or you run it): `./scripts/deploy.sh`
3. Script will: detect Postgres/Redis/Memcached (use native or start via Docker), run migrations, build Go binaries to `bin/`, start api-gateway and scheduler-service in background. Logs: `logs/*.log`; PIDs: `logs/*.pid`; stop with `kill $(cat logs/api-gateway.pid) $(cat logs/scheduler-service.pid)`.

## Agent memory (optional)

For the Cursor agent-memory MCP: Redis with vector support (Redis Stack), Ollama with `nomic-embed-text`. See `.cursor/mcp.json` and `packages/agent-memory`. Bootstrap with `node dist/bootstrap.js` in `packages/agent-memory`.

## LLM endpoint configuration

`cmd/llm-router` now uses real provider endpoints with fallback order:

1. Use requested provider model (`openai/*`, `anthropic/*`, `gemini/*`, or `ollama/*`) when corresponding credentials are available.
2. If provider credentials are missing or request fails, fallback to local Ollama model.
3. Default fallback model: `llama3:8b`.

Set in `.env`:

- `OLLAMA_HOST` (default `http://localhost:11434`)
- `OLLAMA_MODEL` (default `llama3:8b`)
- Optional cloud keys:
  - `OPENAI_API_KEY`
  - `ANTHROPIC_API_KEY`
  - `GEMINI_API_KEY`

## Workspace and code generation

Execution workers can generate code via LLM and write files to disk. Configure in `.env`:

- `WORKSPACE_ROOT` — root directory for generated projects (default `./workspace`)
- `TOOL_RUNTIME=workspace` — enables the file/shell workspace runtime
- `LLM_GRPC_ADDR` — address of the llm-router gRPC service (default `localhost:9093`)

Run the e-commerce test to verify the full pipeline:

```bash
bash examples/ecommerce-test/run.sh
```

## Mac Mini and native hardware

For Mac Mini with Metal/ANE: use the same `scripts/deploy.sh`. Run Ollama natively on the host for Metal-accelerated embeddings/inference. See [Mac Mini deployment](mac-mini-deployment.md) for details.
