package memory

import (
	"testing"
)

func TestFormatVector(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got := formatVector(nil)
		if got != "[]" {
			t.Errorf("formatVector(nil) = %q, want []", got)
		}
		got = formatVector([]float32{})
		if got != "[]" {
			t.Errorf("formatVector([]) = %q, want []", got)
		}
	})
	t.Run("single", func(t *testing.T) {
		got := formatVector([]float32{1.5})
		if got != "[1.5]" {
			t.Errorf("formatVector([1.5]) = %q, want [1.5]", got)
		}
	})
	t.Run("multiple", func(t *testing.T) {
		got := formatVector([]float32{0.1, -0.2, 1e-3})
		if got != "[0.1,-0.2,0.001]" {
			t.Errorf("formatVector(...) = %q", got)
		}
	})
}

func TestParseVector(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got, err := parseVector("")
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Errorf("parseVector(\"\") = %v, want nil", got)
		}
		got, _ = parseVector("[]")
		if got != nil {
			t.Errorf("parseVector(\"[]\") = %v, want nil", got)
		}
	})
	t.Run("roundtrip", func(t *testing.T) {
		vec := make([]float32, embeddingDim)
		for i := range vec {
			vec[i] = float32(i) * 0.001
		}
		s := formatVector(vec)
		parsed, err := parseVector(s)
		if err != nil {
			t.Fatal(err)
		}
		if len(parsed) != embeddingDim {
			t.Fatalf("len(parsed) = %d, want %d", len(parsed), embeddingDim)
		}
		for i := range vec {
			if parsed[i] != vec[i] {
				t.Errorf("parsed[%d] = %v, want %v", i, parsed[i], vec[i])
			}
		}
	})
}
