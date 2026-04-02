package circuitbreaker

import (
	"testing"
	"time"
)

func TestNewCreatesClosedBreaker(t *testing.T) {
	b := New(5, 30*time.Second, 10*time.Second)
	if b == nil {
		t.Fatal("New returned nil")
	}
	if b.State() != StateClosed {
		t.Errorf("initial state: got %v, want StateClosed", b.State())
	}
	if b.threshold != 5 {
		t.Errorf("threshold: got %d, want 5", b.threshold)
	}
}

func TestNewDefaultsForZeroValues(t *testing.T) {
	b := New(0, 0, 0)
	if b.threshold != 5 {
		t.Errorf("threshold default: got %d, want 5", b.threshold)
	}
	if b.window != 30*time.Second {
		t.Errorf("window default: got %v, want 30s", b.window)
	}
	if b.cooldown != 10*time.Second {
		t.Errorf("cooldown default: got %v, want 10s", b.cooldown)
	}
}

func TestNewDefaultsForNegativeValues(t *testing.T) {
	b := New(-1, -1, -1)
	if b.threshold != 5 {
		t.Errorf("threshold default: got %d, want 5", b.threshold)
	}
	if b.window != 30*time.Second {
		t.Errorf("window default: got %v, want 30s", b.window)
	}
	if b.cooldown != 10*time.Second {
		t.Errorf("cooldown default: got %v, want 10s", b.cooldown)
	}
}

func TestClosedStateAllowsCalls(t *testing.T) {
	b := New(5, 30*time.Second, 10*time.Second)
	for i := 0; i < 10; i++ {
		if !b.Allow() {
			t.Errorf("Allow() returned false in closed state (iteration %d)", i)
		}
	}
}

func TestOpensAfterThresholdFailures(t *testing.T) {
	b := New(3, 30*time.Second, 10*time.Second)
	for i := 0; i < 2; i++ {
		b.RecordFailure()
		if b.State() != StateClosed {
			t.Errorf("should remain closed after %d failures", i+1)
		}
	}
	b.RecordFailure() // 3rd failure → open
	if b.State() != StateOpen {
		t.Errorf("state: got %v, want StateOpen after threshold failures", b.State())
	}
}

func TestOpenStateRejectsCalls(t *testing.T) {
	b := New(1, 30*time.Second, time.Hour) // long cooldown so it stays open
	b.RecordFailure()
	if b.State() != StateOpen {
		t.Fatal("breaker should be open")
	}
	if b.Allow() {
		t.Error("Allow() returned true in open state")
	}
}

func TestHalfOpenAfterCooldown(t *testing.T) {
	b := New(1, 30*time.Second, 10*time.Millisecond) // short cooldown
	b.RecordFailure()
	if b.State() != StateOpen {
		t.Fatal("breaker should be open")
	}

	time.Sleep(20 * time.Millisecond)

	// Allow() should transition to half-open and allow one trial
	if !b.Allow() {
		t.Error("Allow() should allow one trial in half-open state")
	}
	if b.State() != StateHalfOpen {
		t.Errorf("state: got %v, want StateHalfOpen", b.State())
	}
}

func TestSuccessfulCallInHalfOpenClosesCircuit(t *testing.T) {
	b := New(1, 30*time.Second, 10*time.Millisecond)
	b.RecordFailure()
	time.Sleep(20 * time.Millisecond)
	b.Allow() // transitions to half-open

	b.RecordSuccess()
	if b.State() != StateClosed {
		t.Errorf("state: got %v, want StateClosed after success in half-open", b.State())
	}
	// failures should be cleared
	if len(b.failures) != 0 {
		t.Errorf("failures not cleared after close: %d", len(b.failures))
	}
}

func TestFailedCallInHalfOpenReopensCircuit(t *testing.T) {
	b := New(1, 30*time.Second, 10*time.Millisecond)
	b.RecordFailure()
	time.Sleep(20 * time.Millisecond)
	b.Allow() // transitions to half-open

	b.RecordFailure()
	if b.State() != StateOpen {
		t.Errorf("state: got %v, want StateOpen after failure in half-open", b.State())
	}
}

func TestHalfOpenAllowsOnlyOneTrial(t *testing.T) {
	b := New(1, 30*time.Second, 10*time.Millisecond)
	b.RecordFailure()
	time.Sleep(20 * time.Millisecond)

	// First Allow() transitions Open→HalfOpen, returns true (trial from transition).
	if !b.Allow() {
		t.Error("first Allow() transitioning to half-open should return true")
	}
	// halfOpenTry is now true; second Allow() consumes it and returns true.
	if !b.Allow() {
		t.Error("second Allow() consuming halfOpenTry should return true")
	}
	// Third Allow() — halfOpenTry is now false, should be blocked.
	if b.Allow() {
		t.Error("third Allow() in half-open (trial consumed) should return false")
	}
}

func TestRecordSuccessInClosedStateIsNoOp(t *testing.T) {
	b := New(5, 30*time.Second, 10*time.Second)
	b.RecordSuccess() // no-op in closed
	if b.State() != StateClosed {
		t.Errorf("state changed unexpectedly: %v", b.State())
	}
}

func TestFailuresOutsideWindowNotCounted(t *testing.T) {
	b := New(3, 50*time.Millisecond, 10*time.Second) // very short window
	b.RecordFailure()
	b.RecordFailure()
	// wait for window to expire
	time.Sleep(60 * time.Millisecond)
	// these old failures should be pruned on the next RecordFailure
	b.RecordFailure() // only 1 in window, should not open
	if b.State() != StateClosed {
		t.Errorf("state: got %v, want StateClosed (old failures pruned)", b.State())
	}
}

func TestAllowMethod(t *testing.T) {
	b := New(2, 30*time.Second, time.Hour)
	// closed → allow
	if !b.Allow() {
		t.Error("Allow() should return true in closed state")
	}
	b.RecordFailure()
	b.RecordFailure()
	// open → reject
	if b.Allow() {
		t.Error("Allow() should return false in open state")
	}
}

func TestStateMethod(t *testing.T) {
	b := New(1, 30*time.Second, time.Hour)
	if b.State() != StateClosed {
		t.Errorf("initial: got %v, want StateClosed", b.State())
	}
	b.RecordFailure()
	if b.State() != StateOpen {
		t.Errorf("after failure: got %v, want StateOpen", b.State())
	}
}
