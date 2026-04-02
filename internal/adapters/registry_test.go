package adapters

import (
	"context"
	"encoding/json"
	"testing"
)

// mockAdapter is a minimal Adapter implementation for tests.
type mockAdapter struct {
	name    string
	healthy bool
}

func (m *mockAdapter) Name() string { return m.name }

func (m *mockAdapter) DispatchGoal(_ context.Context, _ string, _ GoalContext) (string, error) {
	return "job-1", nil
}

func (m *mockAdapter) PollStatus(_ context.Context, _ string) (*JobResult, error) {
	return &JobResult{Status: StatusCompleted}, nil
}

func (m *mockAdapter) HandleCallback(_ context.Context, _ json.RawMessage) error {
	return nil
}

func (m *mockAdapter) ListCapabilities(_ context.Context) ([]Capability, error) {
	return []Capability{{Name: m.name, Version: "1.0"}}, nil
}

func (m *mockAdapter) HealthCheck(_ context.Context) (bool, error) {
	return m.healthy, nil
}

func TestNewRegistryIsEmpty(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	names := r.List()
	if len(names) != 0 {
		t.Errorf("expected empty registry, got %v", names)
	}
}

func TestRegisterAddsAdapter(t *testing.T) {
	r := NewRegistry()
	a := &mockAdapter{name: "dtec"}
	if err := r.Register(a); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	names := r.List()
	if len(names) != 1 || names[0] != "dtec" {
		t.Errorf("expected [dtec], got %v", names)
	}
}

func TestRegisterDuplicateReturnsError(t *testing.T) {
	r := NewRegistry()
	a := &mockAdapter{name: "dtec"}
	if err := r.Register(a); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	err := r.Register(a)
	if err == nil {
		t.Fatal("expected error on duplicate register, got nil")
	}
}

func TestRegisterNilReturnsError(t *testing.T) {
	r := NewRegistry()
	err := r.Register(nil)
	if err == nil {
		t.Fatal("expected error registering nil adapter, got nil")
	}
}

func TestRegisterEmptyNameReturnsError(t *testing.T) {
	r := NewRegistry()
	a := &mockAdapter{name: ""}
	err := r.Register(a)
	if err == nil {
		t.Fatal("expected error registering adapter with empty name, got nil")
	}
}

func TestGetReturnsRegisteredAdapter(t *testing.T) {
	r := NewRegistry()
	a := &mockAdapter{name: "agentforce"}
	_ = r.Register(a)

	got, ok := r.Get("agentforce")
	if !ok {
		t.Fatal("Get returned ok=false for registered adapter")
	}
	if got.Name() != "agentforce" {
		t.Errorf("got adapter name %q, want %q", got.Name(), "agentforce")
	}
}

func TestGetReturnsFalseForUnknown(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("Get returned ok=true for unknown adapter")
	}
}

func TestListReturnsAllNames(t *testing.T) {
	r := NewRegistry()
	names := []string{"a", "b", "c"}
	for _, n := range names {
		_ = r.Register(&mockAdapter{name: n})
	}
	listed := r.List()
	if len(listed) != len(names) {
		t.Fatalf("expected %d names, got %d: %v", len(names), len(listed), listed)
	}
	seen := make(map[string]bool)
	for _, n := range listed {
		seen[n] = true
	}
	for _, n := range names {
		if !seen[n] {
			t.Errorf("name %q not in List result", n)
		}
	}
}

func TestHealthCheckAllChecksAllAdapters(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&mockAdapter{name: "healthy", healthy: true})
	_ = r.Register(&mockAdapter{name: "unhealthy", healthy: false})

	results := r.HealthCheckAll(context.Background())
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results["healthy"] {
		t.Error("expected 'healthy' adapter to be healthy")
	}
	if results["unhealthy"] {
		t.Error("expected 'unhealthy' adapter to be unhealthy")
	}
}

func TestHealthCheckAllEmptyRegistry(t *testing.T) {
	r := NewRegistry()
	results := r.HealthCheckAll(context.Background())
	if len(results) != 0 {
		t.Errorf("expected empty results for empty registry, got %v", results)
	}
}
