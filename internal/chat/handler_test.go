package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckSessionRateLimit_AllowsUnderLimit(t *testing.T) {
	// Use a unique session ID to avoid state leakage
	sessionID := "test-rate-limit-allow-" + time.Now().Format("150405.000000000")
	for i := 0; i < 5; i++ {
		allowed, retrySecs := checkSessionRateLimit(sessionID, 10)
		if !allowed {
			t.Errorf("iteration %d: expected allowed=true, got false (retrySecs=%d)", i, retrySecs)
		}
		if retrySecs != 0 {
			t.Errorf("iteration %d: expected retrySecs=0, got %d", i, retrySecs)
		}
	}
}

func TestCheckSessionRateLimit_BlocksAtLimit(t *testing.T) {
	sessionID := "test-rate-limit-block-" + time.Now().Format("150405.000000000")
	maxPerMin := 3
	for i := 0; i < maxPerMin; i++ {
		allowed, _ := checkSessionRateLimit(sessionID, maxPerMin)
		if !allowed {
			t.Fatalf("expected allowed on iteration %d", i)
		}
	}
	allowed, retrySecs := checkSessionRateLimit(sessionID, maxPerMin)
	if allowed {
		t.Error("expected blocked after hitting limit")
	}
	if retrySecs < 0 {
		t.Errorf("retrySecs should be >= 0, got %d", retrySecs)
	}
}

func TestCheckSessionRateLimit_DifferentSessionsIndependent(t *testing.T) {
	ts := time.Now().Format("150405.000000000")
	session1 := "sess1-" + ts
	session2 := "sess2-" + ts

	// Exhaust session1
	for i := 0; i < 2; i++ {
		checkSessionRateLimit(session1, 2)
	}
	// session1 is now blocked
	if allowed, _ := checkSessionRateLimit(session1, 2); allowed {
		t.Error("session1 should be blocked")
	}
	// session2 should still be allowed
	if allowed, _ := checkSessionRateLimit(session2, 2); !allowed {
		t.Error("session2 should not be blocked")
	}
}

func TestValidateToken_ValidResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: got %s, want POST", r.Method)
		}
		if r.URL.Path != "/validate" {
			t.Errorf("path: got %s, want /validate", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"valid":   true,
			"subject": "user-123",
		})
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	subject, err := validateToken(t.Context(), client, srv.URL, "my-token")
	if err != nil {
		t.Fatalf("validateToken: %v", err)
	}
	if subject != "user-123" {
		t.Errorf("subject: got %q, want user-123", subject)
	}
}

func TestValidateToken_InvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"valid":   false,
			"subject": "",
		})
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := validateToken(t.Context(), client, srv.URL, "bad-token")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestValidateToken_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := validateToken(t.Context(), client, srv.URL, "tok")
	if err == nil {
		t.Error("expected error for non-OK status")
	}
}

func TestValidateToken_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := validateToken(t.Context(), client, srv.URL, "tok")
	if err == nil {
		t.Error("expected error for bad JSON response")
	}
}

func TestValidateToken_ServerDown(t *testing.T) {
	client := &http.Client{Timeout: 100 * time.Millisecond}
	_, err := validateToken(t.Context(), client, "http://127.0.0.1:1", "tok")
	if err == nil {
		t.Error("expected error when server is down")
	}
}
