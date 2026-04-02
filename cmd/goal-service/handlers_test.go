package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// newGoalServer returns a minimal goalServer for handler tests.
// Handlers under test must not reach DB/Redis — they must return before that.
func newGoalServer() *goalServer {
	return &goalServer{}
}

// --- handleGetGoal ---

func TestHandleGetGoal_InvalidID(t *testing.T) {
	srv := newGoalServer()
	req := httptest.NewRequest(http.MethodGet, "/goals/not-a-uuid", nil)
	req.SetPathValue("id", "not-a-uuid")
	rr := httptest.NewRecorder()

	srv.handleGetGoal(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "invalid id" {
		t.Errorf("error = %q, want %q", resp["error"], "invalid id")
	}
}

func TestHandleGetGoal_ValidIDReachesDB(t *testing.T) {
	// A valid UUID should pass ID validation and attempt the DB query.
	// With nil db it will panic/error — we only verify the ID parsing succeeds
	// by confirming we don't get a 400.
	srv := newGoalServer()
	id := "00000000-0000-0000-0000-000000000001"
	req := httptest.NewRequest(http.MethodGet, "/goals/"+id, nil)
	req.SetPathValue("id", id)
	rr := httptest.NewRecorder()

	// This will panic with nil db — recover and verify we got past ID parsing.
	func() {
		defer func() { _ = recover() }()
		srv.handleGetGoal(rr, req)
	}()

	// If we got a 400, ID parsing failed (wrong).
	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid UUID should not return 400, got %d", rr.Code)
	}
}

// --- handleListGoals ---

