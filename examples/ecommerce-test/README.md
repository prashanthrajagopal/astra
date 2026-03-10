# E-Commerce Test — Real-World Astra Validation

This example drives Astra end-to-end: an autonomous agent plans and builds a
fully functional Next.js 14 e-commerce website from a single goal description.

## What it tests

1. **LLM-backed planning** — The goal service uses the LLM router to decompose
   a high-level goal into a DAG of 8–15 concrete tasks.
2. **Code generation** — Execution workers call the LLM to generate TypeScript,
   React components, and configuration files.
3. **Workspace file I/O** — The `WorkspaceRuntime` writes generated files to
   the project directory.
4. **Shell execution** — `shell_exec` tasks run commands like `npm install`.
5. **Task orchestration** — The scheduler respects task dependencies, running
   independent tasks in parallel.
6. **Approval flow** — Dangerous operations (shell commands) trigger approval
   requests that must be approved before execution proceeds.
7. **Dashboard visibility** — All progress is visible in the Astra dashboard
   at `http://localhost:8080/dashboard/`.

## Prerequisites

- Astra services running (`scripts/deploy.sh`)
- An LLM provider configured in `.env` (OpenAI, Claude, Gemini, or Ollama)
- Node.js 18+ (to run the generated Next.js project)

## Quick start

```bash
# From the astra repo root:
bash examples/ecommerce-test/run.sh
```

The script will:
1. Verify Astra services are up (or start them).
2. Create an agent and submit the e-commerce goal.
3. Auto-approve pending requests (`--auto-approve`).
4. Poll task status until the goal completes.
5. Print instructions to run the generated site.

## Manual run

```bash
go run ./examples/ecommerce-test/ \
  --gateway http://localhost:8080 \
  --identity http://localhost:8085 \
  --goal-service http://localhost:8088 \
  --access-control http://localhost:8086 \
  --workspace ./workspace/ecommerce-store \
  --auto-approve \
  --poll 10s \
  --timeout 30m
```

## Running the generated site

After the goal completes:

```bash
cd workspace/ecommerce-store
npm install   # if not already done by the agent
npm run dev
# Open http://localhost:3000
```

## LLM provider recommendations

| Provider | Model | Quality | Speed |
|----------|-------|---------|-------|
| OpenAI | gpt-4o | Best | Fast |
| Anthropic | claude-3.5-sonnet | Excellent | Fast |
| Ollama | llama3:8b | Variable | Slow |

For reliable results, set `OPENAI_API_KEY` or `ANTHROPIC_API_KEY` in `.env`.
