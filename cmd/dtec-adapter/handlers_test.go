package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"astra/internal/adapters"
)

// --- handleHealth ---

func TestHandleHealth_AllHealthy(t *testing.T) {
	// Use a mock registry that reports all adapters healthy.
	registry := adapters.NewRegistry()

	handler := handleHealth(registry)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	// Empty registry → no adapters → allHealthy=true
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for empty registry, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if healthy, ok := resp["healthy"].(bool); !ok || !healthy {
		t.Errorf("expected healthy=true, got %v", resp["healthy"])
	}
}

func TestHandleHealth_ContentType(t *testing.T) {
	registry := adapters.NewRegistry()
	handler := handleHealth(registry)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json content-type, got %q", ct)
	}
}

// --- handleCallback ---

func TestHandleCallback_ValidPayload(t *testing.T) {
	adapter := newDtecAdapter("https://api.dtec.example.com", "tok")
	handler := handleCallback(adapter)

	body := `{"job_id":"job-123","event":"complete","status":"completed","data":{}}`
	req := httptest.NewRequest(http.MethodPost, "/callbacks/dtec", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCallback_MissingJobID(t *testing.T) {
	adapter := newDtecAdapter("https://api.dtec.example.com", "tok")
	handler := handleCallback(adapter)

	// Payload without job_id — HandleCallback returns error
	body := `{"event":"complete","status":"completed"}`
	req := httptest.NewRequest(http.MethodPost, "/callbacks/dtec", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for missing job_id, got %d", rec.Code)
	}
}

func TestHandleCallback_InvalidJSON(t *testing.T) {
	adapter := newDtecAdapter("https://api.dtec.example.com", "tok")
	handler := handleCallback(adapter)

	req := httptest.NewRequest(http.MethodPost, "/callbacks/dtec", strings.NewReader("{bad}"))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for invalid JSON, got %d", rec.Code)
	}
}

func TestHandleCallback_EmptyBody(t *testing.T) {
	adapter := newDtecAdapter("https://api.dtec.example.com", "tok")
	handler := handleCallback(adapter)

	req := httptest.NewRequest(http.MethodPost, "/callbacks/dtec", strings.NewReader(""))
	rec := httptest.NewRecorder()
	handler(rec, req)

	// Empty body → JSON decode error → 500
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for empty body, got %d", rec.Code)
	}
}

// --- handleCapabilities ---

func TestHandleCapabilities_ReturnsCapabilities(t *testing.T) {
	adapter := newDtecAdapter("https://api.dtec.example.com", "tok")
	handler := handleCapabilities(adapter)

	req := httptest.NewRequest(http.MethodGet, "/adapters/dtec/capabilities", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var caps []adapters.Capability
	if err := json.Unmarshal(rec.Body.Bytes(), &caps); err != nil {
		t.Fatalf("unmarshal capabilities: %v", err)
	}
	if len(caps) == 0 {
		t.Error("expected at least one capability")
	}
}

func TestHandleCapabilities_ContentType(t *testing.T) {
	adapter := newDtecAdapter("https://api.dtec.example.com", "tok")
	handler := handleCapabilities(adapter)

	req := httptest.NewRequest(http.MethodGet, "/adapters/dtec/capabilities", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json, got %q", ct)
	}
}

// --- DispatchGoal with httptest mock ---

func TestDispatchGoal_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/goals/dispatch" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(dtecDispatchResponse{JobID: "job-dispatch-ok"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	adapter := newDtecAdapter(srv.URL, "test-token")
	goal := adapters.GoalContext{
		GoalID:   "goal-1",
		GoalText: "monitor venue",
		AgentID:  "agent-1",
		Priority: 100,
	}
	jobID, err := adapter.DispatchGoal(context.Background(), "ref-1", goal)
	if err != nil {
		t.Fatalf("DispatchGoal: %v", err)
	}
	if jobID != "job-dispatch-ok" {
		t.Errorf("jobID = %q, want job-dispatch-ok", jobID)
	}
}

