package adapters

import (
	"context"
	"fmt"
	"sync"
)

// Registry manages registered adapters and provides lookup by name.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

// NewRegistry creates an empty adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
	}
}

// Register adds an adapter to the registry keyed by adapter.Name().
// Returns an error if an adapter with the same name is already registered.
func (r *Registry) Register(adapter Adapter) error {
	if adapter == nil {
		return fmt.Errorf("adapters: cannot register nil adapter")
	}
	name := adapter.Name()
	if name == "" {
		return fmt.Errorf("adapters: adapter Name() must not be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[name]; exists {
		return fmt.Errorf("adapters: adapter %q already registered", name)
	}
	r.adapters[name] = adapter
	return nil
}

// Get returns the adapter registered under name and a boolean indicating
// whether it was found.
func (r *Registry) Get(name string) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[name]
	return a, ok
}

// List returns the names of all registered adapters in an unspecified order.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	return names
}

// HealthCheckAll calls HealthCheck on every registered adapter concurrently
// and returns a map of adapter name → healthy.
func (r *Registry) HealthCheckAll(ctx context.Context) map[string]bool {
	r.mu.RLock()
	snapshot := make(map[string]Adapter, len(r.adapters))
	for k, v := range r.adapters {
		snapshot[k] = v
	}
	r.mu.RUnlock()

	type result struct {
		name    string
		healthy bool
	}
	ch := make(chan result, len(snapshot))
	for name, adapter := range snapshot {
		go func(n string, a Adapter) {
			ok, _ := a.HealthCheck(ctx)
			ch <- result{name: n, healthy: ok}
		}(name, adapter)
	}

	out := make(map[string]bool, len(snapshot))
	for range snapshot {
		r := <-ch
		out[r.name] = r.healthy
	}
	return out
}
