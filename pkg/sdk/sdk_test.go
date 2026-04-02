package sdk

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig_Values(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"KernelGRPCAddr", cfg.KernelGRPCAddr, "localhost:9091"},
		{"TaskGRPCAddr", cfg.TaskGRPCAddr, "localhost:9090"},
		{"MemoryGRPCAddr", cfg.MemoryGRPCAddr, "localhost:9092"},
		{"ToolRuntimeHTTPAddr", cfg.ToolRuntimeHTTPAddr, "http://localhost:8083"},
		{"ActorType", cfg.ActorType, "sdk-agent"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestDefaultConfig_Timeout(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.RequestTimeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", cfg.RequestTimeout)
	}
}

func TestNewHTTPClient_Fields(t *testing.T) {
	c := NewHTTPClient("http://example.com/", "agent-1", "tok")
	if c.baseURL != "http://example.com" {
		t.Errorf("trailing slash not trimmed: %q", c.baseURL)
	}
	if c.agentID != "agent-1" {
		t.Errorf("agentID not set: %q", c.agentID)
	}
	if c.authToken != "tok" {
		t.Errorf("authToken not set: %q", c.authToken)
	}
	if c.httpClient == nil {
		t.Fatal("httpClient is nil")
	}
}

func TestNewHTTPClient_DefaultTimeout(t *testing.T) {
	c := NewHTTPClient("http://example.com", "agent-1", "tok")
	if c.httpClient.Timeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", c.httpClient.Timeout)
	}
}

func TestNewHTTPClient_TrailingSlashVariants(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"http://host/", "http://host"},
		{"http://host", "http://host"},
		{"http://host:8080/", "http://host:8080"},
	}
	for _, tc := range tests {
		c := NewHTTPClient(tc.input, "a", "t")
		if c.baseURL != tc.want {
			t.Errorf("NewHTTPClient(%q).baseURL = %q, want %q", tc.input, c.baseURL, tc.want)
		}
	}
}

func TestNewHTTPClient_EmptyToken(t *testing.T) {
	c := NewHTTPClient("http://host", "agent", "")
	if strings.Contains(c.authToken, "Bearer") {
		t.Error("empty token should not produce Bearer prefix")
	}
}
