package main

import (
	"testing"

	"github.com/google/uuid"
)

func TestStrVal(t *testing.T) {
	m := map[string]interface{}{
		"str":    "hello",
		"number": int64(42),
		"float":  3.14,
	}
	tests := []struct {
		key  string
		want string
	}{
		{"str", "hello"},
		{"number", ""},
		{"float", ""},
		{"missing", ""},
	}
	for _, tt := range tests {
		got := strVal(m, tt.key)
		if got != tt.want {
			t.Errorf("strVal(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestIntVal(t *testing.T) {
	m := map[string]interface{}{
		"str_num": "123",
		"int64":   int64(99),
		"int":     int(7),
		"float64": float64(4.9),
		"str_bad": "notanumber",
	}
	tests := []struct {
		key  string
		want int
	}{
		{"str_num", 123},
		{"int64", 99},
		{"int", 7},
		{"float64", 4},
		{"str_bad", 0},
		{"missing", 0},
	}
	for _, tt := range tests {
		got := intVal(m, tt.key)
		if got != tt.want {
			t.Errorf("intVal(%q) = %d, want %d", tt.key, got, tt.want)
		}
	}
}

func TestFloatVal(t *testing.T) {
	m := map[string]interface{}{
		"str_float": "1.5",
		"float64":   float64(2.5),
		"int64":     int64(3),
		"int":       int(4),
		"str_bad":   "notafloat",
	}
	tests := []struct {
		key  string
		want float64
	}{
		{"str_float", 1.5},
		{"float64", 2.5},
		{"int64", 3.0},
		{"int", 4.0},
		{"str_bad", 0.0},
		{"missing", 0.0},
	}
	for _, tt := range tests {
		got := floatVal(m, tt.key)
		if got != tt.want {
			t.Errorf("floatVal(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestParseUUID(t *testing.T) {
	valid := "550e8400-e29b-41d4-a716-446655440000"
	tests := []struct {
		input   string
		wantNil bool
	}{
		{"", true},
		{"notauuid", true},
		{valid, false},
	}
	for _, tt := range tests {
		got := parseUUID(tt.input)
		isNil := got == uuid.Nil
		if isNil != tt.wantNil {
			t.Errorf("parseUUID(%q): got nil=%v, want nil=%v", tt.input, isNil, tt.wantNil)
		}
		if !tt.wantNil {
			if got.String() != valid {
				t.Errorf("parseUUID(%q) = %v, want %v", tt.input, got, valid)
			}
		}
	}
}
