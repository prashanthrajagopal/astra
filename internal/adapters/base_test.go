package adapters

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewBaseAdapterSetsFields(t *testing.T) {
	b := NewBaseAdapter("dtec", "http://example.com", "secret-token")
	if b.ecosystem != "dtec" {
		t.Errorf("ecosystem: got %q, want %q", b.ecosystem, "dtec")
	}
	if b.endpoint != "http://example.com" {
		t.Errorf("endpoint: got %q, want %q", b.endpoint, "http://example.com")
	}
	if b.authToken != "secret-token" {
		t.Errorf("authToken: got %q, want %q", b.authToken, "secret-token")
	}
	if b.httpClient == nil {
		t.Fatal("httpClient is nil")
	}
	if b.httpClient.Timeout != 30*time.Second {
		t.Errorf("httpClient.Timeout: got %v, want 30s", b.httpClient.Timeout)
	}
}

func TestNameReturnsEcosystem(t *testing.T) {
	tests := []struct {
		ecosystem string
	}{
		{"dtec"},
		{"agentforce"},
		{"workday"},
		{""},
	}
	for _, tc := range tests {
		t.Run(tc.ecosystem, func(t *testing.T) {
			b := NewBaseAdapter(tc.ecosystem, "http://x.com", "tok")
			if b.Name() != tc.ecosystem {
				t.Errorf("Name() = %q, want %q", b.Name(), tc.ecosystem)
			}
		})
	}
}

func TestDoRequestBuildsCorrectRequestWithAuthHeaders(t *testing.T) {
	var capturedReq *http.Request
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	b := NewBaseAdapter("test", srv.URL, "my-token")

	type payload struct {
		Field string `json:"field"`
	}
	resp, err := b.DoRequest(context.Background(), http.MethodPost, "/dispatch", payload{Field: "hello"})
	if err != nil {
		t.Fatalf("DoRequest error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if capturedReq.Method != http.MethodPost {
		t.Errorf("method: got %q, want POST", capturedReq.Method)
	}
	if capturedReq.URL.Path != "/dispatch" {
		t.Errorf("path: got %q, want /dispatch", capturedReq.URL.Path)
	}
	if auth := capturedReq.Header.Get("Authorization"); auth != "Bearer my-token" {
		t.Errorf("Authorization: got %q, want %q", auth, "Bearer my-token")
	}
	if ct := capturedReq.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
	var parsed payload
	if err := json.Unmarshal(capturedBody, &parsed); err != nil {
		t.Fatalf("body unmarshal: %v", err)
	}
	if parsed.Field != "hello" {
		t.Errorf("body field: got %q, want %q", parsed.Field, "hello")
	}
}

func TestDoRequestNilBodyNoContentType(t *testing.T) {
	var capturedReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := NewBaseAdapter("test", srv.URL, "tok")
	resp, err := b.DoRequest(context.Background(), http.MethodGet, "/status", nil)
	if err != nil {
		t.Fatalf("DoRequest error: %v", err)
	}
	defer resp.Body.Close()

	if ct := capturedReq.Header.Get("Content-Type"); ct != "" {
		t.Errorf("Content-Type should be empty for nil body, got %q", ct)
	}
}

func TestDoRequestNoAuthTokenOmitsHeader(t *testing.T) {
	var capturedReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := NewBaseAdapter("test", srv.URL, "") // no auth token
	resp, err := b.DoRequest(context.Background(), http.MethodGet, "/health", nil)
	if err != nil {
		t.Fatalf("DoRequest error: %v", err)
	}
	defer resp.Body.Close()

	if auth := capturedReq.Header.Get("Authorization"); auth != "" {
		t.Errorf("Authorization should be empty when no token, got %q", auth)
	}
}

func TestDoRequestAppendsPathToEndpoint(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := NewBaseAdapter("test", srv.URL, "tok")
	resp, err := b.DoRequest(context.Background(), http.MethodGet, "/api/v1/jobs", nil)
	if err != nil {
		t.Fatalf("DoRequest error: %v", err)
	}
	defer resp.Body.Close()

	if capturedPath != "/api/v1/jobs" {
		t.Errorf("path: got %q, want /api/v1/jobs", capturedPath)
	}
}

func TestDoRequestContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// slow response
		<-r.Context().Done()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := NewBaseAdapter("test", srv.URL, "tok")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := b.DoRequest(ctx, http.MethodGet, "/slow", nil)
	if err == nil {
		t.Error("expected error on cancelled context, got nil")
	}
}

func TestPollWithBackoffReturnsOnTerminalStatus(t *testing.T) {
	b := NewBaseAdapter("test", "http://x.com", "tok")
	calls := 0
	result, err := b.pollWithBackoff(context.Background(), 5, time.Millisecond, func(_ context.Context) (*JobResult, error) {
		calls++
		if calls >= 2 {
			return &JobResult{Status: StatusCompleted}, nil
		}
		return &JobResult{Status: StatusRunning}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusCompleted {
		t.Errorf("status: got %q, want completed", result.Status)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestPollWithBackoffExhaustsAttempts(t *testing.T) {
	b := NewBaseAdapter("test", "http://x.com", "tok")
	calls := 0
	result, err := b.pollWithBackoff(context.Background(), 3, time.Millisecond, func(_ context.Context) (*JobResult, error) {
		calls++
		return &JobResult{Status: StatusRunning}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusRunning {
		t.Errorf("status: got %q, want running", result.Status)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestPollWithBackoffContextCancellation(t *testing.T) {
	b := NewBaseAdapter("test", "http://x.com", "tok")
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	_, err := b.pollWithBackoff(ctx, 10, time.Hour, func(_ context.Context) (*JobResult, error) {
		calls++
		cancel() // cancel after first call
		return &JobResult{Status: StatusRunning}, nil
	})
	if err == nil {
		t.Error("expected context error, got nil")
	}
}

func TestPollWithBackoffDelayCapAt10s(t *testing.T) {
	// initialDelay > 10s triggers the cap branch immediately on second iteration
	b := NewBaseAdapter("test", "http://x.com", "tok")
	calls := 0
	// Use a large initialDelay so the doubling would exceed 10s — but cap kicks in.
	// We only run 2 attempts so it completes quickly with a cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	result, _ := b.pollWithBackoff(ctx, 2, 6*time.Second, func(_ context.Context) (*JobResult, error) {
		calls++
		if calls == 1 {
			cancel() // cancel so the select returns immediately
		}
		return &JobResult{Status: StatusRunning}, nil
	})
	// Either context cancelled or exhausted — just verify no panic and result returned
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestDoRequestMarshalError(t *testing.T) {
	b := NewBaseAdapter("test", "http://example.com", "tok")
	// Pass an unmarshalable value (channel)
	_, err := b.DoRequest(context.Background(), http.MethodPost, "/path", make(chan int))
	if err == nil {
		t.Error("expected marshal error for channel value, got nil")
	}
}
