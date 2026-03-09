"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
const mcp_js_1 = require("@modelcontextprotocol/sdk/server/mcp.js");
const stdio_js_1 = require("@modelcontextprotocol/sdk/server/stdio.js");
const zod_1 = require("zod");
const uuid_1 = require("uuid");
const redis_1 = require("./redis");
const embeddings_1 = require("./embeddings");
const PROJECT = process.env.AGENT_MEMORY_PROJECT || 'astra-kernel';
const ASTRA_LAYER_OPTIONS = [
    'kernel',
    'services',
    'infrastructure',
    'database',
    'deployment',
    'cursor_config',
    'documentation',
];
const server = new mcp_js_1.McpServer({
    name: 'agent-memory',
    version: '1.0.0',
});
const queryMemorySchema = {
    query: zod_1.z.string().describe('Natural language description of what you are looking for'),
    topK: zod_1.z.number().min(1).max(20).default(5).describe('Number of results to return'),
    type: zod_1.z.enum(['investigation', 'decision', 'pattern', 'error_fix', 'work_summary']).optional()
        .describe('Filter by memory type'),
    layer: zod_1.z.enum(ASTRA_LAYER_OPTIONS).optional()
        .describe('Filter by Astra layer: kernel, services, infrastructure, database, deployment, cursor_config, documentation'),
};
// eslint-disable-next-line @typescript-eslint/no-explicit-any
server.registerTool('query_memory', {
    description: 'Search agent memory for relevant past work, investigations, decisions, patterns, and fixes. Call this BEFORE starting a task to check if similar work was done before.',
    inputSchema: queryMemorySchema,
}, async (args) => {
    const { query, topK, type, layer } = args;
    try {
        await (0, redis_1.ensureConnected)();
        const queryEmbedding = await (0, embeddings_1.embed)(query);
        const results = await (0, redis_1.queryMemory)(queryEmbedding, topK ?? 5, {
            type,
            project: PROJECT,
            layer,
        });
        if (results.length === 0) {
            return {
                content: [{ type: 'text', text: 'No relevant memories found.' }],
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
                    type: 'text',
                    text: `Found ${results.length} relevant memories:\n\n${formatted.join('\n\n---\n\n')}`,
                }],
        };
    }
    catch (err) {
        return {
            content: [{
                    type: 'text',
                    text: `Memory query failed (non-blocking): ${err instanceof Error ? err.message : String(err)}`,
                }],
            isError: true,
        };
    }
});
const storeMemorySchema = {
    type: zod_1.z.enum(['investigation', 'decision', 'pattern', 'error_fix', 'work_summary'])
        .describe('Type of memory'),
    summary: zod_1.z.string().max(500).describe('Concise 1-2 sentence summary (main searchable content)'),
    detail: zod_1.z.string().max(2000).optional().describe('Optional extended context'),
    tags: zod_1.z.array(zod_1.z.string()).min(1).max(10).describe('Tags for filtering (e.g. finance, rss, auth)'),
    files: zod_1.z.array(zod_1.z.string()).optional().describe('Relevant file paths'),
    layer: zod_1.z.enum(ASTRA_LAYER_OPTIONS).optional()
        .describe('Astra layer: kernel, services, infrastructure, database, deployment, cursor_config, documentation'),
};
// eslint-disable-next-line @typescript-eslint/no-explicit-any
server.registerTool('store_memory', {
    description: 'Store a memory entry after completing a task. Only leadership agents (tech-lead, sre-lead, architect) should call this. Store concise, reusable knowledge — not raw conversation.',
    inputSchema: storeMemorySchema,
}, async (args) => {
    const { type, summary, detail, tags, files, layer } = args;
    try {
        await (0, redis_1.ensureConnected)();
        const now = new Date();
        const entry = {
            id: (0, uuid_1.v4)(),
            type,
            project: PROJECT,
            summary,
            detail,
            tags,
            files,
            layer,
            createdAt: now.toISOString(),
            expiresAt: (0, redis_1.computeExpiresAt)(type, now),
        };
        const embedding = await (0, embeddings_1.embed)(summary);
        await (0, redis_1.storeMemory)(entry, embedding);
        return {
            content: [{
                    type: 'text',
                    text: `Memory stored (id: ${entry.id}, type: ${type}, expires: ${entry.expiresAt})`,
                }],
        };
    }
    catch (err) {
        return {
            content: [{
                    type: 'text',
                    text: `Memory store failed (non-blocking): ${err instanceof Error ? err.message : String(err)}`,
                }],
            isError: true,
        };
    }
});
// eslint-disable-next-line @typescript-eslint/no-explicit-any
server.registerTool('memory_stats', {
    description: 'Get statistics about the agent memory store. Useful for monitoring memory health.',
    inputSchema: {},
}, async () => {
    try {
        await (0, redis_1.ensureConnected)();
        const stats = await (0, redis_1.getMemoryStats)();
        const ollamaOk = await (0, embeddings_1.healthCheck)();
        return {
            content: [{
                    type: 'text',
                    text: [
                        `**Agent Memory Stats**`,
                        `Total entries: ${stats.totalEntries}`,
                        `By type: ${JSON.stringify(stats.byType, null, 2)}`,
                        `Ollama healthy: ${ollamaOk}`,
                    ].join('\n'),
                }],
        };
    }
    catch (err) {
        return {
            content: [{
                    type: 'text',
                    text: `Stats retrieval failed: ${err instanceof Error ? err.message : String(err)}`,
                }],
            isError: true,
        };
    }
});
// eslint-disable-next-line @typescript-eslint/no-explicit-any
server.registerTool('cleanup_memory', {
    description: 'Delete expired memory entries. Run periodically to keep the memory store lean.',
    inputSchema: {},
}, async () => {
    try {
        await (0, redis_1.ensureConnected)();
        const deleted = await (0, redis_1.deleteExpiredMemories)();
        return {
            content: [{
                    type: 'text',
                    text: `Cleanup complete. Deleted ${deleted} expired entries.`,
                }],
        };
    }
    catch (err) {
        return {
            content: [{
                    type: 'text',
                    text: `Cleanup failed: ${err instanceof Error ? err.message : String(err)}`,
                }],
            isError: true,
        };
    }
});
async function main() {
    const transport = new stdio_js_1.StdioServerTransport();
    await server.connect(transport);
    process.stderr.write('agent-memory MCP server started (stdio)\n');
    const graceful = async () => {
        process.stderr.write('agent-memory shutting down...\n');
        await (0, redis_1.shutdown)();
        process.exit(0);
    };
    process.on('SIGINT', graceful);
    process.on('SIGTERM', graceful);
}
main().catch((err) => {
    process.stderr.write(`agent-memory failed to start: ${err}\n`);
    process.exit(1);
});
//# sourceMappingURL=index.js.map