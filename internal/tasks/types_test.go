package tasks

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestStatusConstants_AllValues verifies every Status constant is non-empty and unique,
// including StatusDeadLetter which was absent from the original test.
func TestStatusConstants_AllValues(t *testing.T) {
	all := []struct {
		name string
		s    Status
	}{
		{"StatusCreated", StatusCreated},
		{"StatusPending", StatusPending},
		{"StatusQueued", StatusQueued},
		{"StatusScheduled", StatusScheduled},
		{"StatusRunning", StatusRunning},
		{"StatusCompleted", StatusCompleted},
		{"StatusFailed", StatusFailed},
		{"StatusDeadLetter", StatusDeadLetter},
	}
	seen := make(map[Status]bool)
	for _, tc := range all {
		t.Run(tc.name, func(t *testing.T) {
			if tc.s == "" {
				t.Errorf("%s is empty string", tc.name)
			}
			if seen[tc.s] {
				t.Errorf("duplicate status value: %q", tc.s)
			}
			seen[tc.s] = true
		})
	}
	if len(seen) != 8 {
		t.Errorf("expected 8 distinct statuses, got %d", len(seen))
	}
}

// TestStatusConstants_StringValues verifies exact string representation of each constant.
func TestStatusConstants_StringValues(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusCreated, "created"},
		{StatusPending, "pending"},
		{StatusQueued, "queued"},
		{StatusScheduled, "scheduled"},
		{StatusRunning, "running"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusDeadLetter, "dead_letter"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if string(tc.status) != tc.want {
				t.Errorf("want %q, got %q", tc.want, string(tc.status))
			}
		})
	}
}

// TestTask_ZeroValue verifies a zero-value Task has expected defaults.
func TestTask_ZeroValue(t *testing.T) {
	var task Task
	if task.ID != uuid.Nil {
		t.Errorf("ID: want uuid.Nil, got %v", task.ID)
	}
	if task.Status != "" {
		t.Errorf("Status: want empty, got %q", task.Status)
	}
	if task.Priority != 0 {
		t.Errorf("Priority: want 0, got %d", task.Priority)
	}
	if task.Retries != 0 {
		t.Errorf("Retries: want 0, got %d", task.Retries)
	}
	if task.MaxRetries != 0 {
		t.Errorf("MaxRetries: want 0, got %d", task.MaxRetries)
	}
	if task.Payload != nil {
		t.Errorf("Payload: want nil, got %v", task.Payload)
	}
	if task.Result != nil {
		t.Errorf("Result: want nil, got %v", task.Result)
	}
}

