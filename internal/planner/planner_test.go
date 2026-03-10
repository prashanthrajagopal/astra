package planner

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestPlanner_Plan(t *testing.T) {
	p := New()
	goalID := uuid.New()
	agentID := uuid.New()
	goalText := "implement feature X"
	g, err := p.Plan(context.Background(), goalID, goalText, agentID, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(g.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(g.Tasks))
	}

	// Verify no agent_context in payload when opts is nil
	for _, task := range g.Tasks {
		var m map[string]interface{}
		_ = json.Unmarshal(task.Payload, &m)
		if _, ok := m["agent_context"]; ok {
			t.Error("expected no agent_context in payload when opts is nil")
		}
	}
	if len(g.Dependencies) != 1 {
		t.Errorf("expected 1 dependency (implement -> analyze), got %d", len(g.Dependencies))
	}
	if g.ID == uuid.Nil {
		t.Error("expected non-nil Graph ID")
	}
	for _, task := range g.Tasks {
		if task.ID == uuid.Nil {
			t.Errorf("task has nil ID")
		}
		if task.GraphID != g.ID {
			t.Errorf("task GraphID %s != graph ID %s", task.GraphID, g.ID)
		}
		if task.GoalID != goalID {
			t.Errorf("task GoalID %s != goalID %s", task.GoalID, goalID)
		}
		if task.AgentID != agentID {
			t.Errorf("task AgentID %s != agentID %s", task.AgentID, agentID)
		}
		if task.Type != "code_generate" {
			t.Errorf("expected task type code_generate, got %s", task.Type)
		}
		if task.Payload == nil {
			t.Errorf("expected non-nil Payload")
		}
	}
}

func TestPlanner_Plan_AgentContextPropagation(t *testing.T) {
	p := New()
	goalID := uuid.New()
	agentID := uuid.New()
	goalText := "implement feature X"
	agentCtx := json.RawMessage(`{"system_prompt":"you are expert","rules":[],"skills":[],"context_docs":[]}`)
	opts := &PlanOptions{Workspace: "/test", AgentContext: agentCtx}
	g, err := p.Plan(context.Background(), goalID, goalText, agentID, opts)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, task := range g.Tasks {
		var m map[string]interface{}
		if err := json.Unmarshal(task.Payload, &m); err != nil {
			t.Fatalf("Unmarshal payload: %v", err)
		}
		ac, ok := m["agent_context"]
		if !ok || ac == nil {
			t.Error("expected agent_context in payload when opts.AgentContext is set")
		}
	}
}
