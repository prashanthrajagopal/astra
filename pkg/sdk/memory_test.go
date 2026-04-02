package sdk

import (
	"math"
	"testing"
)

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

func TestFloatsToBytes_Nil(t *testing.T) {
	if b := floatsToBytes(nil); b != nil {
		t.Errorf("floatsToBytes(nil) = %v, want nil", b)
	}
}

func TestFloatsToBytes_Empty(t *testing.T) {
	if b := floatsToBytes([]float32{}); b != nil {
		t.Errorf("floatsToBytes([]) = %v, want nil", b)
	}
}

func TestBytesToFloats_Nil(t *testing.T) {
	if f := bytesToFloats(nil); f != nil {
		t.Errorf("bytesToFloats(nil) = %v, want nil", f)
	}
}

func TestBytesToFloats_Empty(t *testing.T) {
	if f := bytesToFloats([]byte{}); f != nil {
		t.Errorf("bytesToFloats([]) = %v, want nil", f)
	}
}

func TestBytesToFloats_NonMultipleOf4(t *testing.T) {
	// 3 bytes is not a multiple of 4; should return nil
	if f := bytesToFloats([]byte{0x01, 0x02, 0x03}); f != nil {
		t.Errorf("bytesToFloats(3 bytes) = %v, want nil", f)
	}
}

func TestFloatsToBytes_Length(t *testing.T) {
	in := []float32{1.0, 2.0, 3.0}
	b := floatsToBytes(in)
	if len(b) != len(in)*4 {
		t.Errorf("expected %d bytes, got %d", len(in)*4, len(b))
	}
}

func TestFloatsToBytes_RoundTrip_Table(t *testing.T) {
	tests := []struct {
		name string
		in   []float32
	}{
		{"zero", []float32{0.0}},
		{"positive", []float32{1.0, 2.0, 3.0}},
		{"negative", []float32{-1.0, -2.5}},
		{"mixed", []float32{0.25, -1.5, 42.0, 0.0, -0.0}},
		{"large", []float32{1e30, -1e30}},
		{"small", []float32{1e-30, -1e-30}},
		{"single", []float32{3.14159}},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			b := floatsToBytes(tc.in)
			out := bytesToFloats(b)
			if len(out) != len(tc.in) {
				t.Fatalf("length mismatch: got %d want %d", len(out), len(tc.in))
			}
			for i := range tc.in {
				if out[i] != tc.in[i] {
					t.Errorf("index %d: got %v want %v", i, out[i], tc.in[i])
				}
			}
		})
	}
}

func TestFloatsToBytes_NaNInfRoundTrip(t *testing.T) {
	in := []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))}
	b := floatsToBytes(in)
	out := bytesToFloats(b)
	if len(out) != 3 {
		t.Fatalf("length: got %d", len(out))
	}
	if !math.IsNaN(float64(out[0])) {
		t.Error("expected NaN at index 0")
	}
	if !math.IsInf(float64(out[1]), 1) {
		t.Error("expected +Inf at index 1")
	}
	if !math.IsInf(float64(out[2]), -1) {
		t.Error("expected -Inf at index 2")
	}
}
