package circuitbreaker

import (
	"sync"
	"time"
)

// State is the circuit state.
type State int

const (
	StateClosed   State = iota
	StateOpen
	StateHalfOpen
)

// Breaker is a per-target circuit breaker using a sliding window of failures.
type Breaker struct {
	mu          sync.Mutex
	threshold   int
	window      time.Duration
	cooldown    time.Duration
	failures    []time.Time
	state       State
	openUntil   time.Time
	halfOpenTry bool
}

// New returns a circuit breaker with the given threshold (failures to open), window (time window for counting failures), and cooldown (time before half-open).
func New(threshold int, window, cooldown time.Duration) *Breaker {
	if threshold <= 0 {
		threshold = 5
	}
	if window <= 0 {
		window = 30 * time.Second
	}
	if cooldown <= 0 {
		cooldown = 10 * time.Second
	}
	return &Breaker{
		threshold: threshold,
		window:    window,
		cooldown:  cooldown,
		failures:  nil,
		state:     StateClosed,
	}
}

// Allow returns true if the call is allowed (circuit closed or half-open and allowing a trial). If false, the caller should return 503.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	switch b.state {
	case StateClosed:
		return true
	case StateOpen:
		if now.Before(b.openUntil) {
			return false
		}
		b.state = StateHalfOpen
		b.halfOpenTry = true
		return true
	case StateHalfOpen:
		if b.halfOpenTry {
			b.halfOpenTry = false
			return true
		}
		return false
	}
	return true
}

// RecordSuccess records a successful call. Call after a downstream call succeeds.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == StateHalfOpen {
		b.state = StateClosed
		b.failures = nil
	}
}

// RecordFailure records a failed call. Call after a downstream call fails (error or 5xx).
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-b.window)
	// prune old failures
	var kept []time.Time
	for _, t := range b.failures {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	b.failures = kept

	if b.state == StateHalfOpen {
		b.state = StateOpen
		b.openUntil = now.Add(b.cooldown)
		return
	}
	if b.state == StateClosed && len(b.failures) >= b.threshold {
		b.state = StateOpen
		b.openUntil = now.Add(b.cooldown)
	}
}

// State returns the current state (for metrics or logging).
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}
