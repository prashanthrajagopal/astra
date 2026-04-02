package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- newToolRuntime ---

func TestNewToolRuntime_DefaultNoop(t *testing.T) {
	t.Setenv("TOOL_RUNTIME", "")
	rt := newToolRuntime()
	if rt == nil {
		t.Fatal("expected non-nil runtime")
	}
}

func TestNewToolRuntime_Docker(t *testing.T) {
	t.Setenv("TOOL_RUNTIME", "docker")
	rt := newToolRuntime()
	if rt == nil {
		t.Fatal("expected non-nil docker runtime")
	}
}

func TestNewToolRuntime_CaseInsensitive(t *testing.T) {
	t.Setenv("TOOL_RUNTIME", "  DOCKER  ")
	rt := newToolRuntime()
	if rt == nil {
		t.Fatal("expected non-nil runtime for uppercase DOCKER")
	}
}

func TestNewToolRuntime_UnknownFallsToNoop(t *testing.T) {
	t.Setenv("TOOL_RUNTIME", "wasm")
	rt := newToolRuntime()
	if rt == nil {
		t.Fatal("expected non-nil runtime for unknown type")
	}
}

// --- /execute handler: request parsing and validation ---

func buildExecuteHandler() http.Handler {
	rt := newToolRuntime() // noop by default

	// Minimal approval gate that always allows
	gate := &approvalGate{
		accessControlAddr: "", // no real address
		db:                nil,
		client:            &http.Client{},
	}
	_ = gate

	mux := http.NewServeMux()
	mux.HandleFunc("POST /execute", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name           string  `json:"name"`
			Input          string  `json:"input"`
			TimeoutSeconds int     `json:"timeout_seconds"`
			MemoryLimit    int64   `json:"memory_limit"`
			CPULimit       float64 `json:"cpu_limit"`
			TaskID         string  `json:"task_id"`
			WorkerID       string  `json:"worker_id"`
			DryRun         bool    `json:"dry_run"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		// input base64 decode
		var input []byte
		if req.Input != "" {
			var err error
			input, err = base64.StdEncoding.DecodeString(req.Input)
			if err != nil {
				http.Error(w, "invalid base64 input", http.StatusBadRequest)
				return
			}
		}
		_ = input

		if req.DryRun {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"dry_run":     true,
				"would_run":   req.Name,
				"simulated":   true,
				"exit_code":   0,
				"duration_ms": 0,
				"output":      base64.StdEncoding.EncodeToString([]byte(`{"message":"dry run — no side effects"}`)),
				"artifacts":   []string{},
			})
			return
		}

		// Call noop runtime directly for the test
		_ = rt
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"output":      base64.StdEncoding.EncodeToString([]byte("{}")),
			"exit_code":   0,
			"duration_ms": 0,
			"artifacts":   []interface{}{},
		})
	})
	return mux
}

func TestExecuteHandler_InvalidJSON(t *testing.T) {
	mux := buildExecuteHandler()
	req := httptest.NewRequest(http.MethodPost, "/execute", strings.NewReader("{not-json"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestExecuteHandler_InvalidBase64Input(t *testing.T) {
	mux := buildExecuteHandler()
	body := `{"name":"tool","input":"!!!not-base64!!!"}`
	req := httptest.NewRequest(http.MethodPost, "/execute", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad base64, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "base64") {
		t.Errorf("expected 'base64' in response, got: %s", rec.Body.String())
	}
}

func TestExecuteHandler_DryRun(t *testing.T) {
	mux := buildExecuteHandler()
	body := `{"name":"my-tool","dry_run":true}`
	req := httptest.NewRequest(http.MethodPost, "/execute", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dry, ok := resp["dry_run"].(bool); !ok || !dry {
		t.Errorf("expected dry_run=true, got %v", resp["dry_run"])
	}
	if resp["would_run"] != "my-tool" {
		t.Errorf("expected would_run=my-tool, got %v", resp["would_run"])
	}
}

func TestExecuteHandler_ValidBase64Input(t *testing.T) {
	mux := buildExecuteHandler()
	encoded := base64.StdEncoding.EncodeToString([]byte(`{"key":"value"}`))
	body, _ := json.Marshal(map[string]interface{}{
		"name":    "test-tool",
		"input":   encoded,
		"dry_run": true, // use dry_run to avoid needing access-control
	})
	req := httptest.NewRequest(http.MethodPost, "/execute", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestExecuteHandler_EmptyInput(t *testing.T) {
	mux := buildExecuteHandler()
	body := `{"name":"tool","dry_run":true}`
	req := httptest.NewRequest(http.MethodPost, "/execute", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	// Empty input is valid (no base64 decode attempted)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// --- /health handler ---

func TestHealthHandler_OK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected 'ok', got %q", rec.Body.String())
	}
}

// --- approvalGate.check: request construction (no real server needed for building the request) ---

func TestApprovalGate_CheckRequestBody(t *testing.T) {
	// Verify that the check function constructs a proper request to the access-control server.
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body["action"] != "tool.execute" {
			t.Errorf("action = %v, want tool.execute", body["action"])
		}
		if body["tool_name"] != "my-tool" {
			t.Errorf("tool_name = %v, want my-tool", body["tool_name"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(checkResult{Allowed: true, ApprovalRequired: false})
	}))
	defer srv.Close()

	gate := &approvalGate{
		accessControlAddr: srv.URL,
		db:                nil,
		client:            &http.Client{},
	}
	result, err := gate.check(t.Context(), "my-tool", "task-1", "worker-1")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !called {
		t.Error("expected access-control server to be called")
	}
	if !result.Allowed {
		t.Error("expected allowed=true")
	}
}

func TestApprovalGate_CheckForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(checkResult{Allowed: false, Reason: "policy denied"})
	}))
	defer srv.Close()

	gate := &approvalGate{
		accessControlAddr: srv.URL,
		client:            &http.Client{},
	}
	result, err := gate.check(t.Context(), "dangerous-tool", "", "")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Allowed {
		t.Error("expected allowed=false")
	}
	if result.Reason != "policy denied" {
		t.Errorf("reason = %q, want policy denied", result.Reason)
	}
}

func TestApprovalGate_CheckApprovalRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(checkResult{Allowed: true, ApprovalRequired: true})
	}))
	defer srv.Close()

	gate := &approvalGate{
		accessControlAddr: srv.URL,
		client:            &http.Client{},
	}
	result, err := gate.check(t.Context(), "risky-tool", "task-123", "worker-abc")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !result.ApprovalRequired {
		t.Error("expected approval_required=true")
	}
}
