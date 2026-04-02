package adapters

import (
	"encoding/json"
	"testing"
)

func TestStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		status   Status
		expected string
	}{
		{"pending", StatusPending, "pending"},
		{"running", StatusRunning, "running"},
		{"completed", StatusCompleted, "completed"},
		{"failed", StatusFailed, "failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.status) != tc.expected {
				t.Errorf("Status %q: got %q, want %q", tc.name, tc.status, tc.expected)
			}
		})
	}
}

func TestGoalContextJSONMarshal(t *testing.T) {
	tests := []struct {
		name    string
		gc      GoalContext
		wantErr bool
	}{
		{
			name: "full fields",
			gc: GoalContext{
				GoalID:   "goal-1",
				GoalText: "do something",
				AgentID:  "agent-1",
				Priority: 5,
				Metadata: json.RawMessage(`{"key":"val"}`),
			},
		},
		{
			name: "empty metadata omitted",
			gc: GoalContext{
				GoalID:   "goal-2",
				GoalText: "another task",
				AgentID:  "agent-2",
				Priority: 0,
			},
		},
		{
			name: "zero value",
			gc:   GoalContext{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.gc)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Marshal error = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			var got GoalContext
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if got.GoalID != tc.gc.GoalID {
				t.Errorf("GoalID: got %q, want %q", got.GoalID, tc.gc.GoalID)
			}
			if got.GoalText != tc.gc.GoalText {
				t.Errorf("GoalText: got %q, want %q", got.GoalText, tc.gc.GoalText)
			}
			if got.AgentID != tc.gc.AgentID {
				t.Errorf("AgentID: got %q, want %q", got.AgentID, tc.gc.AgentID)
			}
			if got.Priority != tc.gc.Priority {
				t.Errorf("Priority: got %d, want %d", got.Priority, tc.gc.Priority)
			}
		})
	}
}

func TestJobResultJSONMarshal(t *testing.T) {
	tests := []struct {
		name string
		jr   JobResult
	}{
		{
			name: "completed with output",
			jr: JobResult{
				Status: StatusCompleted,
				Output: json.RawMessage(`{"result":"ok"}`),
			},
		},
		{
			name: "failed with error",
			jr: JobResult{
				Status: StatusFailed,
				Error:  "something went wrong",
			},
		},
		{
			name: "pending no output",
			jr: JobResult{
				Status: StatusPending,
			},
		},
		{
			name: "running no error",
			jr: JobResult{
				Status: StatusRunning,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.jr)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			var got JobResult
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if got.Status != tc.jr.Status {
				t.Errorf("Status: got %q, want %q", got.Status, tc.jr.Status)
			}
			if got.Error != tc.jr.Error {
				t.Errorf("Error: got %q, want %q", got.Error, tc.jr.Error)
			}
		})
	}
}

func TestCapabilityStruct(t *testing.T) {
	tests := []struct {
		name string
		cap  Capability
	}{
		{
			name: "full capability",
			cap:  Capability{Name: "code-gen", Description: "generates code", Version: "1.0"},
		},
		{
			name: "empty capability",
			cap:  Capability{},
		},
		{
			name: "name only",
			cap:  Capability{Name: "search"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.cap)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			var got Capability
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if got.Name != tc.cap.Name {
				t.Errorf("Name: got %q, want %q", got.Name, tc.cap.Name)
			}
			if got.Description != tc.cap.Description {
				t.Errorf("Description: got %q, want %q", got.Description, tc.cap.Description)
			}
			if got.Version != tc.cap.Version {
				t.Errorf("Version: got %q, want %q", got.Version, tc.cap.Version)
			}
		})
	}
}
