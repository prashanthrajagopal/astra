/**
 * Bootstrap script to seed the agent memory store with curated Astra project memories.
 * Does NOT parse transcripts — directly stores the defined memories.
 */
import { v4 as uuidv4 } from 'uuid';
import {
  ensureConnected,
  storeMemory,
  computeExpiresAt,
  getMemoryStats,
  shutdown,
  type MemoryEntry,
} from './redis';
import { embed, healthCheck } from './embeddings';

const PROJECT = process.env.AGENT_MEMORY_PROJECT || 'astra-kernel';

interface ExtractedMemory {
  type: MemoryEntry['type'];
  summary: string;
  detail?: string;
  tags: string[];
  files?: string[];
  layer?: string;
}

const SEED_MEMORIES: ExtractedMemory[] = [
  // Architecture Decisions (1-8)
  {
    type: 'decision',
    summary: 'Astra uses a microkernel architecture with 5 core components: Actor Runtime, Task Graph Engine, Scheduler, Message Bus, and State Manager. The kernel exposes gRPC APIs; all other services run in user-space.',
    tags: ['architecture', 'kernel', 'microkernel'],
    layer: 'kernel',
    files: ['docs/PRD.md', 'internal/kernel/kernel.go'],
  },
  {
    type: 'decision',
    summary: 'Postgres is the single source of truth for all durable state. Redis handles ephemeral state, streams, and distributed locks. Memcached caches LLM responses and embeddings.',
    tags: ['architecture', 'postgres', 'redis', 'memcached'],
    layer: 'infrastructure',
    files: ['docker-compose.yml', 'pkg/db/db.go'],
  },
  {
    type: 'decision',
    summary: 'Actor model uses goroutine-per-actor with 1024-capacity buffered mailboxes and supervision trees. BaseActor implements non-blocking Receive with ErrMailboxFull on overflow.',
    tags: ['architecture', 'actors', 'concurrency'],
    layer: 'kernel',
    files: ['internal/actors/actor.go', 'internal/actors/supervisor.go'],
  },
  {
    type: 'decision',
    summary: 'Task engine uses a 6-state lifecycle (created→pending→queued→scheduled→running→completed/failed) with transactional state transitions that append to the events table for event sourcing.',
    tags: ['architecture', 'tasks', 'event-sourcing'],
    layer: 'kernel',
    files: ['internal/tasks/task.go', 'internal/tasks/store.go'],
  },
  {
    type: 'decision',
    summary: "Merged Product Owner into Project Manager as a single entry point. Astra's detailed engineering PRD doesn't need separate product/project roles. Also decided against a Business Analyst agent since Astra is infrastructure, not a business-domain product.",
    tags: ['architecture', 'agents', 'workflow'],
    layer: 'cursor_config',
    files: ['.cursor/agents/project-manager.md'],
  },
  {
    type: 'decision',
    summary: 'Scheduler uses FOR UPDATE SKIP LOCKED for lock-free ready task detection across multiple scheduler instances. Sharding via consistent hashing on agent_id or graph_id.',
    tags: ['architecture', 'scheduler', 'postgres'],
    layer: 'kernel',
    files: ['internal/scheduler/scheduler.go', 'internal/tasks/store.go'],
  },
  {
    type: 'decision',
    summary: 'Tool sandboxing supports three levels: WASM for lightweight, Docker containers for standard, Firecracker microVMs for untrusted code. Each sandbox has memory/CPU limits and timeouts.',
    tags: ['architecture', 'tools', 'sandbox', 'security'],
    layer: 'services',
    files: ['internal/tools/runtime.go'],
  },
  {
    type: 'decision',
    summary: 'LLM router selects between local, premium, and code-specialized model tiers based on task type and priority. Response caching in Memcached to reduce costs.',
    tags: ['architecture', 'llm', 'cost-optimization'],
    layer: 'services',
    files: ['internal/llm/router.go'],
  },
  // Work Summaries (9-14, 21-22)
  {
    type: 'work_summary',
    summary: 'Created comprehensive consolidated PRD (docs/PRD.md, 1677 lines) from 3 source scaffold repos and original PRD. Includes full Go code snippets, proto definitions, SQL migrations, Redis stream patterns, and 6-phase implementation roadmap.',
    tags: ['prd', 'documentation', 'planning'],
    layer: 'documentation',
    files: ['docs/PRD.md'],
  },
  {
    type: 'work_summary',
    summary: 'Scaffolded full monorepo with 16 service entrypoints (cmd/), 13 internal packages, 7 shared packages (pkg/), 2 proto files, 9 SQL migrations, Helm chart, docker-compose, and dev scripts. All Go files compile-ready with real implementations, not stubs.',
    tags: ['scaffold', 'monorepo', 'go'],
    files: ['cmd/', 'internal/', 'pkg/', 'proto/', 'migrations/', 'deployments/'],
  },
  {
    type: 'work_summary',
    summary: 'Set up complete Cursor AI development framework: 8 rules (delegation, engineering standards, architect, go-engineer, security, performance, requirements, QA), 11 agent definitions, 6 skills, 5 commands, and MCP config.',
    tags: ['cursor', 'agents', 'workflow', 'configuration'],
    layer: 'cursor_config',
    files: ['.cursor/'],
  },
  {
    type: 'work_summary',
    summary: 'Implemented kernel package with actor registry, spawn/send/stop operations, and Prometheus metrics integration. Kernel manages actors via sync.RWMutex-protected map with O(1) lookup.',
    tags: ['kernel', 'actors', 'implementation'],
    layer: 'kernel',
    files: ['internal/kernel/kernel.go'],
  },
  {
    type: 'work_summary',
    summary: 'Implemented Redis Streams messaging bus with consumer group support, XAdd publish, XReadGroup consume with automatic ack, and graceful connection pooling (50 connections, 2s dial timeout).',
    tags: ['messaging', 'redis', 'streams'],
    layer: 'kernel',
    files: ['internal/messaging/bus.go'],
  },
  {
    type: 'work_summary',
    summary: 'Created 9 ordered SQL migration files: extensions (uuid-ossp, pgvector), agents/goals/tasks tables, task_dependencies, memories with VECTOR(1536), artifacts, workers, events (BIGSERIAL), indexes including ivfflat on embeddings, and constraints with update_updated_at triggers.',
    tags: ['database', 'migrations', 'postgres', 'pgvector'],
    layer: 'database',
    files: ['migrations/'],
  },
  // Patterns (15-20)
  {
    type: 'pattern',
    summary: 'All API reads must respond within 10ms (P99). Achieved via cache-first reads (Redis/Memcached → Postgres), connection pooling (25 max open, 10 idle), and prepared statements. Median scheduling latency target is 50ms.',
    tags: ['performance', 'sla', 'caching'],
    layer: 'kernel',
    files: ['pkg/db/db.go', '.cursor/rules/PERFORMANCE-RULE.mdc'],
  },
  {
    type: 'pattern',
    summary: 'Go error handling pattern: always wrap errors with fmt.Errorf(\'pkg.Func: %w\', err) for stack traces. Use structured logging with slog (JSON handler). Pass context.Context as first parameter everywhere.',
    tags: ['go', 'error-handling', 'logging', 'patterns'],
    layer: 'kernel',
    files: ['pkg/logger/logger.go'],
  },
  {
    type: 'pattern',
    summary: 'gRPC service pattern: kernel.proto defines 5 RPCs (SpawnActor, SendMessage, QueryState, SubscribeStream, PublishEvent). task.proto defines 6 RPCs (CreateTask, ScheduleTask, CompleteTask, FailTask, GetTask, GetGraph). All use proto3 with go_package options.',
    tags: ['grpc', 'proto', 'api'],
    layer: 'kernel',
    files: ['proto/kernel.proto', 'proto/task.proto'],
  },
  {
    type: 'pattern',
    summary: 'Commit messages use natural sentences (no conventional prefixes like fix:, feat:, chore:). Example: \'scaffold full monorepo structure and remove source directory\'. Multi-line body describes what was done and why.',
    tags: ['git', 'conventions', 'workflow'],
  },
  {
    type: 'pattern',
    summary: 'Monorepo layout: cmd/ for service entrypoints, internal/ for private packages (kernel layer + service layer), pkg/ for shared stable libraries, proto/ for gRPC definitions, migrations/ for ordered SQL files.',
    tags: ['go', 'project-structure', 'monorepo'],
    files: ['go.mod'],
  },
  {
    type: 'pattern',
    summary: 'Supervisor pattern: RestartPolicy enum (Immediate, Backoff, Escalate, Terminate) with circuit breaker (maxRestarts within time window). Supervisor watches child actors and handles failures.',
    tags: ['actors', 'supervision', 'fault-tolerance'],
    layer: 'kernel',
    files: ['internal/actors/supervisor.go'],
  },
  // Infrastructure (21-22)
  {
    type: 'work_summary',
    summary: 'Docker Compose provides local dev infrastructure: pgvector/pgvector:pg17 for Postgres with vector support, Redis 7 Alpine with AOF persistence, Memcached 1.6, MinIO for S3-compatible object storage.',
    tags: ['docker', 'infrastructure', 'local-dev'],
    layer: 'infrastructure',
    files: ['docker-compose.yml'],
  },
  {
    type: 'work_summary',
    summary: 'Helm chart created with deployment template (liveness/readiness probes on /health), service template (gRPC + HTTP ports), and values.yaml with resource limits, autoscaling config, and infrastructure connection settings.',
    tags: ['helm', 'kubernetes', 'deployment'],
    layer: 'infrastructure',
    files: ['deployments/helm/astra/'],
  },
];

