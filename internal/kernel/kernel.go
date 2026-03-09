package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"astra/internal/actors"
	"astra/pkg/metrics"
)

type Kernel struct {
	mu     sync.RWMutex
	actors map[string]actors.Actor
}

func New() *Kernel {
	return &Kernel{
		actors: make(map[string]actors.Actor),
	}
}

func (k *Kernel) Spawn(a actors.Actor) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.actors[a.ID()] = a
	metrics.ActorCount.Inc()
	slog.Info("actor spawned", "actor_id", a.ID())
}

func (k *Kernel) Send(ctx context.Context, target string, msg actors.Message) error {
	k.mu.RLock()
	a, ok := k.actors[target]
	k.mu.RUnlock()
	if !ok {
		return fmt.Errorf("kernel.Send: actor %s not found", target)
	}
	return a.Receive(ctx, msg)
}

func (k *Kernel) Stop(id string) error {
	k.mu.Lock()
	a, ok := k.actors[id]
	if ok {
		delete(k.actors, id)
	}
	k.mu.Unlock()
	if !ok {
		return fmt.Errorf("kernel.Stop: actor %s not found", id)
	}
	metrics.ActorCount.Dec()
	return a.Stop()
}

func (k *Kernel) ActorCount() int {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return len(k.actors)
}
