"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.getRedis = getRedis;
exports.ensureConnected = ensureConnected;
exports.computeExpiresAt = computeExpiresAt;
exports.storeMemory = storeMemory;
exports.queryMemory = queryMemory;
exports.deleteExpiredMemories = deleteExpiredMemories;
exports.getMemoryStats = getMemoryStats;
exports.shutdown = shutdown;
const ioredis_1 = __importDefault(require("ioredis"));
const REDIS_URL = process.env.AGENT_MEMORY_REDIS_URL || 'redis://localhost:6379/1';
const VECTOR_KEY = 'AGENT_MEMORY:vectors';
const META_PREFIX = 'AGENT_MEMORY:meta:';
const VECTOR_DIM = 768;
let client = null;
function getRedis() {
    if (!client) {
        client = new ioredis_1.default(REDIS_URL, { lazyConnect: true, maxRetriesPerRequest: 3 });
    }
    return client;
}
async function ensureConnected() {
    const redis = getRedis();
    if (redis.status === 'wait') {
        await redis.connect();
    }
}
const TTL_DAYS = {
    decision: 365,
    pattern: 365,
    work_summary: 180,
    investigation: 90,
    error_fix: 90,
};
function computeExpiresAt(type, createdAt) {
    const expires = new Date(createdAt);
    expires.setDate(expires.getDate() + TTL_DAYS[type]);
    return expires.toISOString();
}
async function storeMemory(entry, embedding) {
    const redis = getRedis();
    const metaKey = `${META_PREFIX}${entry.id}`;
    const metaPayload = {
        id: entry.id,
        type: entry.type,
        project: entry.project,
        summary: entry.summary,
        tags: entry.tags.join(','),
        createdAt: entry.createdAt,
        expiresAt: entry.expiresAt,
    };
    if (entry.detail)
        metaPayload.detail = entry.detail;
    if (entry.files?.length)
        metaPayload.files = entry.files.join(',');
    if (entry.layer)
        metaPayload.layer = entry.layer;
    if (entry.sourceTranscriptId)
        metaPayload.sourceTranscriptId = entry.sourceTranscriptId;
    const pipeline = redis.pipeline();
    pipeline.hset(metaKey, metaPayload);
    const ttlSeconds = Math.floor((new Date(entry.expiresAt).getTime() - Date.now()) / 1000);
    if (ttlSeconds > 0) {
        pipeline.expire(metaKey, ttlSeconds);
    }
    await pipeline.exec();
    const floatArgs = embedding.map(String);
    await redis.call('VADD', VECTOR_KEY, 'VALUES', String(VECTOR_DIM), ...floatArgs, entry.id);
}
async function queryMemory(embedding, topK = 5, filters) {
    const redis = getRedis();
    const floatArgs = embedding.map(String);
    const rawResults = await redis.call('VSIM', VECTOR_KEY, 'VALUES', String(VECTOR_DIM), ...floatArgs, 'COUNT', String(topK * 3));
    if (!rawResults || rawResults.length === 0)
        return [];
    const entries = [];
    for (const id of rawResults) {
        if (entries.length >= topK)
            break;
        const metaKey = `${META_PREFIX}${id}`;
        const meta = await redis.hgetall(metaKey);
        if (!meta || !meta.id)
            continue;
        if (filters?.type && meta.type !== filters.type)
            continue;
        if (filters?.project && meta.project !== filters.project)
            continue;
        if (filters?.layer && meta.layer !== filters.layer)
            continue;
        const expired = meta.expiresAt && new Date(meta.expiresAt).getTime() < Date.now();
        if (expired)
            continue;
        entries.push({
            id: meta.id,
            type: meta.type,
            project: meta.project,
            summary: meta.summary,
            detail: meta.detail || undefined,
            tags: meta.tags ? meta.tags.split(',') : [],
            files: meta.files ? meta.files.split(',') : undefined,
            layer: meta.layer || undefined,
            createdAt: meta.createdAt,
            expiresAt: meta.expiresAt,
            sourceTranscriptId: meta.sourceTranscriptId || undefined,
        });
    }
    return entries;
}
async function deleteExpiredMemories() {
    const redis = getRedis();
    const now = Date.now();
    const keys = await redis.keys(`${META_PREFIX}*`);
    let deleted = 0;
    for (const key of keys) {
        const expiresAt = await redis.hget(key, 'expiresAt');
        if (expiresAt && new Date(expiresAt).getTime() < now) {
            const id = await redis.hget(key, 'id');
            await redis.del(key);
            if (id) {
                await redis.call('VREM', VECTOR_KEY, id).catch(() => { });
            }
            deleted++;
        }
    }
    return deleted;
}
async function getMemoryStats() {
    const redis = getRedis();
    const keys = await redis.keys(`${META_PREFIX}*`);
    const byType = {};
    for (const key of keys) {
        const type = await redis.hget(key, 'type');
        if (type) {
            byType[type] = (byType[type] || 0) + 1;
        }
    }
    return { totalEntries: keys.length, byType };
}
async function shutdown() {
    if (client) {
        await client.quit();
        client = null;
    }
}
//# sourceMappingURL=redis.js.map