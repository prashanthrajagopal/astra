package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// newTestServer creates a server with nil DB and client — safe for tests that
// only exercise request parsing / validation logic (before any DB call).
func newTestServer() *server {
	return &server{}
}

// --- handleHealth ---

func TestHandleHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handleHealth(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected 'ok', got %q", w.Body.String())
	}
}

// --- handleCheck ---

func TestHandleCheck_ValidBody(t *testing.T) {
	srv := newTestServer()
	body, _ := json.Marshal(checkReq{Subject: "user1", Action: "read", Resource: "/agents"})
	req := httptest.NewRequest(http.MethodPost, "/check", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleCheck(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp checkResp
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Allowed {
		t.Errorf("expected allowed=true, got false (reason: %q)", resp.Reason)
	}
}

func TestHandleCheck_InvalidJSON(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/check", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	srv.handleCheck(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCheck_EmptyBody(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/check", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	srv.handleCheck(w, req)
	// Empty body is valid JSON — policy evaluates with zero values and allows.
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleCheck_DangerousTool(t *testing.T) {
	srv := newTestServer()
	body, _ := json.Marshal(checkReq{Action: "tool.execute", ToolName: "kubectl-delete"})
	req := httptest.NewRequest(http.MethodPost, "/check", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleCheck(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp checkResp
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Allowed {
		t.Error("expected allowed=false for dangerous tool")
	}
	if !resp.ApprovalRequired {
		t.Error("expected approval_required=true for dangerous tool")
	}
}

func TestHandleCheck_ContentTypeJSON(t *testing.T) {
	srv := newTestServer()
	body, _ := json.Marshal(checkReq{Action: "health"})
	req := httptest.NewRequest(http.MethodPost, "/check", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleCheck(w, req)
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON content-type, got %q", ct)
	}
}

// --- handleApprove / handleDeny request validation ---

func TestHandleApprove_InvalidUUID(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/approvals/not-a-uuid/approve", strings.NewReader(`{"decided_by":"user1"}`))
	req.SetPathValue("id", "not-a-uuid")
	w := httptest.NewRecorder()
	srv.handleApprove(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid approval id") {
		t.Errorf("expected invalid approval id, got %q", w.Body.String())
	}
}

func TestHandleDeny_InvalidUUID(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/approvals/bad/deny", strings.NewReader(`{"decided_by":"user1"}`))
	req.SetPathValue("id", "bad")
	w := httptest.NewRecorder()
	srv.handleDeny(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleApprove_InvalidBody(t *testing.T) {
	srv := newTestServer()
	validUUID := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/approvals/"+validUUID+"/approve", strings.NewReader("not json"))
	req.SetPathValue("id", validUUID)
	w := httptest.NewRecorder()
	srv.handleApprove(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDeny_InvalidBody(t *testing.T) {
	srv := newTestServer()
	validUUID := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/approvals/"+validUUID+"/deny", strings.NewReader("not json"))
	req.SetPathValue("id", validUUID)
	w := httptest.NewRecorder()
	srv.handleDeny(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- handleDecide request validation ---

func TestHandleDecide_InvalidUUID(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/approvals/bad-id/decide", strings.NewReader(`{"decision":"approved","user_id":"u1"}`))
	req.SetPathValue("id", "bad-id")
	w := httptest.NewRecorder()
	srv.handleDecide(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid approval id") {
		t.Errorf("expected invalid approval id, got %q", w.Body.String())
	}
}

func TestHandleDecide_InvalidBody(t *testing.T) {
	srv := newTestServer()
	validUUID := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/approvals/"+validUUID+"/decide", strings.NewReader("not json"))
	req.SetPathValue("id", validUUID)
	w := httptest.NewRecorder()
	srv.handleDecide(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleDecide_BadDecisionValue(t *testing.T) {
	srv := newTestServer()
	validUUID := uuid.New().String()
	body, _ := json.Marshal(map[string]string{"decision": "maybe", "user_id": "u1"})
	req := httptest.NewRequest(http.MethodPost, "/approvals/"+validUUID+"/decide", bytes.NewReader(body))
	req.SetPathValue("id", validUUID)
	w := httptest.NewRecorder()
	srv.handleDecide(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "approved") && !strings.Contains(w.Body.String(), "denied") {
		t.Errorf("expected decision validation error, got %q", w.Body.String())
	}
}

func TestHandleDecide_MissingUserID(t *testing.T) {
	srv := newTestServer()
	validUUID := uuid.New().String()
	body, _ := json.Marshal(map[string]string{"decision": "approved"})
	req := httptest.NewRequest(http.MethodPost, "/approvals/"+validUUID+"/decide", bytes.NewReader(body))
	req.SetPathValue("id", validUUID)
	// No X-User-Id header, no user_id in body
	w := httptest.NewRecorder()
	srv.handleDecide(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing user_id, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "user_id") {
		t.Errorf("expected user_id error, got %q", w.Body.String())
	}
}

func TestHandleDecide_ValidDecisionApproved(t *testing.T) {
	// Verify that "approved" is a valid decision value (passes that check).
	// We test only the body parsing — using body with user_id inline to avoid
	// the DB path being reached with a nil db (which would panic).
	// This test exercises the decision validation branch only.
	srv := newTestServer()
	validUUID := uuid.New().String()

	for _, decision := range []string{"approved", "denied"} {
		body, _ := json.Marshal(map[string]string{"decision": decision, "user_id": "u1"})
		req := httptest.NewRequest(http.MethodPost, "/approvals/"+validUUID+"/decide", bytes.NewReader(body))
		req.SetPathValue("id", validUUID)
		w := httptest.NewRecorder()
		// handleDecide will panic on nil DB after passing validation.
		// Use recover to confirm we got past the decision/user_id checks.
		func() {
			defer func() { recover() }()
			srv.handleDecide(w, req)
		}()
		// If we got 400, the decision was rejected as invalid — that's a failure.
		if w.Code == http.StatusBadRequest {
			t.Errorf("decision %q should be valid, got 400: %s", decision, w.Body.String())
		}
	}
}

// --- handleGetByID request validation ---

func TestHandleGetByID_InvalidUUID(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/approvals/not-valid-uuid", nil)
	req.SetPathValue("id", "not-valid-uuid")
	w := httptest.NewRecorder()
	srv.handleGetByID(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid approval id") {
		t.Errorf("expected invalid approval id, got %q", w.Body.String())
	}
}

func TestHandleGetByID_ValidUUID_PassesValidation(t *testing.T) {
	// Valid UUID passes the uuid.Parse check; the handler then calls DB.
	// With nil DB it panics — use recover to confirm we got past UUID validation.
	srv := newTestServer()
	validUUID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/approvals/"+validUUID, nil)
	req.SetPathValue("id", validUUID)
	w := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		srv.handleGetByID(w, req)
	}()
	// If we got 400, UUID validation failed — that's a bug.
	if w.Code == http.StatusBadRequest {
		t.Errorf("valid UUID should pass validation, got 400: %s", w.Body.String())
	}
}
