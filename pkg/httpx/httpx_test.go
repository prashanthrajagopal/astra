package httpx

import (
	"net/http"
	"testing"
	"time"

	"astra/pkg/config"
)

func TestNewClient_NoTLS(t *testing.T) {
	cfg := &config.Config{TLSEnabled: false}
	client, err := NewClient(cfg, 5*time.Second)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Timeout != 5*time.Second {
		t.Errorf("Timeout: got %v, want 5s", client.Timeout)
	}
}

func TestNewClient_Timeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{"zero", 0},
		{"one_second", time.Second},
		{"thirty_seconds", 30 * time.Second},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			client, err := NewClient(cfg, tc.timeout)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if client.Timeout != tc.timeout {
				t.Errorf("Timeout: got %v, want %v", client.Timeout, tc.timeout)
			}
		})
	}
}

func TestNewClient_TLSEnabled_MissingCAFile(t *testing.T) {
	cfg := &config.Config{
		TLSEnabled: true,
		TLSCAFile:  "/nonexistent/ca.pem",
	}
	_, err := NewClient(cfg, time.Second)
	if err == nil {
		t.Error("expected error for missing CA file, got nil")
	}
}

func TestNewClient_TLSEnabled_NoCAFile(t *testing.T) {
	// TLS enabled without CA file — should succeed (uses system roots)
	cfg := &config.Config{
		TLSEnabled:            true,
		TLSInsecureSkipVerify: true,
	}
	client, err := NewClient(cfg, time.Second)
	if err != nil {
		t.Fatalf("NewClient with TLS (no CA): %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewClient_TLSEnabled_BadCertKeyPair(t *testing.T) {
	cfg := &config.Config{
		TLSEnabled:  true,
		TLSCertFile: "/nonexistent/cert.pem",
		TLSKeyFile:  "/nonexistent/key.pem",
	}
	_, err := NewClient(cfg, time.Second)
	if err == nil {
		t.Error("expected error for missing cert/key files, got nil")
	}
}

func TestNewClient_ReturnsHTTPClient(t *testing.T) {
	cfg := &config.Config{}
	client, err := NewClient(cfg, 10*time.Second)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, ok := interface{}(client).(*http.Client); !ok {
		t.Error("expected *http.Client")
	}
}

func TestNewClient_TransportNotNil(t *testing.T) {
	cfg := &config.Config{TLSEnabled: false}
	client, err := NewClient(cfg, time.Second)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.Transport == nil {
		t.Error("expected non-nil Transport")
	}
}
