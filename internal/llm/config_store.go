package llm

import (
	"context"
	"database/sql"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// ProviderConfig represents one row from llm_provider_config.
type ProviderConfig struct {
	Provider  string `json:"provider"`
	APIKey    string `json:"api_key"`
	HostURL   string `json:"host_url"`
	ModelName string `json:"model_name"`
	Enabled   bool   `json:"enabled"`
	IsDefault bool   `json:"is_default"`
	MaxTokens int    `json:"max_tokens"`
	UpdatedAt string `json:"updated_at"`
}

// ConfigStore reads LLM provider config from the database and caches it in memory.
type ConfigStore struct {
	db      *sql.DB
	mu      sync.RWMutex
	configs map[string]ProviderConfig
}

// NewConfigStore creates a new ConfigStore. Call Load() before using.
func NewConfigStore(db *sql.DB) *ConfigStore {
	return &ConfigStore{
		db:      db,
		configs: make(map[string]ProviderConfig),
	}
}

// Load reads all rows from llm_provider_config into the in-memory cache.
func (s *ConfigStore) Load(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT provider, api_key, host_url, model_name, enabled, is_default, max_tokens, updated_at::text
		 FROM llm_provider_config ORDER BY provider`)
	if err != nil {
		return err
	}
	defer rows.Close()

	m := make(map[string]ProviderConfig)
	for rows.Next() {
		var c ProviderConfig
		if err := rows.Scan(&c.Provider, &c.APIKey, &c.HostURL, &c.ModelName,
			&c.Enabled, &c.IsDefault, &c.MaxTokens, &c.UpdatedAt); err != nil {
			return err
		}
		m[c.Provider] = c
	}
	if err := rows.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	s.configs = m
	s.mu.Unlock()
	return nil
}

// StartRefresh launches a goroutine that calls Load every 30 seconds until ctx is cancelled.
func (s *ConfigStore) StartRefresh(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.Load(ctx); err != nil {
					slog.Warn("llm config refresh failed", "err", err)
				}
			}
		}
	}()
}

// Get returns the config for a single provider.
func (s *ConfigStore) Get(provider string) (ProviderConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.configs[provider]
	return c, ok
}

// GetAll returns all provider configs sorted by provider name.
func (s *ConfigStore) GetAll() []ProviderConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ProviderConfig, 0, len(s.configs))
	for _, c := range s.configs {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out
}

// GetDefault returns the config where is_default=true.
func (s *ConfigStore) GetDefault() (ProviderConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.configs {
		if c.IsDefault {
			return c, true
		}
	}
	return ProviderConfig{}, false
}

// Upsert writes a single provider config row.
func (s *ConfigStore) Upsert(ctx context.Context, cfg ProviderConfig) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO llm_provider_config (provider, api_key, host_url, model_name, enabled, is_default, max_tokens, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		 ON CONFLICT (provider) DO UPDATE SET
		   api_key     = EXCLUDED.api_key,
		   host_url    = EXCLUDED.host_url,
		   model_name  = EXCLUDED.model_name,
		   enabled     = EXCLUDED.enabled,
		   is_default  = EXCLUDED.is_default,
		   max_tokens  = EXCLUDED.max_tokens,
		   updated_at  = now()`,
		cfg.Provider, cfg.APIKey, cfg.HostURL, cfg.ModelName,
		cfg.Enabled, cfg.IsDefault, cfg.MaxTokens)
	return err
}

// SetDefault sets is_default=true for the given provider and false for all others.
func (s *ConfigStore) SetDefault(ctx context.Context, provider string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`UPDATE llm_provider_config SET is_default=false, updated_at=now() WHERE is_default=true`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE llm_provider_config SET is_default=true, updated_at=now() WHERE provider=$1`, provider); err != nil {
		return err
	}
	return tx.Commit()
}
