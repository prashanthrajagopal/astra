package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestServer returns a server with no signing secret (skip verification) and nil store/bus.
// Tests that require store/bus interactions only validate up to the point where they're needed.
func newTestServer(signingSecret string) *server {
	return &server{signingSecret: signingSecret}
}

func TestHandleSlackEvents_MethodNotAllowed(t *testing.T) {
	srv := newTestServer("")
	req := httptest.NewRequest(http.MethodGet, "/slack/events", nil)
	w := httptest.NewRecorder()
	srv.handleSlackEvents(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleSlackEvents_BadBody(t *testing.T) {
	srv := newTestServer("")
	// body that causes read error — we simulate with a nil body (causes bad body)
	req := httptest.NewRequest(http.MethodPost, "/slack/events", nil)
	req.Body = nil
	w := httptest.NewRecorder()
	// nil body: readBody will fail on ReadFrom
	// Actually http.NoBody is nil-safe; force an error by closing body.
	// Use a custom ReadCloser that errors.
	req.Body = errReader{}
	w = httptest.NewRecorder()
	srv.handleSlackEvents(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// errReader implements io.ReadCloser and always returns an error on Read.
type errReader struct{}

func (errReader) Read(p []byte) (int, error) { return 0, bytes.ErrTooLarge }
func (errReader) Close() error               { return nil }

func TestHandleSlackEvents_InvalidJSON(t *testing.T) {
	srv := newTestServer("")
	body := []byte(`not json`)
	req := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleSlackEvents(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSlackEvents_URLVerification(t *testing.T) {
	srv := newTestServer("")
	payload := map[string]string{
		"type":      "url_verification",
		"challenge": "test-challenge-token",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleSlackEvents(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["challenge"] != "test-challenge-token" {
		t.Errorf("expected challenge echo, got %q", resp["challenge"])
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON content-type, got %q", ct)
	}
}

func TestHandleSlackEvents_UnknownType(t *testing.T) {
	srv := newTestServer("")
	payload := map[string]string{"type": "something_else"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleSlackEvents(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for unknown type, got %d", w.Code)
	}
}

func TestHandleSlackEvents_EventCallback_BotMessage(t *testing.T) {
	// Bot messages (bot_id set) should be ignored with 200.
	srv := newTestServer("")
	evt := map[string]interface{}{
		"type":   "message",
		"bot_id": "B123",
		"user":   "U456",
	}
	evtJSON, _ := json.Marshal(evt)
	payload := map[string]interface{}{
		"type":  "event_callback",
		"event": json.RawMessage(evtJSON),
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleSlackEvents(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for bot message, got %d", w.Code)
	}
}

func TestHandleSlackEvents_EventCallback_NonMessageType(t *testing.T) {
	srv := newTestServer("")
	evt := map[string]interface{}{
		"type": "reaction_added",
		"user": "U456",
	}
	evtJSON, _ := json.Marshal(evt)
	payload := map[string]interface{}{
		"type":  "event_callback",
		"event": json.RawMessage(evtJSON),
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleSlackEvents(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for non-message event, got %d", w.Code)
	}
}

func TestHandleSlackEvents_EventCallback_MissingFields(t *testing.T) {
	// Message event missing team_id/channel/user should return 200 (silently dropped).
	srv := newTestServer("")
	evt := map[string]interface{}{
		"type": "message",
		// no team_id, channel, user
	}
	evtJSON, _ := json.Marshal(evt)
	payload := map[string]interface{}{
		"type":  "event_callback",
		"event": json.RawMessage(evtJSON),
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleSlackEvents(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for missing fields, got %d", w.Code)
	}
}

func TestHandleSlackEvents_WithSigningSecret_MissingSig(t *testing.T) {
	// With a signing secret set, missing signature should return 401.
	srv := newTestServer("mysecret")
	payload := map[string]string{"type": "url_verification", "challenge": "abc"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	// No X-Slack-Signature header set
	w := httptest.NewRecorder()
	srv.handleSlackEvents(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid signature, got %d", w.Code)
	}
}

func TestGetStr(t *testing.T) {
	m := map[string]interface{}{
		"key":    "value",
		"number": 42,
	}
	if got := getStr(m, "key"); got != "value" {
		t.Errorf("expected 'value', got %q", got)
	}
	if got := getStr(m, "missing"); got != "" {
		t.Errorf("expected empty string for missing key, got %q", got)
	}
	if got := getStr(m, "number"); got != "" {
		t.Errorf("expected empty string for non-string value, got %q", got)
	}
}
