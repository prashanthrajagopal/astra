package agent

import (
	"testing"
	"time"

	"astra/internal/actors"
	"astra/internal/kernel"
	"astra/internal/planner"

	"github.com/google/uuid"
)

func newTestKernel() *kernel.Kernel {
	return kernel.New()
}

func TestNew_ReturnsAgent(t *testing.T) {
	k := newTestKernel()
	p := planner.New()
	a := New("test-agent", k, p, nil, nil)
	if a == nil {
		t.Fatal("New returned nil")
	}
	// Clean up actor goroutine.
	_ = a.Stop()
}

func TestNew_IDIsValidUUID(t *testing.T) {
	k := newTestKernel()
	p := planner.New()
	a := New("test-agent", k, p, nil, nil)
	defer func() { _ = a.Stop() }()

	if a.ID == uuid.Nil {
		t.Error("agent ID is nil UUID")
	}
	parsed, err := uuid.Parse(a.ID.String())
	if err != nil {
		t.Errorf("agent ID is not a valid UUID: %v", err)
	}
	if parsed != a.ID {
		t.Errorf("parsed UUID %v != original ID %v", parsed, a.ID)
	}
}

func TestNew_NameIsSet(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"my-agent"},
		{"production-agent-1"},
		{""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k := newTestKernel()
			p := planner.New()
			a := New(tc.name, k, p, nil, nil)
			defer func() { _ = a.Stop() }()
			if a.Name != tc.name {
				t.Errorf("Name = %q, want %q", a.Name, tc.name)
			}
		})
	}
}

func TestNew_StatusDefaultsToActive(t *testing.T) {
	k := newTestKernel()
	p := planner.New()
	a := New("agent-status-test", k, p, nil, nil)
	defer func() { _ = a.Stop() }()

	if a.Status != "active" {
		t.Errorf("Status = %q, want active", a.Status)
	}
}

func TestNew_UniqueIDs(t *testing.T) {
	k := newTestKernel()
	p := planner.New()
	a1 := New("agent-1", k, p, nil, nil)
	a2 := New("agent-2", k, p, nil, nil)
	defer func() { _ = a1.Stop() }()
	defer func() { _ = a2.Stop() }()

	if a1.ID == a2.ID {
		t.Errorf("two agents have the same ID: %v", a1.ID)
	}
}

func TestNewFromExisting_UsesProvidedID(t *testing.T) {
	k := newTestKernel()
	p := planner.New()
	id := uuid.New()
	a := NewFromExisting(id, "restored-agent", k, p, nil, nil)
	defer func() { _ = a.Stop() }()

	if a.ID != id {
		t.Errorf("ID = %v, want %v", a.ID, id)
	}
	if a.Name != "restored-agent" {
		t.Errorf("Name = %q, want restored-agent", a.Name)
	}
}

func TestStop_SetsStatusStopped(t *testing.T) {
	k := newTestKernel()
	p := planner.New()
	a := New("stop-test", k, p, nil, nil)

	if a.Status != "active" {
		t.Errorf("initial Status = %q, want active", a.Status)
	}
	_ = a.Stop()
	if a.Status != "stopped" {
		t.Errorf("post-Stop Status = %q, want stopped", a.Status)
	}
}

func TestWithSupervisor_Option(t *testing.T) {
	k := newTestKernel()
	p := planner.New()
	sup := actors.NewSupervisor(actors.RestartImmediate, 3, 60*time.Second)
	terminated := false
	onTerm := func(id string) { terminated = true }

	a := New("supervised-agent", k, p, nil, nil, WithSupervisor(sup, onTerm))
	defer func() { _ = a.Stop() }()

	if a.supervisor == nil {
		t.Error("supervisor not set")
	}
	if a.onTerminate == nil {
		t.Error("onTerminate not set")
	}
	_ = terminated // referenced to avoid unused-variable error
}
