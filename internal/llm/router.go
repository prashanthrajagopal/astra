package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bradfitz/gomemcache/memcache"

	"astra/pkg/metrics"
)

// ModelTier is the routing tier for model selection.
type ModelTier string

const (
	TierLocal   ModelTier = "local"
	TierPremium ModelTier = "premium"
	TierCode    ModelTier = "code"
)

// Fallback model constants used when no DB store is available (e.g. tests).
const (
	ModelLocal   = "ollama/llama3:8b"
	ModelPremium = "openai/gpt-4o-mini"
	ModelCode    = "openai/gpt-4.1-mini"
)

// Usage holds token and cost metadata for a completion.
type Usage struct {
	TokensIn    int
	TokensOut   int
	Model       string
	LatencyMs   int64
	CostDollars float64
}

// CompletionOptions are optional completion parameters.
type CompletionOptions struct {
	ModelHint string // optional; if empty, default "local" is used
	MaxTokens int    // optional; 0 means backend default
}

// Router is the interface for model routing and completion.
type Router interface {
	Route(taskType string, priority int) ModelTier
	Complete(ctx context.Context, modelHint string, prompt string, options *CompletionOptions) (response string, usage Usage, err error)
}

// LLMBackend performs the actual LLM completion. Implementations may call external APIs.
type LLMBackend interface {
	Complete(ctx context.Context, model string, prompt string) (response string, tokensIn int, tokensOut int, err error)
}

// routerImpl implements Router with optional Memcached cache.
type routerImpl struct {
	backend LLMBackend
	store   *ConfigStore
	mc      *memcache.Client
	ttl     int32
}

// NewRouter returns a Router with StubBackend and no cache (for tests or when cache is not configured).
func NewRouter() *routerImpl {
	return &routerImpl{
		backend: &StubBackend{},
		mc:      nil,
		ttl:     0,
	}
}

// NewRouterWithCache returns a Router that uses the given backend and caches responses in Memcached.
// store is used for DB-driven model resolution; pass nil to use fallback constants (e.g. in tests).
func NewRouterWithCache(backend LLMBackend, store *ConfigStore, mc *memcache.Client, ttlSeconds int) *routerImpl {
	return &routerImpl{
		backend: backend,
		store:   store,
		mc:      mc,
		ttl:     int32(ttlSeconds),
	}
}

// Route selects a model tier by task type and priority.
func (r *routerImpl) Route(taskType string, priority int) ModelTier {
	switch {
	case taskType == "classification":
		return TierLocal
	case taskType == "code_generation":
		return TierCode
	case priority < 50:
		return TierPremium
	default:
		return TierLocal
	}
}

// cachedResponse is the value stored in memcache: response text plus token counts so cache hits return real usage.
type cachedResponse struct {
	R   string `json:"r"`
	In  int    `json:"in"`
	Out int    `json:"out"`
}

// Complete returns a completion for the prompt, using cache when available.
func (r *routerImpl) Complete(ctx context.Context, modelHint string, prompt string, options *CompletionOptions) (string, Usage, error) {
	model := r.resolveModel(modelHint)
	key := cacheKey(model, prompt)

	if r.mc != nil {
		item, err := r.mc.Get(key)
		if err == nil {
			var cached cachedResponse
			if jsonErr := json.Unmarshal(item.Value, &cached); jsonErr == nil {
				return cached.R, Usage{TokensIn: cached.In, TokensOut: cached.Out, Model: model, LatencyMs: 0}, nil
			}
			return string(item.Value), Usage{Model: model, LatencyMs: 0}, nil
		}
		if err != memcache.ErrCacheMiss {
			return "", Usage{}, fmt.Errorf("llm: cache get: %w", err)
		}
	}

	start := time.Now()
	resp, tokensIn, tokensOut, err := r.backend.Complete(ctx, model, prompt)
	if err != nil {
		return "", Usage{}, fmt.Errorf("llm: backend: %w", err)
	}
	latencyMs := time.Since(start).Milliseconds()
	usage := Usage{
		TokensIn:  tokensIn,
		TokensOut: tokensOut,
		Model:     model,
		LatencyMs: latencyMs,
	}

	metrics.LLMTokenUsageTotal.WithLabelValues(model, "in").Add(float64(tokensIn))
	metrics.LLMTokenUsageTotal.WithLabelValues(model, "out").Add(float64(tokensOut))
	if usage.CostDollars > 0 {
		metrics.LLMCostDollars.WithLabelValues(model).Add(usage.CostDollars)
	}

	if r.mc != nil && r.ttl > 0 {
		payload, _ := json.Marshal(cachedResponse{R: resp, In: tokensIn, Out: tokensOut})
		if err := r.mc.Set(&memcache.Item{Key: key, Value: payload, Expiration: r.ttl}); err != nil {
			_ = err
		}
	}

	return resp, usage, nil
}

// resolveModel maps a hint (tier name or provider/model string) to a concrete "provider/model" string.
// When a ConfigStore is present, provider configs and the default are read from the DB.
func (r *routerImpl) resolveModel(hint string) string {
	if hint == "" {
		hint = "local"
	}
	if strings.Contains(hint, "/") {
		return hint
	}
	switch hint {
	case "local", "ollama", "mlx":
		if r.store != nil {
			if def, ok := r.store.GetDefault(); ok && def.Enabled {
				return def.Provider + "/" + def.ModelName
			}
		}
		return ModelLocal
	case "premium":
		if r.store != nil {
			for _, p := range []string{"openai", "anthropic"} {
				if cfg, ok := r.store.Get(p); ok && cfg.Enabled {
					return p + "/" + cfg.ModelName
				}
			}
		}
		return ModelPremium
	case "code":
		if r.store != nil {
			for _, p := range []string{"openai", "anthropic"} {
				if cfg, ok := r.store.Get(p); ok && cfg.Enabled {
					return p + "/" + cfg.ModelName
				}
			}
		}
		return ModelCode
	case "claude", "anthropic":
		if r.store != nil {
			if cfg, ok := r.store.Get("anthropic"); ok && cfg.Enabled {
				return "anthropic/" + cfg.ModelName
			}
		}
		return "anthropic/claude-3-5-sonnet-latest"
	case "openai":
		if r.store != nil {
			if cfg, ok := r.store.Get("openai"); ok && cfg.Enabled {
				return "openai/" + cfg.ModelName
			}
		}
		return ModelPremium
	case "gemini", "google":
		if r.store != nil {
			if cfg, ok := r.store.Get("gemini"); ok && cfg.Enabled {
				return "gemini/" + cfg.ModelName
			}
		}
		return "gemini/gemini-1.5-pro"
	default:
		return hint
	}
}

func cacheKey(model string, prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return "llm:resp:" + model + ":" + hex.EncodeToString(h[:])
}

// StubBackend returns a fixed response and fake token counts for tests or when no real API is configured.
type StubBackend struct {
	Response  string
	TokensIn  int
	TokensOut int
}

func (s *StubBackend) Complete(ctx context.Context, model string, prompt string) (string, int, int, error) {
	resp := s.Response
	if resp == "" {
		resp = "stub completion"
	}
	tin := s.TokensIn
	if tin == 0 {
		tin = 10
	}
	tout := s.TokensOut
	if tout == 0 {
		tout = 20
	}
	return resp, tin, tout, nil
}
