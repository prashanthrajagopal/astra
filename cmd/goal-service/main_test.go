package main

import (
	"encoding/json"
	"testing"

	"astra/internal/tasks"

	"github.com/google/uuid"
)

func TestBuildPlanPayload(t *testing.T) {
	goalID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	agentID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	graphID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	task1ID := uuid.MustParse("00000000-0000-0000-0000-000000000004")
	task2ID := uuid.MustParse("00000000-0000-0000-0000-000000000005")

	t.Run("empty graph", func(t *testing.T) {
		graph := &tasks.Graph{
			ID:           graphID,
			Tasks:        []tasks.Task{},
			Dependencies: []tasks.Dependency{},
		}
		got := buildPlanPayload(graph, goalID, agentID, "do nothing")
		if got == nil {
			t.Fatal("expected non-nil result")
		}
		if got.GoalID != goalID.String() {
			t.Errorf("GoalID = %q, want %q", got.GoalID, goalID.String())
		}
		if got.GraphID != graphID.String() {
			t.Errorf("GraphID = %q, want %q", got.GraphID, graphID.String())
		}
		if got.AgentID != agentID.String() {
			t.Errorf("AgentID = %q, want %q", got.AgentID, agentID.String())
		}
		if got.GoalText != "do nothing" {
			t.Errorf("GoalText = %q, want %q", got.GoalText, "do nothing")
		}
		if len(got.Tasks) != 0 {
			t.Errorf("Tasks len = %d, want 0", len(got.Tasks))
		}
		if len(got.Dependencies) != 0 {
			t.Errorf("Dependencies len = %d, want 0", len(got.Dependencies))
		}
	})

	t.Run("graph with tasks and dependencies", func(t *testing.T) {
		payload1 := json.RawMessage(`{"cmd":"echo hello"}`)
		graph := &tasks.Graph{
			ID: graphID,
			Tasks: []tasks.Task{
				{
					ID:         task1ID,
					Type:       "shell_exec",
					Payload:    payload1,
					Priority:   5,
					MaxRetries: 3,
				},
				{
					ID:         task2ID,
					Type:       "code_generate",
					Payload:    nil,
					Priority:   1,
					MaxRetries: 0,
				},
			},
			Dependencies: []tasks.Dependency{
				{TaskID: task2ID, DependsOn: task1ID},
			},
		}
		got := buildPlanPayload(graph, goalID, agentID, "run pipeline")

		if len(got.Tasks) != 2 {
			t.Fatalf("Tasks len = %d, want 2", len(got.Tasks))
		}

		// First task
		if got.Tasks[0].ID != task1ID.String() {
			t.Errorf("Tasks[0].ID = %q, want %q", got.Tasks[0].ID, task1ID.String())
		}
		if got.Tasks[0].Type != "shell_exec" {
			t.Errorf("Tasks[0].Type = %q, want %q", got.Tasks[0].Type, "shell_exec")
		}
		if string(got.Tasks[0].Payload) != string(payload1) {
			t.Errorf("Tasks[0].Payload = %q, want %q", got.Tasks[0].Payload, payload1)
		}
		if got.Tasks[0].Priority != 5 {
			t.Errorf("Tasks[0].Priority = %d, want 5", got.Tasks[0].Priority)
		}
		if got.Tasks[0].MaxRetries != 3 {
			t.Errorf("Tasks[0].MaxRetries = %d, want 3", got.Tasks[0].MaxRetries)
		}

		// Second task with nil payload should become "{}"
		if got.Tasks[1].ID != task2ID.String() {
			t.Errorf("Tasks[1].ID = %q, want %q", got.Tasks[1].ID, task2ID.String())
		}
		if string(got.Tasks[1].Payload) != "{}" {
			t.Errorf("Tasks[1].Payload = %q, want %q", got.Tasks[1].Payload, "{}")
		}

		// Dependency
		if len(got.Dependencies) != 1 {
			t.Fatalf("Dependencies len = %d, want 1", len(got.Dependencies))
		}
		if got.Dependencies[0].TaskID != task2ID.String() {
			t.Errorf("Dependencies[0].TaskID = %q, want %q", got.Dependencies[0].TaskID, task2ID.String())
		}
		if got.Dependencies[0].DependsOn != task1ID.String() {
			t.Errorf("Dependencies[0].DependsOn = %q, want %q", got.Dependencies[0].DependsOn, task1ID.String())
		}
	})

	t.Run("nil task payload becomes empty object", func(t *testing.T) {
		graph := &tasks.Graph{
			ID: graphID,
			Tasks: []tasks.Task{
				{ID: task1ID, Type: "noop", Payload: nil},
			},
			Dependencies: nil,
		}
		got := buildPlanPayload(graph, goalID, agentID, "test")
		if string(got.Tasks[0].Payload) != "{}" {
			t.Errorf("nil payload should become {}, got %q", got.Tasks[0].Payload)
		}
	})
}

func TestUUIDSliceToArrayLiteral(t *testing.T) {
	tests := []struct {
		name string
		ids  []uuid.UUID
		want string
	}{
		{
			name: "empty slice",
			ids:  []uuid.UUID{},
			want: "{}",
		},
		{
			name: "nil slice",
			ids:  nil,
			want: "{}",
		},
		{
			name: "single id",
			ids:  []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-000000000001")},
			want: "{00000000-0000-0000-0000-000000000001}",
		},
		{
			name: "multiple ids",
			ids: []uuid.UUID{
				uuid.MustParse("00000000-0000-0000-0000-000000000001"),
				uuid.MustParse("00000000-0000-0000-0000-000000000002"),
				uuid.MustParse("00000000-0000-0000-0000-000000000003"),
			},
			want: "{00000000-0000-0000-0000-000000000001,00000000-0000-0000-0000-000000000002,00000000-0000-0000-0000-000000000003}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uuidSliceToArrayLiteral(tt.ids)
			if got != tt.want {
				t.Errorf("uuidSliceToArrayLiteral() = %q, want %q", got, tt.want)
			}
		})
	}
}