func TestDispatchGoal_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	adapter := newDtecAdapter(srv.URL, "tok")
	_, err := adapter.DispatchGoal(context.Background(), "ref", adapters.GoalContext{
		GoalID: "g1", GoalText: "test", AgentID: "a1",
	})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestDispatchGoal_EmptyJobID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dtecDispatchResponse{JobID: ""})
	}))
	defer srv.Close()

	adapter := newDtecAdapter(srv.URL, "tok")
	_, err := adapter.DispatchGoal(context.Background(), "ref", adapters.GoalContext{
		GoalID: "g1", GoalText: "test", AgentID: "a1",
	})
	if err == nil {
		t.Fatal("expected error for empty job_id")
	}
	if !strings.Contains(err.Error(), "empty job_id") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDispatchGoal_ErrorInResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dtecDispatchResponse{Error: "quota exceeded"})
	}))
	defer srv.Close()

	adapter := newDtecAdapter(srv.URL, "tok")
	_, err := adapter.DispatchGoal(context.Background(), "ref", adapters.GoalContext{
		GoalID: "g1", GoalText: "test", AgentID: "a1",
	})
	if err == nil {
		t.Fatal("expected error for non-empty error field")
	}
	if !strings.Contains(err.Error(), "quota exceeded") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- PollStatus with httptest mock ---

func TestPollStatus_Completed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dtecStatusResponse{
			JobID:  "job-1",
			Status: "completed",
			Output: json.RawMessage(`{"result":"done"}`),
		})
	}))
	defer srv.Close()

	adapter := newDtecAdapter(srv.URL, "tok")
	result, err := adapter.PollStatus(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("PollStatus: %v", err)
	}
	if result.Status != adapters.StatusCompleted {
		t.Errorf("status = %v, want Completed", result.Status)
	}
}

func TestPollStatus_Pending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dtecStatusResponse{JobID: "job-2", Status: "pending"})
	}))
	defer srv.Close()

	adapter := newDtecAdapter(srv.URL, "tok")
	result, err := adapter.PollStatus(context.Background(), "job-2")
	if err != nil {
		t.Fatalf("PollStatus: %v", err)
	}
	if result.Status != adapters.StatusPending {
		t.Errorf("status = %v, want Pending", result.Status)
	}
}

func TestPollStatus_Running(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dtecStatusResponse{JobID: "job-3", Status: "running"})
	}))
	defer srv.Close()

	adapter := newDtecAdapter(srv.URL, "tok")
	result, err := adapter.PollStatus(context.Background(), "job-3")
	if err != nil {
		t.Fatalf("PollStatus: %v", err)
	}
	if result.Status != adapters.StatusRunning {
		t.Errorf("status = %v, want Running", result.Status)
	}
}

func TestPollStatus_Failed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dtecStatusResponse{JobID: "job-4", Status: "failed", Error: "execution failed"})
	}))
	defer srv.Close()

	adapter := newDtecAdapter(srv.URL, "tok")
	result, err := adapter.PollStatus(context.Background(), "job-4")
	if err != nil {
		t.Fatalf("PollStatus: %v", err)
	}
	if result.Status != adapters.StatusFailed {
		t.Errorf("status = %v, want Failed", result.Status)
	}
	if result.Error != "execution failed" {
		t.Errorf("error = %q, want execution failed", result.Error)
	}
}

func TestPollStatus_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	adapter := newDtecAdapter(srv.URL, "tok")
	_, err := adapter.PollStatus(context.Background(), "missing-job")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPollStatus_UnknownStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dtecStatusResponse{JobID: "job-5", Status: "unknown_state"})
	}))
	defer srv.Close()

	adapter := newDtecAdapter(srv.URL, "tok")
	result, err := adapter.PollStatus(context.Background(), "job-5")
	if err != nil {
		t.Fatalf("PollStatus: %v", err)
	}
	// Unknown status → defaults to Pending
	if result.Status != adapters.StatusPending {
		t.Errorf("unknown status should map to Pending, got %v", result.Status)
	}
}

// --- HealthCheck ---

func TestHealthCheck_Healthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dtecHealthResponse{Healthy: true})
	}))
	defer srv.Close()

	adapter := newDtecAdapter(srv.URL, "tok")
	healthy, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if !healthy {
		t.Error("expected healthy=true")
	}
}

func TestHealthCheck_Unhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	adapter := newDtecAdapter(srv.URL, "tok")
	healthy, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if healthy {
		t.Error("expected healthy=false for 503")
	}
}

func TestHealthCheck_200WithUnparseableBody(t *testing.T) {
	// 200 with non-JSON body → still counts as healthy
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	adapter := newDtecAdapter(srv.URL, "tok")
	healthy, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if !healthy {
		t.Error("expected healthy=true for 200 with unparseable body")
	}
}
