package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
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

// Resolved model names for cache keys and usage (constants; config can override later).
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
// ttlSeconds is the cache TTL (e.g. 86400 for 24h).
func NewRouterWithCache(backend LLMBackend, mc *memcache.Client, ttlSeconds int) *routerImpl {
	return &routerImpl{
		backend: backend,
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
// Cache key: llm:resp:{model}:{sha256(prompt)}. On cache hit, usage includes cached TokensIn/TokensOut (LatencyMs=0).
func (r *routerImpl) Complete(ctx context.Context, modelHint string, prompt string, options *CompletionOptions) (string, Usage, error) {
	model := resolveModel(modelHint)
	key := cacheKey(model, prompt)

	if r.mc != nil {
		item, err := r.mc.Get(key)
		if err == nil {
			var cached cachedResponse
			if jsonErr := json.Unmarshal(item.Value, &cached); jsonErr == nil {
				return cached.R, Usage{TokensIn: cached.In, TokensOut: cached.Out, Model: model, LatencyMs: 0}, nil
			}
			// backward compat: value may be raw response (no usage)
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
			// non-fatal: response still valid
			_ = err
		}
	}

	return resp, usage, nil
}

// resolveModel maps modelHint (or tier name) to a concrete model name for cache and backend.
func resolveModel(modelHint string) string {
	if modelHint == "" {
		modelHint = "local"
	}
	switch modelHint {
	case "local":
		if strings.ToLower(os.Getenv("LLM_DEFAULT_PROVIDER")) == "mlx" {
			mlxModel := os.Getenv("MLX_MODEL")
			if mlxModel == "" {
				mlxModel = "Qwen2.5-7B-Instruct-4bit"
			}
			return "mlx/" + mlxModel
		}
		return ModelLocal
	case "mlx":
		mlxModel := os.Getenv("MLX_MODEL")
		if mlxModel == "" {
			mlxModel = "Qwen2.5-7B-Instruct-4bit"
		}
		return "mlx/" + mlxModel
	case "premium":
		return ModelPremium
	case "code":
		return ModelCode
	case "openai":
		return ModelPremium
	case "claude", "anthropic":
		return "anthropic/claude-3-5-sonnet-latest"
	case "gemini", "google":
		return "gemini/gemini-1.5-pro"
	case "ollama":
		return ModelLocal
	default:
		if strings.Contains(modelHint, "/") {
			return modelHint
		}
		return modelHint
	}
}

func cacheKey(model string, prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return "llm:resp:" + model + ":" + hex.EncodeToString(h[:])
}

// StubBackend returns a fixed response and fake token counts for tests or when no real API is configured.
type StubBackend struct {
	Response  string // default "stub completion"
	TokensIn  int    // default 10
	TokensOut int    // default 20
}

// Complete implements LLMBackend.
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
