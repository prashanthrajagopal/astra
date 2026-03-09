package memory

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"

	"github.com/bradfitz/gomemcache/memcache"
)

// Embedder produces vector embeddings for text. Dimension is 1536.
type Embedder interface {
	Embed(ctx context.Context, content string) ([]float32, error)
}

// StubEmbedder returns a deterministic 1536-dim vector for tests or when no real embedding API is configured.
// The vector is derived from a hash of the content so the same content yields the same vector.
type StubEmbedder struct{}

func NewStubEmbedder() *StubEmbedder {
	return &StubEmbedder{}
}

func (e *StubEmbedder) Embed(ctx context.Context, content string) ([]float32, error) {
	h := sha256.Sum256([]byte(content))
	vec := make([]float32, embeddingDim)
	for i := 0; i < embeddingDim; i++ {
		// Use bytes from hash to get deterministic values in [-1, 1)
		j := i % (sha256.Size / 4)
		u := binary.BigEndian.Uint32(h[j*4 : (j+1)*4])
		vec[i] = float32(u)/float32(^uint32(0))*2 - 1
	}
	return vec, nil
}

// CachedEmbedder wraps an Embedder and caches results in Memcached.
// Cache key: embed:{sha256(content)}. Values are 1536 float32s stored as binary (1536*4 bytes).
type CachedEmbedder struct {
	embedder Embedder
	mc       *memcache.Client
	ttl      int32
}

// NewCachedEmbedder returns a CachedEmbedder. ttlSeconds is the cache TTL (e.g. 7*24*3600 for 7 days).
func NewCachedEmbedder(embedder Embedder, mc *memcache.Client, ttlSeconds int) *CachedEmbedder {
	return &CachedEmbedder{
		embedder: embedder,
		mc:       mc,
		ttl:      int32(ttlSeconds),
	}
}

func (e *CachedEmbedder) Embed(ctx context.Context, content string) ([]float32, error) {
	key := cacheKey(content)
	item, err := e.mc.Get(key)
	if err == nil {
		vec, err := decodeEmbedding(item.Value)
		if err != nil {
			return nil, fmt.Errorf("CachedEmbedder: decode cache: %w", err)
		}
		return vec, nil
	}
	if err != memcache.ErrCacheMiss {
		return nil, fmt.Errorf("CachedEmbedder: get: %w", err)
	}

	vec, err := e.embedder.Embed(ctx, content)
	if err != nil {
		return nil, err
	}
	val := encodeEmbedding(vec)
	if err := e.mc.Set(&memcache.Item{Key: key, Value: val, Expiration: e.ttl}); err != nil {
		return nil, fmt.Errorf("CachedEmbedder: set: %w", err)
	}
	return vec, nil
}

func cacheKey(content string) string {
	h := sha256.Sum256([]byte(content))
	return "embed:" + hex.EncodeToString(h[:])
}

func encodeEmbedding(vec []float32) []byte {
	b := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(b[i*4:(i+1)*4], math.Float32bits(v))
	}
	return b
}

func decodeEmbedding(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("invalid length %d", len(b))
	}
	n := len(b) / 4
	vec := make([]float32, n)
	for i := 0; i < n; i++ {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4 : (i+1)*4]))
	}
	return vec, nil
}

// EmbeddingFromBytes decodes little-endian float32 bytes into an embedding slice.
// If len(b) is 0, returns nil, nil. If len(b) != embeddingDim*4, returns an error.
// Used by gRPC handlers to convert request embedding bytes to []float32.
func EmbeddingFromBytes(b []byte) ([]float32, error) {
	if len(b) == 0 {
		return nil, nil
	}
	if len(b) != embeddingDim*4 {
		return nil, fmt.Errorf("embedding bytes length %d, want %d", len(b), embeddingDim*4)
	}
	return decodeEmbedding(b)
}

// EmbeddingToBytes encodes an embedding slice to little-endian float32 bytes.
// Used by gRPC handlers to convert []float32 to response bytes.
func EmbeddingToBytes(vec []float32) []byte {
	return encodeEmbedding(vec)
}
