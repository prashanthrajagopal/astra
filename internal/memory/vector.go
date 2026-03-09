package memory

import (
	"fmt"
	"strconv"
	"strings"
)

// formatVector formats a slice of float32 as a PostgreSQL vector literal "[a,b,c,...]".
func formatVector(embedding []float32) string {
	if len(embedding) == 0 {
		return "[]"
	}
	b := strings.Builder{}
	b.Grow(len(embedding) * 12) // rough estimate
	b.WriteString("[")
	for i, v := range embedding {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'f', -1, 32))
	}
	b.WriteString("]")
	return b.String()
}

// parseVector parses a PostgreSQL vector literal string "[a,b,c,...]" into []float32.
// Returns nil if s is empty or invalid.
func parseVector(s string) ([]float32, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" || s == "NULL" {
		return nil, nil
	}
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	parts := strings.Split(s, ",")
	out := make([]float32, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		f, err := strconv.ParseFloat(p, 32)
		if err != nil {
			return nil, fmt.Errorf("parseVector: %w", err)
		}
		out = append(out, float32(f))
	}
	return out, nil
}
