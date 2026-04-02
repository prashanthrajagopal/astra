package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"astra/internal/planner"

	"github.com/google/uuid"
)

func newTestMux() http.Handler {
	p := planner.New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /plan", func(w http.ResponseWriter, r *http.Request) {
		var req planRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		goalID, err := uuid.Parse(req.GoalID)
		if err != nil {
			http.Error(w, `{"error":"invalid goal_id"}`, http.StatusBadRequest)
			return
		}
		agentID, err := uuid.Parse(req.AgentID)
		if err != nil {
			http.Error(w, `{"error":"invalid agent_id"}`, http.StatusBadRequest)
			return
		}
		graph, err := p.Plan(r.Context(), goalID, req.GoalText, agentID, nil)
		if err != nil {
			http.Error(w, `{"error":"plan failed"}`, http.StatusInternalServerError)
			return
		}
		tasksJSON := make([]map[string]interface{}, len(graph.Tasks))
		for i, t := range graph.Tasks {
			tasksJSON[i] = map[string]interface{}{
				"id":       t.ID.String(),
				"graph_id": t.GraphID.String(),
				"goal_id":  t.GoalID.String(),
				"agent_id": t.AgentID.String(),
				"type":     t.Type,
				"status":   string(t.Status),
			}
		}
		resp := map[string]interface{}{
			"graph_id": graph.ID.String(),
			"tasks":    tasksJSON,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	return mux
}

func TestPlannerHealth(t *testing.T) {
	mux := newTestMux()
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

func TestPlanHandler_InvalidJSON(t *testing.T) {
	mux := newTestMux()
	req := httptest.NewRequest(http.MethodPost, "/plan", strings.NewReader("{bad"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestPlanHandler_InvalidGoalID(t *testing.T) {
	mux := newTestMux()
	body, _ := json.Marshal(planRequest{
		GoalID:   "not-a-uuid",
		AgentID:  uuid.New().String(),
		GoalText: "do something",
	})
	req := httptest.NewRequest(http.MethodPost, "/plan", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "goal_id") {
		t.Errorf("expected error mentioning goal_id, got: %s", rec.Body.String())
	}
}

func TestPlanHandler_InvalidAgentID(t *testing.T) {
	mux := newTestMux()
	body, _ := json.Marshal(planRequest{
		GoalID:   uuid.New().String(),
		AgentID:  "bad-agent-id",
		GoalText: "do something",
	})
	req := httptest.NewRequest(http.MethodPost, "/plan", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "agent_id") {
		t.Errorf("expected error mentioning agent_id, got: %s", rec.Body.String())
	}
}

func TestPlanHandler_ValidRequest_FallbackGraph(t *testing.T) {
	mux := newTestMux()
	goalID := uuid.New().String()
	agentID := uuid.New().String()
	body, _ := json.Marshal(planRequest{
		GoalID:   goalID,
		AgentID:  agentID,
		GoalText: "build a hello world app",
	})
	req := httptest.NewRequest(http.MethodPost, "/plan", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["graph_id"] == "" || resp["graph_id"] == nil {
		t.Error("expected non-empty graph_id")
	}
	tasks, ok := resp["tasks"].([]interface{})
	if !ok || len(tasks) == 0 {
		t.Error("expected non-empty tasks array")
	}
}

func TestPlanHandler_EmptyGoalText(t *testing.T) {
	mux := newTestMux()
	body, _ := json.Marshal(planRequest{
		GoalID:   uuid.New().String(),
		AgentID:  uuid.New().String(),
		GoalText: "",
	})
	req := httptest.NewRequest(http.MethodPost, "/plan", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	// Empty goal text still produces fallback graph (no LLM router configured)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestPlanRequest_Struct(t *testing.T) {
	pr := planRequest{
		GoalID:   "g1",
		AgentID:  "a1",
		GoalText: "do X",
	}
	b, err := json.Marshal(pr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var pr2 planRequest
	if err := json.Unmarshal(b, &pr2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if pr2.GoalID != "g1" || pr2.AgentID != "a1" || pr2.GoalText != "do X" {
		t.Errorf("round-trip failed: %+v", pr2)
	}
}