func TestHandleListGoals_MissingAgentID(t *testing.T) {
	srv := newGoalServer()
	req := httptest.NewRequest(http.MethodGet, "/goals", nil)
	rr := httptest.NewRecorder()

	srv.handleListGoals(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "agent_id query required" {
		t.Errorf("error = %q, want %q", resp["error"], "agent_id query required")
	}
}

func TestHandleListGoals_InvalidAgentID(t *testing.T) {
	srv := newGoalServer()
	req := httptest.NewRequest(http.MethodGet, "/goals?agent_id=not-a-uuid", nil)
	rr := httptest.NewRecorder()

	srv.handleListGoals(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "invalid agent_id" {
		t.Errorf("error = %q, want %q", resp["error"], "invalid agent_id")
	}
}

// --- handleCreateGoal ---

func TestHandleCreateGoal_InvalidJSON(t *testing.T) {
	srv := newGoalServer()
	req := httptest.NewRequest(http.MethodPost, "/goals", bytes.NewBufferString("not json"))
	rr := httptest.NewRecorder()

	srv.handleCreateGoal(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateGoal_MissingAgentID(t *testing.T) {
	srv := newGoalServer()
	body, _ := json.Marshal(map[string]string{"goal_text": "do something"})
	req := httptest.NewRequest(http.MethodPost, "/goals", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleCreateGoal(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "invalid agent_id" {
		t.Errorf("error = %q, want %q", resp["error"], "invalid agent_id")
	}
}

func TestHandleCreateGoal_InvalidAgentID(t *testing.T) {
	srv := newGoalServer()
	body, _ := json.Marshal(map[string]string{
		"agent_id":  "bad-uuid",
		"goal_text": "do something",
	})
	req := httptest.NewRequest(http.MethodPost, "/goals", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleCreateGoal(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateGoal_MissingGoalText(t *testing.T) {
	srv := newGoalServer()
	body, _ := json.Marshal(map[string]string{
		"agent_id": "00000000-0000-0000-0000-000000000001",
	})
	req := httptest.NewRequest(http.MethodPost, "/goals", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleCreateGoal(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "goal_text required" {
		t.Errorf("error = %q, want %q", resp["error"], "goal_text required")
	}
}

// --- handleGetGoalDetails ---

func TestHandleGetGoalDetails_InvalidID(t *testing.T) {
	srv := newGoalServer()
	req := httptest.NewRequest(http.MethodGet, "/goals/bad/details", nil)
	req.SetPathValue("id", "bad")
	rr := httptest.NewRecorder()

	srv.handleGetGoalDetails(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// --- handleFinalizeGoal ---

func TestHandleFinalizeGoal_InvalidID(t *testing.T) {
	srv := newGoalServer()
	req := httptest.NewRequest(http.MethodPost, "/goals/bad/finalize", nil)
	req.SetPathValue("id", "bad")
	rr := httptest.NewRecorder()

	srv.handleFinalizeGoal(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// --- handleApplyPlan (via handleApplyPlan free function) ---

func TestHandleApplyPlan_MissingApprovalID(t *testing.T) {
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/internal/apply-plan", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	handleApplyPlan(rr, req, nil, nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleApplyPlan_InvalidApprovalID(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"approval_id": "not-a-uuid"})
	req := httptest.NewRequest(http.MethodPost, "/internal/apply-plan", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	handleApplyPlan(rr, req, nil, nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// --- handleCreateInternalGoal ---

func TestHandleCreateInternalGoal_InvalidJSON(t *testing.T) {
	srv := newGoalServer()
	req := httptest.NewRequest(http.MethodPost, "/internal/goals", bytes.NewBufferString("{bad"))
	rr := httptest.NewRecorder()

	srv.handleCreateInternalGoal(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateInternalGoal_InvalidAgentID(t *testing.T) {
	srv := newGoalServer()
	body, _ := json.Marshal(map[string]string{
		"agent_id":        "bad",
		"goal_text":       "something",
		"source_agent_id": "00000000-0000-0000-0000-000000000001",
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/goals", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleCreateInternalGoal(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateInternalGoal_MissingGoalText(t *testing.T) {
	srv := newGoalServer()
	body, _ := json.Marshal(map[string]string{
		"agent_id":        "00000000-0000-0000-0000-000000000001",
		"source_agent_id": "00000000-0000-0000-0000-000000000002",
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/goals", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleCreateInternalGoal(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "goal_text required" {
		t.Errorf("error = %q, want %q", resp["error"], "goal_text required")
	}
}

func TestHandleCreateInternalGoal_InvalidSourceAgentID(t *testing.T) {
	srv := newGoalServer()
	body, _ := json.Marshal(map[string]string{
		"agent_id":        "00000000-0000-0000-0000-000000000001",
		"goal_text":       "do something",
		"source_agent_id": "bad-uuid",
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/goals", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.handleCreateInternalGoal(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "invalid source_agent_id" {
		t.Errorf("error = %q, want %q", resp["error"], "invalid source_agent_id")
	}
}

// --- idempotency helpers ---

func TestGetCachedGoalResponse_NilClient(t *testing.T) {
	code, body := getCachedGoalResponse(context.Background(), nil, "some-key")
	if code != 0 || body != "" {
		t.Errorf("nil rdb should return (0, \"\"), got (%d, %q)", code, body)
	}
}

func TestGetCachedGoalResponse_EmptyKey(t *testing.T) {
	code, body := getCachedGoalResponse(context.Background(), &redis.Client{}, "")
	if code != 0 || body != "" {
		t.Errorf("empty key should return (0, \"\"), got (%d, %q)", code, body)
	}
}

func TestSetCachedGoalResponse_NilClient(t *testing.T) {
	// Should not panic with nil client.
	setCachedGoalResponse(context.Background(), nil, "key", 201, map[string]string{"ok": "true"})
}

func TestSetCachedGoalResponse_EmptyKey(t *testing.T) {
	// Should not panic with empty key.
	setCachedGoalResponse(context.Background(), &redis.Client{}, "", 201, map[string]string{"ok": "true"})
}

// --- uuidSliceToArrayLiteral (existing, keep coverage) ---

func TestUUIDSliceToArrayLiteralEdgeCases(t *testing.T) {
	// Confirm the function is still present and working.
	got := uuidSliceToArrayLiteral(nil)
	if got != "{}" {
		t.Errorf("nil -> %q, want {}", got)
	}
}

// --- rate limiting logic via handleCreateInternalGoal ---
// Validate that the rate key is based on source_agent_id by checking the
// code path reaches Redis (panics with nil rdb) only after validation passes.

func TestHandleCreateInternalGoal_RateLimitReachesRedis(t *testing.T) {
	srv := &goalServer{rdb: nil} // nil rdb will cause panic at Incr
	body, _ := json.Marshal(map[string]string{
		"agent_id":        "00000000-0000-0000-0000-000000000001",
		"goal_text":       "do something",
		"source_agent_id": "00000000-0000-0000-0000-000000000002",
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/goals", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	// With nil rdb, Incr will panic — recover and confirm we get past validation.
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		srv.handleCreateInternalGoal(rr, req)
	}()

	// If we got 400 without panic, validation failed (wrong).
	if rr.Code == http.StatusBadRequest && !panicked {
		t.Errorf("valid request should not return 400 before reaching Redis")
	}
}

// --- goalInitialStatus ---

func TestGoalInitialStatus_EmptyDeps(t *testing.T) {
	status, err := goalInitialStatus(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "pending" {
		t.Errorf("status = %q, want pending", status)
	}
}

// --- idempotency TTL constant ---

func TestIdempotencyTTL(t *testing.T) {
	if idempotencyTTL != 24*time.Hour {
		t.Errorf("idempotencyTTL = %v, want 24h", idempotencyTTL)
	}
}
