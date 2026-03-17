package kernel

import (
	"context"
	"testing"

	"astra/internal/actors"

	"github.com/google/uuid"
)

type mockActor struct {
	id       string
	messages []actors.Message
	stopped  bool
}

func (m *mockActor) ID() string { return m.id }

func (m *mockActor) Receive(_ context.Context, msg actors.Message) error {
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockActor) Stop() error {
	m.stopped = true
	return nil
}

func TestKernel_SpawnAndSend(t *testing.T) {
	k := New()
	mock := &mockActor{id: "actor-1"}
	k.Spawn(mock)
	defer func() { _ = k.Stop("actor-1") }()

	msg := actors.Message{
		ID:     uuid.New().String(),
		Type:   "test",
		Source: "caller",
		Target: "actor-1",
	}
	if err := k.Send(context.Background(), "actor-1", msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(mock.messages) != 1 {
		t.Fatalf("expected 1 message received, got %d", len(mock.messages))
	}
	if mock.messages[0].ID != msg.ID || mock.messages[0].Type != "test" {
		t.Errorf("expected message ID=%s Type=test, got ID=%s Type=%s", msg.ID, mock.messages[0].ID, mock.messages[0].Type)
	}
}

func TestKernel_SendUnknownActor(t *testing.T) {
	k := New()
	msg := actors.Message{ID: "m1", Type: "test", Target: "unknown"}
	err := k.Send(context.Background(), "unknown", msg)
	if err == nil {
		t.Fatal("expected error when sending to unknown actor")
	}
	if err.Error() != "kernel.Send: actor unknown not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestKernel_Stop(t *testing.T) {
	k := New()
	mock := &mockActor{id: "actor-stop"}
	k.Spawn(mock)
	if err := k.Stop("actor-stop"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !mock.stopped {
		t.Error("actor was not stopped")
	}
	msg := actors.Message{ID: "m1", Type: "test"}
	err := k.Send(context.Background(), "actor-stop", msg)
	if err == nil {
		t.Fatal("expected error when sending to stopped actor")
	}
	if err.Error() != "kernel.Send: actor actor-stop not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestKernel_ActorCount(t *testing.T) {
	k := New()
	if n := k.ActorCount(); n != 0 {
		t.Fatalf("initial ActorCount: expected 0, got %d", n)
	}
	k.Spawn(&mockActor{id: "a1"})
	k.Spawn(&mockActor{id: "a2"})
	k.Spawn(&mockActor{id: "a3"})
	if n := k.ActorCount(); n != 3 {
		t.Fatalf("after spawn 3: expected 3, got %d", n)
	}
	_ = k.Stop("a2")
	if n := k.ActorCount(); n != 2 {
		t.Fatalf("after stop one: expected 2, got %d", n)
	}
}
