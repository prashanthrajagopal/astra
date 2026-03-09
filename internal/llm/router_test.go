package llm

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/bradfitz/gomemcache/memcache"
)

func TestComplete_StubBackend_NoCache(t *testing.T) {
	ctx := context.Background()
	r := NewRouter()
	resp, usage, err := r.Complete(ctx, "", "hello", nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp != "stub completion" {
		t.Errorf("response = %q, want stub completion", resp)
	}
	if usage.TokensIn != 10 || usage.TokensOut != 20 {
		t.Errorf("usage = %+v, want TokensIn=10 TokensOut=20", usage)
	}
	if usage.Model != ModelLocal {
		t.Errorf("usage.Model = %q, want %q", usage.Model, ModelLocal)
	}
	if usage.LatencyMs < 0 {
		t.Errorf("usage.LatencyMs = %d, want >= 0", usage.LatencyMs)
	}
}

func TestComplete_StubBackend_CustomOptions(t *testing.T) {
	ctx := context.Background()
	stub := &StubBackend{Response: "custom", TokensIn: 5, TokensOut: 15}
	r := NewRouterWithCache(stub, nil, 0)
	resp, usage, err := r.Complete(ctx, "premium", "hi", &CompletionOptions{ModelHint: "premium"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp != "custom" {
		t.Errorf("response = %q, want custom", resp)
	}
	if usage.TokensIn != 5 || usage.TokensOut != 15 {
		t.Errorf("usage = %+v, want TokensIn=5 TokensOut=15", usage)
	}
	if usage.Model != ModelPremium {
		t.Errorf("usage.Model = %q, want %q", usage.Model, ModelPremium)
	}
}

// countingBackend counts Complete() calls for cache tests.
type countingBackend struct {
	calls int32
	resp  string
	tin   int
	tout  int
}

func (c *countingBackend) Complete(ctx context.Context, model string, prompt string) (string, int, int, error) {
	atomic.AddInt32(&c.calls, 1)
	resp := c.resp
	if resp == "" {
		resp = "cached"
	}
	tin, tout := c.tin, c.tout
	if tin == 0 {
		tin = 1
	}
	if tout == 0 {
		tout = 2
	}
	return resp, tin, tout, nil
}

func TestComplete_WithCache_SecondCallDoesNotCallBackend(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cache test in short mode (requires Memcached)")
	}
	mc := memcache.New("localhost:11211")
	if err := mc.Ping(); err != nil {
		t.Skipf("Memcached not available: %v", err)
	}
	backend := &countingBackend{resp: "first"}
	r := NewRouterWithCache(backend, mc, 60)
	ctx := context.Background()
	prompt := "identical prompt for cache"

	resp1, _, err := r.Complete(ctx, "local", prompt, nil)
	if err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	if resp1 != "first" {
		t.Errorf("first response = %q, want first", resp1)
	}
	calls1 := atomic.LoadInt32(&backend.calls)
	if calls1 != 1 {
		t.Errorf("after first call, backend calls = %d, want 1", calls1)
	}

	resp2, usage2, err := r.Complete(ctx, "local", prompt, nil)
	if err != nil {
		t.Fatalf("second Complete: %v", err)
	}
	if resp2 != "first" {
		t.Errorf("second response = %q, want first (cached)", resp2)
	}
	if usage2.LatencyMs != 0 {
		t.Errorf("cached usage.LatencyMs = %d, want 0", usage2.LatencyMs)
	}
	calls2 := atomic.LoadInt32(&backend.calls)
	if calls2 != 1 {
		t.Errorf("after second call, backend calls = %d, want 1 (cache hit)", calls2)
	}
}

func TestResolveModel(t *testing.T) {
	tests := []struct {
		hint   string
		want   string
	}{
		{"", ModelLocal},
		{"local", ModelLocal},
		{"premium", ModelPremium},
		{"code", ModelCode},
		{"custom/model", "custom/model"},
	}
	for _, tt := range tests {
		got := resolveModel(tt.hint)
		if got != tt.want {
			t.Errorf("resolveModel(%q) = %q, want %q", tt.hint, got, tt.want)
		}
	}
}

func TestCacheKey(t *testing.T) {
	k1 := cacheKey("ollama/llama", "hello")
	k2 := cacheKey("ollama/llama", "hello")
	if k1 != k2 {
		t.Errorf("same model+prompt should give same key: %q vs %q", k1, k2)
	}
	if k1 != cacheKey("ollama/llama", "hello") {
		t.Error("cacheKey not deterministic")
	}
	k3 := cacheKey("ollama/llama", "world")
	if k1 == k3 {
		t.Error("different prompt should give different key")
	}
}