async function main() {
  process.stderr.write('Bootstrap agent memory with curated Astra memories\n');

  const ollamaOk = await healthCheck();
  if (!ollamaOk) {
    process.stderr.write('ERROR: Ollama is not running or nomic-embed-text is not available.\n');
    process.stderr.write('Run: ollama pull nomic-embed-text\n');
    process.exit(1);
  }

  await ensureConnected();

  let stored = 0;
  for (const mem of SEED_MEMORIES) {
    const now = new Date();
    const entry: MemoryEntry = {
      id: uuidv4(),
      type: mem.type,
      project: PROJECT,
      summary: mem.summary,
      detail: mem.detail,
      tags: mem.tags,
      files: mem.files,
      layer: mem.layer,
      createdAt: now.toISOString(),
      expiresAt: computeExpiresAt(mem.type, now),
    };

    try {
      const embedding = await embed(entry.summary);
      await storeMemory(entry, embedding);
      stored++;
      process.stderr.write(`  [${mem.type}] ${entry.summary.slice(0, 80)}...\n`);
    } catch (err) {
      process.stderr.write(
        `  WARN: Failed to store memory: ${err instanceof Error ? err.message : String(err)}\n`
      );
    }
  }

  const stats = await getMemoryStats();
  process.stderr.write('\nBootstrap complete.\n');
  process.stderr.write(`Total memories stored this run: ${stored}\n`);
  process.stderr.write(`Total memories in store: ${stats.totalEntries}\n`);
  process.stderr.write(`By type: ${JSON.stringify(stats.byType)}\n`);

  await shutdown();
}

main().catch((err) => {
  process.stderr.write(`Bootstrap failed: ${err}\n`);
  process.exit(1);
});
