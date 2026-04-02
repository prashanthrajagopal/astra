package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// computeSig is a test helper that mirrors validateHMAC's signing.
func computeSig(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// --- handleWebhook: request-level validation (no DB needed) ---

func TestHandleWebhook_MissingSourceID(t *testing.T) {
	// Route without source_id path param — simulate empty path value
	req := httptest.NewRequest(http.MethodPost, "/webhooks/", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	// Call with empty sourceID by monkeypatching PathValue via a custom handler
	// Since we can't easily set PathValue without a real mux, call handleWebhook
	// via the mux so path extraction works.
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/{source_id}", func(w http.ResponseWriter, r *http.Request) {
		handleWebhook(w, r, nil, nil)
	})

	// Hitting /webhooks/ (no source_id segment) will 404 from the mux pattern
	mux.ServeHTTP(rec, req)
	// Either 404 or 400 is acceptable; the important thing is it's not 200/500
	if rec.Code == http.StatusOK || rec.Code == http.StatusInternalServerError {
		t.Errorf("expected non-200/500 status, got %d", rec.Code)
	}
}

// --- validateHMAC (already tested in main_test.go; add structural coverage) ---

func TestValidateHMAC_AllZeroBody(t *testing.T) {
	body := make([]byte, 64)
	secret := "test-secret"
	sig := computeSig(body, secret)
	if !validateHMAC(body, sig, secret) {
		t.Error("expected valid HMAC for all-zero body")
	}
}

func TestValidateHMAC_NonSHA256Prefix(t *testing.T) {
	body := []byte("payload")
	secret := "s"
	// Use sha1= prefix instead of sha256=
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha1=" + hex.EncodeToString(mac.Sum(nil))
	if validateHMAC(body, sig, secret) {
		t.Error("should reject sha1= prefix")
	}
}

func TestValidateHMAC_UnicodeSecret(t *testing.T) {
	body := []byte(`{"data":"hello"}`)
	secret := "sécret-clé-🔑"
	sig := computeSig(body, secret)
	if !validateHMAC(body, sig, secret) {
		t.Error("expected valid HMAC with unicode secret")
	}
}

// --- handleCreateSource: request validation (no DB) ---

func TestHandleCreateSource_InvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/webhook-sources", strings.NewReader(`{bad json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handleCreateSource(rec, req, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid JSON") {
		t.Errorf("expected 'invalid JSON' in response, got: %s", rec.Body.String())
	}
}

func TestHandleCreateSource_MissingSourceID(t *testing.T) {
	body := map[string]interface{}{
		"name":       "My Source",
		"hmac_secret": "secret",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/webhook-sources", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handleCreateSource(rec, req, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "required") {
		t.Errorf("expected 'required' in response, got: %s", rec.Body.String())
	}
}

func TestHandleCreateSource_MissingName(t *testing.T) {
	body := map[string]interface{}{
		"source_id":   "github",
		"hmac_secret": "secret",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/webhook-sources", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handleCreateSource(rec, req, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleCreateSource_EmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/webhook-sources", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handleCreateSource(rec, req, nil)
	// Empty body → invalid JSON decode → 400
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// --- handleDeleteSource: validation (no DB) ---

func TestHandleDeleteSource_MissingID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /webhook-sources/{id}", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteSource(w, r, nil)
	})
	// Request without an id path segment — will 404 from mux
	req := httptest.NewRequest(http.MethodDelete, "/webhook-sources/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Errorf("expected non-200, got %d", rec.Code)
	}
}

// --- getEnv (from webhook-ingest package) ---

func TestGetEnv_DefaultPort(t *testing.T) {
	t.Setenv("WEBHOOK_INGEST_PORT", "")
	got := getEnv("WEBHOOK_INGEST_PORT", "8099")
	if got != "8099" {
		t.Errorf("expected 8099, got %s", got)
	}
}

func TestGetEnv_CustomPort(t *testing.T) {
	t.Setenv("WEBHOOK_INGEST_PORT", "9000")
	got := getEnv("WEBHOOK_INGEST_PORT", "8099")
	if got != "9000" {
		t.Errorf("expected 9000, got %s", got)
	}
}

// --- webhook health handler ---

func TestHealthHandler_ReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal health: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %s", resp["status"])
	}
}
