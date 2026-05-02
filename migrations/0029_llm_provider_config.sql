-- LLM provider configuration (platform-level, one row per provider).
CREATE TABLE IF NOT EXISTS llm_provider_config (
    provider    TEXT PRIMARY KEY,
    api_key     TEXT NOT NULL DEFAULT '',
    host_url    TEXT NOT NULL DEFAULT '',
    model_name  TEXT NOT NULL DEFAULT '',
    enabled     BOOLEAN NOT NULL DEFAULT false,
    is_default  BOOLEAN NOT NULL DEFAULT false,
    max_tokens  INT NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO llm_provider_config (provider, host_url, model_name, enabled, is_default)
VALUES
    ('openai',    'https://api.openai.com/v1', 'gpt-4o-mini', false, false),
    ('anthropic', 'https://api.anthropic.com/v1', 'claude-3-5-sonnet-latest', false, false),
    ('gemini',    'https://generativelanguage.googleapis.com/v1beta', 'gemini-1.5-pro', false, false),
    ('ollama',    'http://localhost:11434', 'llama3:8b', true, true),
    ('mlx',       'http://localhost:8888', 'Qwen2.5-7B-Instruct-4bit', false, false)
ON CONFLICT (provider) DO NOTHING;
