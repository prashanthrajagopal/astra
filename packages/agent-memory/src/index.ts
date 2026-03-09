import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { z } from 'zod';
import { v4 as uuidv4 } from 'uuid';
import {
  ensureConnected,
  storeMemory,
  queryMemory,
  deleteExpiredMemories,
  getMemoryStats,
  computeExpiresAt,
  shutdown,
  type MemoryEntry,
} from './redis';
import { embed, healthCheck } from './embeddings';

const PROJECT = process.env.AGENT_MEMORY_PROJECT || 'astra-kernel';

const ASTRA_LAYER_OPTIONS = [
  'kernel',
  'services',
  'infrastructure',
  'database',
  'deployment',
  'cursor_config',
  'documentation',
] as const;

const server = new McpServer({
  name: 'agent-memory',
  version: '1.0.0',
});

const queryMemorySchema = {
  query: z.string().describe('Natural language description of what you are looking for'),
  topK: z.number().min(1).max(20).default(5).describe('Number of results to return'),
  type: z.enum(['investigation', 'decision', 'pattern', 'error_fix', 'work_summary']).optional()
    .describe('Filter by memory type'),
  layer: z.enum(ASTRA_LAYER_OPTIONS).optional()
    .describe('Filter by Astra layer: kernel, services, infrastructure, database, deployment, cursor_config, documentation'),
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
(server as any).registerTool(
  'query_memory',
  {
    description: 'Search agent memory for relevant past work, investigations, decisions, patterns, and fixes. Call this BEFORE starting a task to check if similar work was done before.',
    inputSchema: queryMemorySchema,
  },
  async (args: { query: string; topK?: number; type?: string; layer?: string }) => {
    const { query, topK, type, layer } = args;
    try {
      await ensureConnected();
      const queryEmbedding = await embed(query);
      const results = await queryMemory(queryEmbedding, topK ?? 5, {
        type,
        project: PROJECT,
        layer,
      });

      if (results.length === 0) {
        return {
          content: [{ type: 'text' as const, text: 'No relevant memories found.' }],
        };
      }

      const formatted = results.map((m, i) => {
        const parts = [
          `### ${i + 1}. [${m.type}] ${m.summary}`,
          m.detail ? `**Detail:** ${m.detail}` : '',
          m.tags.length ? `**Tags:** ${m.tags.join(', ')}` : '',
          m.files?.length ? `**Files:** ${m.files.join(', ')}` : '',
          m.layer ? `**Layer:** ${m.layer}` : '',
          `**Created:** ${m.createdAt}`,
        ];
        return parts.filter(Boolean).join('\n');
      });

      return {
        content: [{
          type: 'text' as const,
          text: `Found ${results.length} relevant memories:\n\n${formatted.join('\n\n---\n\n')}`,
        }],
      };
    } catch (err) {
      return {
        content: [{
          type: 'text' as const,
          text: `Memory query failed (non-blocking): ${err instanceof Error ? err.message : String(err)}`,
        }],
        isError: true,
      };
    }
  }
);

const storeMemorySchema = {
  type: z.enum(['investigation', 'decision', 'pattern', 'error_fix', 'work_summary'])
    .describe('Type of memory'),
  summary: z.string().max(500).describe('Concise 1-2 sentence summary (main searchable content)'),
  detail: z.string().max(2000).optional().describe('Optional extended context'),
  tags: z.array(z.string()).min(1).max(10).describe('Tags for filtering (e.g. finance, rss, auth)'),
  files: z.array(z.string()).optional().describe('Relevant file paths'),
  layer: z.enum(ASTRA_LAYER_OPTIONS).optional()
    .describe('Astra layer: kernel, services, infrastructure, database, deployment, cursor_config, documentation'),
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
(server as any).registerTool(
  'store_memory',
  {
    description: 'Store a memory entry after completing a task. Only leadership agents (tech-lead, sre-lead, architect) should call this. Store concise, reusable knowledge — not raw conversation.',
    inputSchema: storeMemorySchema,
  },
  async (args: {
    type: 'investigation' | 'decision' | 'pattern' | 'error_fix' | 'work_summary';
    summary: string;
    detail?: string;
    tags: string[];
    files?: string[];
    layer?: string;
  }) => {
    const { type, summary, detail, tags, files, layer } = args;
    try {
      await ensureConnected();
      const now = new Date();
      const entry: MemoryEntry = {
        id: uuidv4(),
        type,
        project: PROJECT,
        summary,
        detail,
        tags,
        files,
        layer,
        createdAt: now.toISOString(),
        expiresAt: computeExpiresAt(type, now),
      };

      const embedding = await embed(summary);
      await storeMemory(entry, embedding);

      return {
        content: [{
          type: 'text' as const,
          text: `Memory stored (id: ${entry.id}, type: ${type}, expires: ${entry.expiresAt})`,
        }],
      };
    } catch (err) {
      return {
        content: [{
          type: 'text' as const,
          text: `Memory store failed (non-blocking): ${err instanceof Error ? err.message : String(err)}`,
        }],
        isError: true,
      };
    }
  }
);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
(server as any).registerTool(
  'memory_stats',
  {
    description: 'Get statistics about the agent memory store. Useful for monitoring memory health.',
    inputSchema: {},
  },
  async () => {
    try {
      await ensureConnected();
      const stats = await getMemoryStats();
      const ollamaOk = await healthCheck();

      return {
        content: [{
          type: 'text' as const,
          text: [
            `**Agent Memory Stats**`,
            `Total entries: ${stats.totalEntries}`,
            `By type: ${JSON.stringify(stats.byType, null, 2)}`,
            `Ollama healthy: ${ollamaOk}`,
          ].join('\n'),
        }],
      };
    } catch (err) {
      return {
        content: [{
          type: 'text' as const,
          text: `Stats retrieval failed: ${err instanceof Error ? err.message : String(err)}`,
        }],
        isError: true,
      };
    }
  }
);

// eslint-disable-next-line @typescript-eslint/no-explicit-any
(server as any).registerTool(
  'cleanup_memory',
  {
    description: 'Delete expired memory entries. Run periodically to keep the memory store lean.',
    inputSchema: {},
  },
  async () => {
    try {
      await ensureConnected();
      const deleted = await deleteExpiredMemories();
      return {
        content: [{
          type: 'text' as const,
          text: `Cleanup complete. Deleted ${deleted} expired entries.`,
        }],
      };
    } catch (err) {
      return {
        content: [{
          type: 'text' as const,
          text: `Cleanup failed: ${err instanceof Error ? err.message : String(err)}`,
        }],
        isError: true,
      };
    }
  }
);

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  process.stderr.write('agent-memory MCP server started (stdio)\n');

  const graceful = async () => {
    process.stderr.write('agent-memory shutting down...\n');
    await shutdown();
    process.exit(0);
  };
  process.on('SIGINT', graceful);
  process.on('SIGTERM', graceful);
}

main().catch((err) => {
  process.stderr.write(`agent-memory failed to start: ${err}\n`);
  process.exit(1);
});
