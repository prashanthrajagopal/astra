package memory

import (
	"context"
	"testing"
)

func TestStubEmbedder_Embed(t *testing.T) {
	ctx := context.Background()
	e := NewStubEmbedder()

	vec, err := e.Embed(ctx, "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != embeddingDim {
		t.Errorf("len(vec) = %d, want %d", len(vec), embeddingDim)
	}

	vec2, err := e.Embed(ctx, "hello")
	if err != nil {
		t.Fatalf("Embed second: %v", err)
	}
	for i := range vec {
		if vec[i] != vec2[i] {
			t.Errorf("deterministic: vec[%d] = %v, vec2[%d] = %v", i, vec[i], i, vec2[i])
		}
	}

	vec3, _ := e.Embed(ctx, "world")
	same := true
	for i := range vec {
		if vec[i] != vec3[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different content should yield different vector")
	}
}

func TestEncodeDecodeEmbedding(t *testing.T) {
	vec := make([]float32, embeddingDim)
	for i := range vec {
		vec[i] = float32(i) * 0.1
	}
	b := encodeEmbedding(vec)
	if len(b) != embeddingDim*4 {
		t.Errorf("encoded length = %d, want %d", len(b), embeddingDim*4)
	}
	dec, err := decodeEmbedding(b)
	if err != nil {
		t.Fatal(err)
	}
	for i := range vec {
		if dec[i] != vec[i] {
			t.Errorf("dec[%d] = %v, want %v", i, dec[i], vec[i])
		}
	}
}
