package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostGoalRequest_Serialization(t *testing.T) {
	req := PostGoalRequest{
		TargetAgentID:    "agent-42",
		GoalText:         "do something",
		Priority:         5,
		CascadeID:        "cascade-1",
		DependsOnGoalIDs: []string{"goal-1", "goal-2"},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["agent_id"] != "agent-42" {
		t.Errorf("agent_id: got %v", m["agent_id"])
	}
	if m["goal_text"] != "do something" {
		t.Errorf("goal_text: got %v", m["goal_text"])
	}
	if m["cascade_id"] != "cascade-1" {
		t.Errorf("cascade_id: got %v", m["cascade_id"])
	}
}

func TestPostGoalRequest_OmitEmpty(t *testing.T) {
	req := PostGoalRequest{
		TargetAgentID: "agent-1",
		GoalText:      "hello",
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	if _, ok := m["cascade_id"]; ok {
		t.Error("cascade_id should be omitted when empty")
	}
	if _, ok := m["depends_on_goal_ids"]; ok {
		t.Error("depends_on_goal_ids should be omitted when nil")
	}
	// priority 0 is omitempty so should be absent
	if _, ok := m["priority"]; ok {
		t.Error("priority 0 should be omitted")
	}
}

func TestPostGoalResponse_Deserialization(t *testing.T) {
	raw := `{"goal_id":"g-1","approval_request_id":"apr-1","status":"pending"}`
	var resp PostGoalResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.GoalID != "g-1" {
		t.Errorf("GoalID: got %q", resp.GoalID)
	}
	if resp.ApprovalRequestID != "apr-1" {
		t.Errorf("ApprovalRequestID: got %q", resp.ApprovalRequestID)
	}
	if resp.Status != "pending" {
		t.Errorf("Status: got %q", resp.Status)
	}
}

func TestGoalStatus_Deserialization(t *testing.T) {
	raw := `{
		"id":"gs-1","agent_id":"agent-1","goal_text":"do it","priority":10,
		"status":"running","cascade_id":"c-1","source_agent_id":"src-1",
		"created_at":"2025-01-01T00:00:00Z","completed_at":"2025-01-02T00:00:00Z",
		"depends_on_goal_ids":["g-0"]
	}`
	var gs GoalStatus
	if err := json.Unmarshal([]byte(raw), &gs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if gs.ID != "gs-1" {
		t.Errorf("ID: %q", gs.ID)
	}
	if gs.AgentID != "agent-1" {
		t.Errorf("AgentID: %q", gs.AgentID)
	}
	if gs.Priority != 10 {
		t.Errorf("Priority: %d", gs.Priority)
	}
	if gs.SourceAgentID != "src-1" {
		t.Errorf("SourceAgentID: %q", gs.SourceAgentID)
	}
	if len(gs.DependsOnGoalIDs) != 1 || gs.DependsOnGoalIDs[0] != "g-0" {
		t.Errorf("DependsOnGoalIDs: %v", gs.DependsOnGoalIDs)
	}
}

func TestPostGoal_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/internal/goals" {
			t.Errorf("expected /internal/goals, got %s", r.URL.Path)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["goal_text"] != "accomplish task" {
			t.Errorf("goal_text: got %v", body["goal_text"])
		}
		// priority defaults to 100 when 0
		if body["priority"].(float64) != 100 {
			t.Errorf("priority: got %v", body["priority"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PostGoalResponse{
			GoalID: "g-99",
			Status: "pending",
		})
	}))
	defer server.Close()

	c := NewHTTPClient(server.URL, "caller-agent", "tok")
	resp, err := c.PostGoal(context.Background(), PostGoalRequest{
		TargetAgentID: "target-agent",
		GoalText:      "accomplish task",
	})
	if err != nil {
		t.Fatalf("PostGoal: %v", err)
	}
	if resp.GoalID != "g-99" {
		t.Errorf("GoalID: got %q", resp.GoalID)
	}
	if resp.Status != "pending" {
		t.Errorf("Status: got %q", resp.Status)
	}
}

func TestPostGoal_SetsSourceAgentID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["source_agent_id"] != "my-agent" {
			t.Errorf("source_agent_id: got %v", body["source_agent_id"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PostGoalResponse{GoalID: "g-1", Status: "pending"})
	}))
	defer server.Close()

	c := NewHTTPClient(server.URL, "my-agent", "tok")
	_, err := c.PostGoal(context.Background(), PostGoalRequest{
		TargetAgentID: "target",
		GoalText:      "test",
	})
	if err != nil {
		t.Fatalf("PostGoal: %v", err)
	}
}

