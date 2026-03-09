import Redis from 'ioredis';
export declare function getRedis(): Redis;
export declare function ensureConnected(): Promise<void>;
export interface MemoryEntry {
    id: string;
    type: 'investigation' | 'decision' | 'pattern' | 'error_fix' | 'work_summary';
    project: string;
    summary: string;
    detail?: string;
    tags: string[];
    files?: string[];
    layer?: string;
    createdAt: string;
    expiresAt: string;
    sourceTranscriptId?: string;
}
export declare function computeExpiresAt(type: MemoryEntry['type'], createdAt: Date): string;
export declare function storeMemory(entry: MemoryEntry, embedding: number[]): Promise<void>;
export declare function queryMemory(embedding: number[], topK?: number, filters?: {
    type?: string;
    project?: string;
    layer?: string;
}): Promise<MemoryEntry[]>;
export declare function deleteExpiredMemories(): Promise<number>;
export declare function getMemoryStats(): Promise<{
    totalEntries: number;
    byType: Record<string, number>;
}>;
export declare function shutdown(): Promise<void>;
//# sourceMappingURL=redis.d.ts.map