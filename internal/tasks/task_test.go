package tasks

import (
	"testing"
)

func TestStatusConstants(t *testing.T) {
	statuses := []Status{
		StatusCreated, StatusPending, StatusQueued, StatusScheduled,
		StatusRunning, StatusCompleted, StatusFailed,
	}
	seen := make(map[Status]bool)
	for _, s := range statuses {
		if s == "" {
			t.Errorf("status constant is empty string")
		}
		if seen[s] {
			t.Errorf("duplicate status value: %q", s)
		}
		seen[s] = true
	}
	if len(seen) != 7 {
		t.Errorf("expected 7 distinct statuses, got %d", len(seen))
	}
}

func TestGraph_Empty(t *testing.T) {
	var g Graph
	if g.Tasks != nil {
		t.Errorf("empty Graph.Tasks should be nil, got %v", g.Tasks)
	}
	if g.Dependencies != nil {
		t.Errorf("empty Graph.Dependencies should be nil, got %v", g.Dependencies)
	}
}