func TestPostGoal_DefaultPriority(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["priority"].(float64) != 100 {
			t.Errorf("expected default priority 100, got %v", body["priority"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PostGoalResponse{GoalID: "g-1", Status: "pending"})
	}))
	defer server.Close()

	c := NewHTTPClient(server.URL, "agent", "tok")
	_, err := c.PostGoal(context.Background(), PostGoalRequest{GoalText: "test", Priority: 0})
	if err != nil {
		t.Fatalf("PostGoal: %v", err)
	}
}

func TestPostGoal_ExplicitPriority(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["priority"].(float64) != 50 {
			t.Errorf("expected priority 50, got %v", body["priority"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PostGoalResponse{GoalID: "g-1", Status: "pending"})
	}))
	defer server.Close()

	c := NewHTTPClient(server.URL, "agent", "tok")
	_, err := c.PostGoal(context.Background(), PostGoalRequest{GoalText: "test", Priority: 50})
	if err != nil {
		t.Fatalf("PostGoal: %v", err)
	}
}

func TestPostGoal_ErrorResponse(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"400_with_message", 400, `{"error":"bad request"}`},
		{"500_with_message", 500, `{"error":"internal error"}`},
		{"404_empty_error", 404, `{"error":""}`},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			c := NewHTTPClient(server.URL, "agent", "tok")
			resp, err := c.PostGoal(context.Background(), PostGoalRequest{GoalText: "test"})
			if err == nil {
				t.Errorf("expected error for status %d, got nil (resp=%+v)", tc.status, resp)
			}
		})
	}
}

func TestGetGoal_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/goals/goal-123" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GoalStatus{
			ID:      "goal-123",
			AgentID: "agent-1",
			Status:  "completed",
		})
	}))
	defer server.Close()

	c := NewHTTPClient(server.URL, "agent", "tok")
	gs, err := c.GetGoal(context.Background(), "goal-123")
	if err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	if gs.ID != "goal-123" {
		t.Errorf("ID: got %q", gs.ID)
	}
	if gs.Status != "completed" {
		t.Errorf("Status: got %q", gs.Status)
	}
}

func TestGetGoal_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer server.Close()

	c := NewHTTPClient(server.URL, "agent", "tok")
	_, err := c.GetGoal(context.Background(), "missing-goal")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestPostGoal_AuthorizationHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer mytoken" {
			t.Errorf("Authorization header: got %q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PostGoalResponse{GoalID: "g-1", Status: "pending"})
	}))
	defer server.Close()

	c := NewHTTPClient(server.URL, "agent", "mytoken")
	_, err := c.PostGoal(context.Background(), PostGoalRequest{GoalText: "test"})
	if err != nil {
		t.Fatalf("PostGoal: %v", err)
	}
}

func TestPostGoal_CascadeAndDependencies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["cascade_id"] != "cas-1" {
			t.Errorf("cascade_id: got %v", body["cascade_id"])
		}
		deps, ok := body["depends_on_goal_ids"].([]interface{})
		if !ok || len(deps) != 2 {
			t.Errorf("depends_on_goal_ids: got %v", body["depends_on_goal_ids"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PostGoalResponse{GoalID: "g-1", Status: "pending"})
	}))
	defer server.Close()

	c := NewHTTPClient(server.URL, "agent", "tok")
	_, err := c.PostGoal(context.Background(), PostGoalRequest{
		GoalText:         "test",
		CascadeID:        "cas-1",
		DependsOnGoalIDs: []string{"g-a", "g-b"},
	})
	if err != nil {
		t.Fatalf("PostGoal: %v", err)
	}
}
