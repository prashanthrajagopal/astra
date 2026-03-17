package actors

import (
	"log/slog"
	"sync"
	"time"
)

type RestartPolicy int

const (
	RestartImmediate RestartPolicy = iota
	RestartBackoff
	Escalate
	Terminate
)

type Supervisor struct {
	mu          sync.RWMutex
	children    map[string]Actor
	policy      RestartPolicy
	restarts    int
	maxRestarts int
	window      time.Duration
	windowStart time.Time
}

func NewSupervisor(policy RestartPolicy, maxRestarts int, window time.Duration) *Supervisor {
	return &Supervisor{
		children:    make(map[string]Actor),
		policy:      policy,
		maxRestarts: maxRestarts,
		window:      window,
		windowStart: time.Now(),
	}
}

func (s *Supervisor) Watch(a Actor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.children[a.ID()] = a
}

func (s *Supervisor) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.children, id)
}

func (s *Supervisor) HandleFailure(id string) RestartPolicy {
	s.mu.Lock()
	defer s.mu.Unlock()

	if time.Since(s.windowStart) > s.window {
		s.restarts = 0
		s.windowStart = time.Now()
	}

	s.restarts++
	if s.restarts > s.maxRestarts {
		slog.Warn("supervisor circuit breaker tripped", "child", id, "restarts", s.restarts)
		return Terminate
	}

	return s.policy
}

func (s *Supervisor) StopAll() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.children {
		_ = a.Stop()
	}
}
