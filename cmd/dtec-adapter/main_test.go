package main

import (
	"context"
	"testing"

	"astra/internal/adapters"
)

func TestDtecAdapterListCapabilities(t *testing.T) {
	adapter := newDtecAdapter("https://api.dtec.example.com", "test-token")

	caps, err := adapter.ListCapabilities(context.Background())
	if err != nil {
		t.Fatalf("ListCapabilities() error = %v", err)
	}

	if len(caps) == 0 {
		t.Fatal("expected at least one capability, got none")
	}

	expectedNames := []string{"venue_monitoring", "crowd_analytics", "perimeter_security"}
	if len(caps) != len(expectedNames) {
		t.Errorf("len(caps) = %d, want %d", len(caps), len(expectedNames))
	}

	capByName := make(map[string]adapters.Capability)
	for _, c := range caps {
		capByName[c.Name] = c
	}

	for _, name := range expectedNames {
		c, ok := capByName[name]
		if !ok {
			t.Errorf("capability %q not found", name)
			continue
		}
		if c.Description == "" {
			t.Errorf("capability %q has empty description", name)
		}
		if c.Version == "" {
			t.Errorf("capability %q has empty version", name)
		}
	}
}

func TestDtecAdapterConstruction(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  string
		authToken string
	}{
		{"with token", "https://api.example.com", "tok-abc"},
		{"empty token", "https://api.example.com", ""},
		{"custom endpoint", "http://localhost:9000", "secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := newDtecAdapter(tt.endpoint, tt.authToken)
			if adapter == nil {
				t.Fatal("expected non-nil adapter")
			}
			if adapter.BaseAdapter == nil {
				t.Fatal("expected non-nil BaseAdapter")
			}
		})
	}
}

func TestGetEnvDtec(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envVal   string
		setEnv   bool
		fallback string
		want     string
	}{
		{
			name:     "returns env value when set",
			key:      "DTEC_TEST_KEY",
			envVal:   "8098",
			setEnv:   true,
			fallback: "default",
			want:     "8098",
		},
		{
			name:     "returns fallback when not set",
			key:      "DTEC_UNSET_KEY",
			setEnv:   false,
			fallback: "8098",
			want:     "8098",
		},
		{
			name:     "empty env value uses fallback",
			key:      "DTEC_EMPTY_KEY",
			envVal:   "",
			setEnv:   true,
			fallback: "https://api.dtec.example.com",
			want:     "https://api.dtec.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.key, tt.envVal)
			}
			got := getEnv(tt.key, tt.fallback)
			if got != tt.want {
				t.Errorf("getEnv(%q, %q) = %q, want %q", tt.key, tt.fallback, got, tt.want)
			}
		})
	}
}
