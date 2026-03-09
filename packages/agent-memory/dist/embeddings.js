"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.embed = embed;
exports.healthCheck = healthCheck;
const http_1 = __importDefault(require("http"));
const OLLAMA_HOST = process.env.OLLAMA_HOST || 'http://localhost:11434';
const EMBED_MODEL = process.env.AGENT_MEMORY_EMBED_MODEL || 'nomic-embed-text';
async function embed(text) {
    const url = new URL('/api/embed', OLLAMA_HOST);
    return new Promise((resolve, reject) => {
        const body = JSON.stringify({ model: EMBED_MODEL, input: text });
        const req = http_1.default.request({
            hostname: url.hostname,
            port: url.port,
            path: url.pathname,
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Content-Length': Buffer.byteLength(body),
            },
            timeout: 30_000,
        }, (res) => {
            let data = '';
            res.on('data', (chunk) => (data += chunk));
            res.on('end', () => {
                try {
                    if (res.statusCode !== 200) {
                        reject(new Error(`Ollama returned ${res.statusCode}: ${data}`));
                        return;
                    }
                    const parsed = JSON.parse(data);
                    if (!parsed.embeddings?.[0]?.length) {
                        reject(new Error('Empty embedding returned from Ollama'));
                        return;
                    }
                    resolve(parsed.embeddings[0]);
                }
                catch (err) {
                    reject(new Error(`Failed to parse Ollama response: ${err}`));
                }
            });
        });
        req.on('error', (err) => reject(new Error(`Ollama connection failed: ${err.message}`)));
        req.on('timeout', () => {
            req.destroy();
            reject(new Error('Ollama embedding request timed out (30s)'));
        });
        req.write(body);
        req.end();
    });
}
async function healthCheck() {
    const url = new URL('/api/tags', OLLAMA_HOST);
    return new Promise((resolve) => {
        const req = http_1.default.request({ hostname: url.hostname, port: url.port, path: url.pathname, method: 'GET', timeout: 5_000 }, (res) => {
            let data = '';
            res.on('data', (chunk) => (data += chunk));
            res.on('end', () => {
                try {
                    const parsed = JSON.parse(data);
                    const hasModel = parsed.models?.some((m) => m.name.startsWith(EMBED_MODEL));
                    resolve(hasModel === true);
                }
                catch {
                    resolve(false);
                }
            });
        });
        req.on('error', () => resolve(false));
        req.on('timeout', () => { req.destroy(); resolve(false); });
        req.end();
    });
}
//# sourceMappingURL=embeddings.js.map