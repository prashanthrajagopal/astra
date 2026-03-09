# Deploying Astra on Mac Mini (Native Hardware)

Run Astra on a Mac Mini with native Postgres/Redis/Memcached when possible, and use Metal for inference via Ollama.

## Single deploy script (DevOps only)

- **Entry point:** `scripts/deploy.sh` (run from repo root).
- **Who runs it:** Only the DevOps agent. Others delegate to DevOps or use `/deploy`.
- **Native-first:** If you have Postgres, Redis, and Memcached installed and running on the Mac, the script uses them and does not start Docker. Docker is used only for services that are not already available.

## Steps

### 1. Run the deploy script

```bash
cd /path/to/astra
cp .env.example .env   # optional
./scripts/deploy.sh
```

The script will use your native Postgres, Redis, and Memcached if they are running at the configured host:port; otherwise it starts only the missing ones via Docker. Then it runs migrations, builds Go binaries to `bin/`, and starts api-gateway and scheduler-service in the background.

### 2. Ollama on the host (Metal)

Install and run Ollama **on macOS** (not in Docker) so it can use Metal:

```bash
# Install: https://ollama.ai or brew install ollama
ollama serve
ollama pull nomic-embed-text
ollama pull llama3.2   # or any model you use
```

Use `OLLAMA_HOST=http://localhost:11434` when pointing Astra or agent-memory at it.

### 3. Optional: start on boot

Use launchd to run Astra services and Ollama on login. Example plist for api-gateway: set `ProgramArguments` to `./bin/api-gateway`, `WorkingDirectory` to the repo, and `EnvironmentVariables` from `.env`. Place in `~/Library/LaunchAgents/` and `launchctl load`.

## Summary

| Component        | Where        | Uses Mac hardware   |
|-----------------|--------------|---------------------|
| Postgres, Redis, Memcached | Native (if running) or Docker | — |
| Astra Go services        | Native (`bin/`)     | CPU                 |
| Ollama                    | Native (host)      | **Metal** (and ANE when supported) |

Deployment is performed only by the DevOps agent; request deployment by delegating to DevOps or running `./scripts/deploy.sh` yourself.
