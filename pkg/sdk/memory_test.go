package sdk

import "testing"

func TestEmbeddingRoundTrip(t *testing.T) {
	in := []float32{0.25, -1.5, 42.0}
	encoded := floatsToBytes(in)
	out := bytesToFloats(encoded)
	if len(out) != len(in) {
		t.Fatalf("length mismatch: got=%d want=%d", len(out), len(in))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Fatalf("value mismatch at %d: got=%v want=%v", i, out[i], in[i])
		}
	}
}
