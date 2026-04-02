package workers

import (
	"testing"

	"astra/internal/messaging"

	"github.com/google/uuid"
)

func TestNew_ReturnsWorker(t *testing.T) {
	bus := messaging.New("localhost:6379")
	w := New("test-host", bus)
	if w == nil {
		t.Fatal("New returned nil")
	}
}

func TestNew_IDIsValidUUID(t *testing.T) {
	bus := messaging.New("localhost:6379")
	w := New("test-host", bus)
	if w.ID == uuid.Nil {
		t.Error("worker ID is nil UUID")
	}
	// Verify it parses as a valid UUID string.
	parsed, err := uuid.Parse(w.ID.String())
	if err != nil {
		t.Errorf("worker ID is not a valid UUID: %v", err)
	}
	if parsed != w.ID {
		t.Errorf("parsed UUID %v != original %v", parsed, w.ID)
	}
}

func TestNew_HostnameIsSet(t *testing.T) {
	bus := messaging.New("localhost:6379")
	w := New("my-worker-host", bus)
	if w.Hostname != "my-worker-host" {
		t.Errorf("Hostname = %q, want %q", w.Hostname, "my-worker-host")
	}
}

func TestNew_UniqueIDs(t *testing.T) {
	bus := messaging.New("localhost:6379")
	w1 := New("host1", bus)
	w2 := New("host2", bus)
	if w1.ID == w2.ID {
		t.Errorf("two workers have the same ID: %v", w1.ID)
	}
}

func TestNew_DifferentHostnames(t *testing.T) {
	tests := []struct {
		hostname string
	}{
		{"worker-01"},
		{"worker-02"},
		{"prod-worker.example.com"},
		{""},
	}
	bus := messaging.New("localhost:6379")
	for _, tc := range tests {
		t.Run(tc.hostname, func(t *testing.T) {
			w := New(tc.hostname, bus)
			if w.Hostname != tc.hostname {
				t.Errorf("Hostname = %q, want %q", w.Hostname, tc.hostname)
			}
		})
	}
}

func TestNewWithDB_ReturnsWorker(t *testing.T) {
	// NewWithDB with nil db still constructs a Worker (db ops fail at runtime).
	bus := messaging.New("localhost:6379")
	w := NewWithDB("db-host", bus, nil)
	if w == nil {
		t.Fatal("NewWithDB returned nil")
	}
	if w.ID == uuid.Nil {
		t.Error("worker ID is nil UUID")
	}
	if w.Hostname != "db-host" {
		t.Errorf("Hostname = %q, want db-host", w.Hostname)
	}
}

func TestNewRegistry_NotNil(t *testing.T) {
	r := NewRegistry(nil)
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
}

func TestWorkerInfo_Fields(t *testing.T) {
	info := WorkerInfo{
		ID:           "worker-id-1",
		Hostname:     "host-1",
		Status:       "active",
		Capabilities: []string{"general", "gpu"},
	}
	if info.ID != "worker-id-1" {
		t.Errorf("ID = %q, want worker-id-1", info.ID)
	}
	if info.Hostname != "host-1" {
		t.Errorf("Hostname = %q, want host-1", info.Hostname)
	}
	if info.Status != "active" {
		t.Errorf("Status = %q, want active", info.Status)
	}
	if len(info.Capabilities) != 2 {
		t.Errorf("Capabilities len = %d, want 2", len(info.Capabilities))
	}
}
