package planner

import (
	"testing"

	"github.com/google/uuid"
)

func TestPlanner_Plan(t *testing.T) {
	p := New()
	goalID := uuid.New()
	goalText := "implement feature X"
	g := p.Plan(goalID, goalText)

	if len(g.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(g.Tasks))
	}
	if g.Dependencies != nil && len(g.Dependencies) != 0 {
		t.Errorf("expected no dependencies, got %d", len(g.Dependencies))
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
		types[task.Type] = true
	}
	if !types["analyze"] || !types["implement"] {
		t.Errorf("expected task types analyze and implement, got %v", types)
	}
	_ = goalText // used to satisfy compiler
}
