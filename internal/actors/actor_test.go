package actors

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestBaseActor_ReceiveAndHandle(t *testing.T) {
	actor := NewBaseActor("test-actor")
	var received []Message
	var mu sync.Mutex

	actor.Start(func(ctx context.Context, msg Message) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	})
	defer func() { _ = actor.Stop() }()

	msg := Message{
		ID:        "msg-1",
		Type:      "test",
		Source:    "sender",
		Target:    "test-actor",
		Timestamp: time.Now(),
	}
	if err := actor.Receive(context.Background(), msg); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	// Allow handler goroutine to process
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 message received, got %d", len(received))
	}
	if received[0].ID != "msg-1" || received[0].Type != "test" {
		t.Errorf("expected msg ID=msg-1 Type=test, got ID=%s Type=%s", received[0].ID, received[0].Type)
	}
}

func TestBaseActor_Stop(t *testing.T) {
	actor := NewBaseActor("test-actor-stop")
	actor.Start(func(context.Context, Message) error { return nil })
	done := make(chan struct{})
	go func() {
		_ = actor.Stop()
		close(done)
	}()
	select {
	case <-done:
		// goroutine exited, no hang
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within 2s (goroutine may be hung)")
	}
}

func TestBaseActor_MailboxFull(t *testing.T) {
	actor := NewBaseActor("test-actor-full")
	// Do NOT Start - no consumer, so we can fill the mailbox
	msg := Message{ID: "fill", Type: "test", Timestamp: time.Now()}

	// Default buffer is 1024; fill it
	for i := 0; i < 1024; i++ {
		if err := actor.Receive(context.Background(), msg); err != nil {
			t.Fatalf("Receive (msg %d): expected nil, got %v", i+1, err)
		}
	}
	// 1025th should return ErrMailboxFull
	if err := actor.Receive(context.Background(), msg); err != ErrMailboxFull {
		t.Fatalf("Receive (1025th): expected ErrMailboxFull, got %v", err)
	}
}
