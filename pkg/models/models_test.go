package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestAgent_FieldAssignment verifies all Agent fields including newer ones can be set.
func TestAgent_FieldAssignment(t *testing.T) {
	id := uuid.New()
	now := time.Now().UTC()

	agent := Agent{
		ID:           id,
		Name:         "my-agent",
		Status:       "active",
		Config:       []byte(`{"model":"gpt-4"}`),
		TrustScore:   0.95,
		Tags:         []string{"production", "llm"},
		Metadata:     json.RawMessage(`{"region":"us-east-1"}`),
		SystemPrompt: "you are a helpful assistant",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if agent.ID != id {
		t.Errorf("ID mismatch")
	}
	if agent.Name != "my-agent" {
		t.Errorf("Name: want %q, got %q", "my-agent", agent.Name)
	}
	if agent.Status != "active" {
		t.Errorf("Status: want %q, got %q", "active", agent.Status)
	}
	if agent.TrustScore != 0.95 {
		t.Errorf("TrustScore: want 0.95, got %f", agent.TrustScore)
	}
	if len(agent.Tags) != 2 {
		t.Errorf("Tags: want 2, got %d", len(agent.Tags))
	}
	if agent.Tags[0] != "production" || agent.Tags[1] != "llm" {
		t.Errorf("Tags: unexpected values %v", agent.Tags)
	}
	if agent.SystemPrompt != "you are a helpful assistant" {
		t.Errorf("SystemPrompt: want %q, got %q", "you are a helpful assistant", agent.SystemPrompt)
	}
	if string(agent.Metadata) != `{"region":"us-east-1"}` {
		t.Errorf("Metadata: want %q, got %q", `{"region":"us-east-1"}`, string(agent.Metadata))
	}
}

// TestAgent_ZeroValue verifies a zero-value Agent has safe defaults.
func TestAgent_ZeroValue(t *testing.T) {
	var a Agent
	if a.ID != uuid.Nil {
		t.Errorf("ID: want uuid.Nil, got %v", a.ID)
	}
	if a.Name != "" {
		t.Errorf("Name: want empty, got %q", a.Name)
	}
	if a.TrustScore != 0 {
		t.Errorf("TrustScore: want 0, got %f", a.TrustScore)
	}
	if a.Tags != nil {
		t.Errorf("Tags: want nil, got %v", a.Tags)
	}
}

// TestGoal_FieldAssignment verifies all Goal fields including newer ones.
func TestGoal_FieldAssignment(t *testing.T) {
	id := uuid.New()
	agentID := uuid.New()
	cascadeID := uuid.New()
	sourceAgentID := uuid.New()
	dep1 := uuid.New()
	dep2 := uuid.New()
	now := time.Now().UTC()
	completedAt := now.Add(-time.Minute)

	goal := Goal{
		ID:               id,
		AgentID:          agentID,
		GoalText:         "build the product",
		Priority:         10,
		Status:           "active",
		CascadeID:        &cascadeID,
		DependsOnGoalIDs: []uuid.UUID{dep1, dep2},
		CompletedAt:      &completedAt,
		SourceAgentID:    &sourceAgentID,
		CreatedAt:        now,
	}

	if goal.ID != id {
		t.Errorf("ID mismatch")
	}
	if goal.AgentID != agentID {
		t.Errorf("AgentID mismatch")
	}
	if goal.GoalText != "build the product" {
		t.Errorf("GoalText: want %q, got %q", "build the product", goal.GoalText)
	}
	if goal.Priority != 10 {
		t.Errorf("Priority: want 10, got %d", goal.Priority)
	}
	if goal.Status != "active" {
		t.Errorf("Status: want %q, got %q", "active", goal.Status)
	}
	if goal.CascadeID == nil || *goal.CascadeID != cascadeID {
		t.Errorf("CascadeID: want %v, got %v", cascadeID, goal.CascadeID)
	}
	if len(goal.DependsOnGoalIDs) != 2 {
		t.Errorf("DependsOnGoalIDs: want 2, got %d", len(goal.DependsOnGoalIDs))
	}
	if goal.CompletedAt == nil || *goal.CompletedAt != completedAt {
		t.Errorf("CompletedAt: want %v, got %v", completedAt, goal.CompletedAt)
	}
	if goal.SourceAgentID == nil || *goal.SourceAgentID != sourceAgentID {
		t.Errorf("SourceAgentID: want %v, got %v", sourceAgentID, goal.SourceAgentID)
	}
}

// TestGoal_NilOptionals verifies nil pointer fields on Goal are safe to inspect.
func TestGoal_NilOptionals(t *testing.T) {
	goal := Goal{
		ID:      uuid.New(),
		AgentID: uuid.New(),
		Status:  "pending",
	}

	if goal.CascadeID != nil {
		t.Errorf("CascadeID: want nil, got %v", goal.CascadeID)
	}
	if goal.CompletedAt != nil {
		t.Errorf("CompletedAt: want nil, got %v", goal.CompletedAt)
	}
	if goal.SourceAgentID != nil {
		t.Errorf("SourceAgentID: want nil, got %v", goal.SourceAgentID)
	}
	if goal.DependsOnGoalIDs != nil {
		t.Errorf("DependsOnGoalIDs: want nil, got %v", goal.DependsOnGoalIDs)
	}
}

// TestEvent_FieldAssignment verifies Event fields are assignable correctly.
func TestEvent_FieldAssignment(t *testing.T) {
	actorID := uuid.New()
	now := time.Now().UTC()

	event := Event{
		ID:        42,
		EventType: "TaskCompleted",
		ActorID:   actorID,
		Payload:   []byte(`{"task_id":"abc"}`),
		CreatedAt: now,
	}

	if event.ID != 42 {
		t.Errorf("ID: want 42, got %d", event.ID)
	}
	if event.EventType != "TaskCompleted" {
		t.Errorf("EventType: want %q, got %q", "TaskCompleted", event.EventType)
	}
	if event.ActorID != actorID {
		t.Errorf("ActorID mismatch")
	}
	if string(event.Payload) != `{"task_id":"abc"}` {
		t.Errorf("Payload: unexpected %q", string(event.Payload))
	}
}

// TestEvent_ZeroValue verifies zero-value Event has safe defaults.
func TestEvent_ZeroValue(t *testing.T) {
	var e Event
	if e.ID != 0 {
		t.Errorf("ID: want 0, got %d", e.ID)
	}
	if e.EventType != "" {
		t.Errorf("EventType: want empty, got %q", e.EventType)
	}
	if e.ActorID != uuid.Nil {
		t.Errorf("ActorID: want uuid.Nil, got %v", e.ActorID)
	}
}

// TestWorker_FieldAssignment verifies all Worker fields can be set.
func TestWorker_FieldAssignment(t *testing.T) {
	id := uuid.New()
	now := time.Now().UTC()

	worker := Worker{
		ID:            id,
		Hostname:      "worker-01.us-east-1",
		Status:        "online",
		Capabilities:  []byte(`["llm","tool_execution"]`),
		LastHeartbeat: now,
		CreatedAt:     now,
	}

	if worker.ID != id {
		t.Errorf("ID mismatch")
	}
	if worker.Hostname != "worker-01.us-east-1" {
		t.Errorf("Hostname: want %q, got %q", "worker-01.us-east-1", worker.Hostname)
	}
	if worker.Status != "online" {
		t.Errorf("Status: want %q, got %q", "online", worker.Status)
	}
	if string(worker.Capabilities) != `["llm","tool_execution"]` {
		t.Errorf("Capabilities: unexpected %q", string(worker.Capabilities))
	}
}

// TestWorker_ZeroValue verifies zero-value Worker has safe defaults.
func TestWorker_ZeroValue(t *testing.T) {
	var w Worker
	if w.ID != uuid.Nil {
		t.Errorf("ID: want uuid.Nil, got %v", w.ID)
	}
	if w.Status != "" {
		t.Errorf("Status: want empty, got %q", w.Status)
	}
}

// TestArtifact_FieldAssignment verifies all Artifact fields can be set.
func TestArtifact_FieldAssignment(t *testing.T) {
	id := uuid.New()
	agentID := uuid.New()
	taskID := uuid.New()
	now := time.Now().UTC()

	artifact := Artifact{
		ID:        id,
		AgentID:   agentID,
		TaskID:    taskID,
		URI:       "s3://my-bucket/artifacts/result.json",
		Metadata:  []byte(`{"size":1024,"mime":"application/json"}`),
		CreatedAt: now,
	}

	if artifact.ID != id {
		t.Errorf("ID mismatch")
	}
	if artifact.AgentID != agentID {
		t.Errorf("AgentID mismatch")
	}
	if artifact.TaskID != taskID {
		t.Errorf("TaskID mismatch")
	}
	if artifact.URI != "s3://my-bucket/artifacts/result.json" {
		t.Errorf("URI: want %q, got %q", "s3://my-bucket/artifacts/result.json", artifact.URI)
	}
	if string(artifact.Metadata) != `{"size":1024,"mime":"application/json"}` {
		t.Errorf("Metadata: unexpected %q", string(artifact.Metadata))
	}
}

// TestArtifact_ZeroValue verifies zero-value Artifact has safe defaults.
func TestArtifact_ZeroValue(t *testing.T) {
	var a Artifact
	if a.ID != uuid.Nil {
		t.Errorf("ID: want uuid.Nil, got %v", a.ID)
	}
	if a.URI != "" {
		t.Errorf("URI: want empty, got %q", a.URI)
	}
}

// TestAgent_MetadataJSON verifies Metadata field stores valid JSON correctly.
func TestAgent_MetadataJSON(t *testing.T) {
	type agentMeta struct {
		Region string `json:"region"`
		Tier   int    `json:"tier"`
	}
	meta := agentMeta{Region: "eu-west-1", Tier: 2}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	agent := Agent{
		ID:       uuid.New(),
		Name:     "eu-agent",
		Metadata: json.RawMessage(raw),
	}

	var got agentMeta
	if err := json.Unmarshal(agent.Metadata, &got); err != nil {
		t.Fatalf("json.Unmarshal Metadata: %v", err)
	}
	if got.Region != "eu-west-1" {
		t.Errorf("Region: want %q, got %q", "eu-west-1", got.Region)
	}
	if got.Tier != 2 {
		t.Errorf("Tier: want 2, got %d", got.Tier)
	}
}
