package planner

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestPlanner_Plan(t *testing.T) {
	p := New()
	goalID := uuid.New()
	agentID := uuid.New()
	goalText := "implement feature X"
	g, err := p.Plan(context.Background(), goalID, goalText, agentID)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(g.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(g.Tasks))
	}
	if len(g.Dependencies) != 1 {
		t.Errorf("expected 1 dependency (implement -> analyze), got %d", len(g.Dependencies))
	}
	if g.ID == uuid.Nil {
		t.Error("expected non-nil Graph ID")
	}
	types := make(map[string]bool)
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
		types[task.Type] = true
	}
	if !types["analyze"] || !types["implement"] {
		t.Errorf("expected task types analyze and implement, got %v", types)
	}
	_ = goalText // used to satisfy compiler
}