// TestTask_FieldAssignment verifies all Task fields can be set and retrieved correctly.
func TestTask_FieldAssignment(t *testing.T) {
	id := uuid.New()
	graphID := uuid.New()
	goalID := uuid.New()
	agentID := uuid.New()
	now := time.Now().UTC()

	task := Task{
		ID:         id,
		GraphID:    graphID,
		GoalID:     goalID,
		AgentID:    agentID,
		Type:       "llm_call",
		Status:     StatusRunning,
		Payload:    []byte(`{"prompt":"hello"}`),
		Result:     []byte(`{"output":"world"}`),
		Priority:   5,
		Retries:    1,
		MaxRetries: 3,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if task.ID != id {
		t.Errorf("ID mismatch")
	}
	if task.GraphID != graphID {
		t.Errorf("GraphID mismatch")
	}
	if task.GoalID != goalID {
		t.Errorf("GoalID mismatch")
	}
	if task.AgentID != agentID {
		t.Errorf("AgentID mismatch")
	}
	if task.Type != "llm_call" {
		t.Errorf("Type: want %q, got %q", "llm_call", task.Type)
	}
	if task.Status != StatusRunning {
		t.Errorf("Status: want %q, got %q", StatusRunning, task.Status)
	}
	if task.Priority != 5 {
		t.Errorf("Priority: want 5, got %d", task.Priority)
	}
	if task.Retries != 1 {
		t.Errorf("Retries: want 1, got %d", task.Retries)
	}
	if task.MaxRetries != 3 {
		t.Errorf("MaxRetries: want 3, got %d", task.MaxRetries)
	}
}

// TestDependency_FieldAssignment verifies Dependency fields can be set correctly.
func TestDependency_FieldAssignment(t *testing.T) {
	taskID := uuid.New()
	dependsOn := uuid.New()

	dep := Dependency{
		TaskID:    taskID,
		DependsOn: dependsOn,
	}

	if dep.TaskID != taskID {
		t.Errorf("TaskID mismatch")
	}
	if dep.DependsOn != dependsOn {
		t.Errorf("DependsOn mismatch")
	}
}

// TestDependency_ZeroValue verifies zero-value Dependency has nil UUIDs.
func TestDependency_ZeroValue(t *testing.T) {
	var dep Dependency
	if dep.TaskID != uuid.Nil {
		t.Errorf("TaskID: want uuid.Nil, got %v", dep.TaskID)
	}
	if dep.DependsOn != uuid.Nil {
		t.Errorf("DependsOn: want uuid.Nil, got %v", dep.DependsOn)
	}
}

// TestGraph_WithTasksAndDependencies verifies a populated Graph holds correct data.
func TestGraph_WithTasksAndDependencies(t *testing.T) {
	graphID := uuid.New()
	taskID1 := uuid.New()
	taskID2 := uuid.New()

	graph := Graph{
		ID: graphID,
		Tasks: []Task{
			{ID: taskID1, Status: StatusPending, Type: "step1"},
			{ID: taskID2, Status: StatusCreated, Type: "step2"},
		},
		Dependencies: []Dependency{
			{TaskID: taskID2, DependsOn: taskID1},
		},
	}

	if graph.ID != graphID {
		t.Errorf("ID mismatch")
	}
	if len(graph.Tasks) != 2 {
		t.Errorf("Tasks: want 2, got %d", len(graph.Tasks))
	}
	if len(graph.Dependencies) != 1 {
		t.Errorf("Dependencies: want 1, got %d", len(graph.Dependencies))
	}
	if graph.Dependencies[0].TaskID != taskID2 {
		t.Errorf("Dependency TaskID mismatch")
	}
	if graph.Dependencies[0].DependsOn != taskID1 {
		t.Errorf("Dependency DependsOn mismatch")
	}
}

// TestGraph_TaskJSON verifies task payload JSON bytes are preserved correctly.
func TestGraph_TaskJSON(t *testing.T) {
	payload := json.RawMessage(`{"model":"gpt-4","prompt":"summarize"}`)
	result := json.RawMessage(`{"summary":"short text"}`)

	task := Task{
		ID:      uuid.New(),
		Type:    "llm_call",
		Status:  StatusCompleted,
		Payload: []byte(payload),
		Result:  []byte(result),
	}

	if string(task.Payload) != string(payload) {
		t.Errorf("Payload: want %q, got %q", string(payload), string(task.Payload))
	}
	if string(task.Result) != string(result) {
		t.Errorf("Result: want %q, got %q", string(result), string(task.Result))
	}
}

// TestNullableUUID verifies nullableUUID returns nil for uuid.Nil and the UUID otherwise.
func TestNullableUUID(t *testing.T) {
	tests := []struct {
		name    string
		id      uuid.UUID
		wantNil bool
	}{
		{"nil uuid", uuid.Nil, true},
		{"valid uuid", uuid.New(), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := nullableUUID(tc.id)
			if tc.wantNil && result != nil {
				t.Errorf("want nil, got %v", result)
			}
			if !tc.wantNil && result == nil {
				t.Error("want non-nil, got nil")
			}
		})
	}
}

// TestErrInvalidTransition verifies the sentinel error is non-nil with a useful message.
func TestErrInvalidTransition(t *testing.T) {
	if ErrInvalidTransition == nil {
		t.Fatal("ErrInvalidTransition must not be nil")
	}
	if ErrInvalidTransition.Error() == "" {
		t.Error("ErrInvalidTransition.Error() must not be empty")
	}
}

// TestCacheKeyPrefixes verifies task cache key prefix constants are non-empty and distinct.
func TestCacheKeyPrefixes(t *testing.T) {
	prefixes := []struct {
		name  string
		value string
	}{
		{"taskKeyPrefix", taskKeyPrefix},
		{"graphKeyPrefix", graphKeyPrefix},
	}
	seen := make(map[string]bool)
	for _, p := range prefixes {
		t.Run(p.name, func(t *testing.T) {
			if p.value == "" {
				t.Errorf("%s must not be empty", p.name)
			}
			if seen[p.value] {
				t.Errorf("duplicate cache key prefix: %q", p.value)
			}
			seen[p.value] = true
		})
	}
}
