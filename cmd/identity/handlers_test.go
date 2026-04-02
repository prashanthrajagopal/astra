package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newIdentityServer returns a minimal identityServer for handler tests.
// Handlers under test must return before reaching the store.
func newIdentityServer() *identityServer {
	return &identityServer{jwtSecret: "test-secret"}
}

// --- handleLogin ---

func TestHandleLogin_InvalidJSON(t *testing.T) {
	srv := newIdentityServer()
	req := httptest.NewRequest(http.MethodPost, "/users/login", bytes.NewBufferString("not json"))
	rr := httptest.NewRecorder()

	srv.handleLogin(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleLogin_MissingEmail(t *testing.T) {
	srv := newIdentityServer()
	body, _ := json.Marshal(map[string]string{"password": "secret"})
	req := httptest.NewRequest(http.MethodPost, "/users/login", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleLogin(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "email and password required" {
		t.Errorf("error = %q, want %q", resp["error"], "email and password required")
	}
}

func TestHandleLogin_MissingPassword(t *testing.T) {
	srv := newIdentityServer()
	body, _ := json.Marshal(map[string]string{"email": "user@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/users/login", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleLogin(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "email and password required" {
		t.Errorf("error = %q, want %q", resp["error"], "email and password required")
	}
}

func TestHandleLogin_BothMissing(t *testing.T) {
	srv := newIdentityServer()
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/users/login", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleLogin(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// --- handleValidateToken ---

func TestHandleValidateToken_InvalidJSON(t *testing.T) {
	srv := newIdentityServer()
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewBufferString("{bad"))
	rr := httptest.NewRecorder()

	srv.handleValidateToken(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleValidateToken_EmptyToken(t *testing.T) {
	srv := newIdentityServer()
	body, _ := json.Marshal(map[string]string{"token": ""})
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleValidateToken(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	var resp validateResp
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Valid {
		t.Error("expected valid=false for empty token")
	}
}

func TestHandleValidateToken_WhitespaceOnlyToken(t *testing.T) {
	srv := newIdentityServer()
	body, _ := json.Marshal(map[string]string{"token": "   "})
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleValidateToken(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	var resp validateResp
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Valid {
		t.Error("expected valid=false for whitespace token")
	}
}

func TestHandleValidateToken_InvalidToken(t *testing.T) {
	srv := newIdentityServer()
	body, _ := json.Marshal(map[string]string{"token": "not.a.valid.jwt"})
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleValidateToken(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	var resp validateResp
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Valid {
		t.Error("expected valid=false for invalid JWT")
	}
}

// --- handleCreateUser ---

func TestHandleCreateUser_InvalidJSON(t *testing.T) {
	srv := newIdentityServer()
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString("bad"))
	rr := httptest.NewRecorder()

	srv.handleCreateUser(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateUser_MissingEmail(t *testing.T) {
	srv := newIdentityServer()
	body, _ := json.Marshal(map[string]string{"name": "Alice", "password": "secret"})
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleCreateUser(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "email, name, and password required" {
		t.Errorf("error = %q, want %q", resp["error"], "email, name, and password required")
	}
}

func TestHandleCreateUser_MissingName(t *testing.T) {
	srv := newIdentityServer()
	body, _ := json.Marshal(map[string]string{"email": "a@b.com", "password": "secret"})
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleCreateUser(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateUser_MissingPassword(t *testing.T) {
	srv := newIdentityServer()
	body, _ := json.Marshal(map[string]string{"email": "a@b.com", "name": "Alice"})
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleCreateUser(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// --- handleGetUser ---

func TestHandleGetUser_InvalidID(t *testing.T) {
	srv := newIdentityServer()
	req := httptest.NewRequest(http.MethodGet, "/users/not-a-uuid", nil)
	req.SetPathValue("id", "not-a-uuid")
	rr := httptest.NewRecorder()

	srv.handleGetUser(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != errInvalidUserID {
		t.Errorf("error = %q, want %q", resp["error"], errInvalidUserID)
	}
}

// --- handleUpdateUser ---

func TestHandleUpdateUser_InvalidID(t *testing.T) {
	srv := newIdentityServer()
	req := httptest.NewRequest(http.MethodPatch, "/users/bad", nil)
	req.SetPathValue("id", "bad")
	rr := httptest.NewRecorder()

	srv.handleUpdateUser(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleUpdateUser_InvalidBody(t *testing.T) {
	srv := newIdentityServer()
	req := httptest.NewRequest(http.MethodPatch, "/users/00000000-0000-0000-0000-000000000001",
		bytes.NewBufferString("not json"))
	req.SetPathValue("id", "00000000-0000-0000-0000-000000000001")
	rr := httptest.NewRecorder()

	srv.handleUpdateUser(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// --- handleResetPassword ---

func TestHandleResetPassword_InvalidID(t *testing.T) {
	srv := newIdentityServer()
	req := httptest.NewRequest(http.MethodPost, "/users/bad/reset-password", nil)
	req.SetPathValue("id", "bad")
	rr := httptest.NewRecorder()

	srv.handleResetPassword(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleResetPassword_MissingNewPassword(t *testing.T) {
	srv := newIdentityServer()
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/users/00000000-0000-0000-0000-000000000001/reset-password",
		bytes.NewBuffer(body))
	req.SetPathValue("id", "00000000-0000-0000-0000-000000000001")
	rr := httptest.NewRecorder()

	srv.handleResetPassword(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "new_password required" {
		t.Errorf("error = %q, want %q", resp["error"], "new_password required")
	}
}
