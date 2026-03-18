package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	taskKeyPrefix  = "task:"
	graphKeyPrefix = "graph:"
)

// CachedStore wraps a Store with Redis cache-aside for GetTask and GetGraph.
// If rdb is nil, all operations delegate to the inner store (no cache).
type CachedStore struct {
	store *Store
	rdb   *redis.Client
	ttl   time.Duration
}

// NewCachedStore returns a CachedStore. If rdb is nil, it behaves like the inner store (no cache).
func NewCachedStore(store *Store, rdb *redis.Client, ttl time.Duration) *CachedStore {
	return &CachedStore{store: store, rdb: rdb, ttl: ttl}
}

// InvalidateTask removes the task from cache. Call after state-changing writes.
func (c *CachedStore) InvalidateTask(ctx context.Context, taskID string) {
	if c.rdb == nil {
		return
	}
	_ = c.rdb.Del(ctx, taskKeyPrefix+taskID).Err()
}

// InvalidateGraph removes the graph from cache. Call after CreateGraph or when graph tasks change.
func (c *CachedStore) InvalidateGraph(ctx context.Context, graphID string) {
	if c.rdb == nil {
		return
	}
	_ = c.rdb.Del(ctx, graphKeyPrefix+graphID).Err()
}

func (c *CachedStore) GetTask(ctx context.Context, taskID string) (*Task, error) {
	if c.rdb == nil {
		return c.store.GetTask(ctx, taskID)
	}
	key := taskKeyPrefix + taskID
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err == nil {
		var t Task
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("tasks.CachedStore GetTask unmarshal: %w", err)
		}
		return &t, nil
	}
	if err != redis.Nil {
		return nil, fmt.Errorf("tasks.CachedStore GetTask redis: %w", err)
	}
	t, err := c.store.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, nil
	}
	data, err = json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("tasks.CachedStore GetTask marshal: %w", err)
	}
	_ = c.rdb.SetEx(ctx, key, data, c.ttl).Err() // best-effort populate; don't fail request
	return t, nil
}

func (c *CachedStore) GetGraph(ctx context.Context, graphID string) (*Graph, []Dependency, error) {
	if c.rdb == nil {
		return c.store.GetGraph(ctx, graphID)
	}
	key := graphKeyPrefix + graphID
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err == nil {
		var g Graph
		if err := json.Unmarshal(data, &g); err != nil {
			return nil, nil, fmt.Errorf("tasks.CachedStore GetGraph unmarshal: %w", err)
		}
		return &g, g.Dependencies, nil
	}
	if err != redis.Nil {
		return nil, nil, fmt.Errorf("tasks.CachedStore GetGraph redis: %w", err)
	}
	graph, deps, err := c.store.GetGraph(ctx, graphID)
	if err != nil {
		return nil, nil, err
	}
	if graph == nil {
		return nil, nil, nil
	}
	data, err = json.Marshal(graph)
	if err != nil {
		return nil, nil, fmt.Errorf("tasks.CachedStore GetGraph marshal: %w", err)
	}
	_ = c.rdb.SetEx(ctx, key, data, c.ttl).Err() // best-effort populate; don't fail request
	return graph, deps, nil
}

func (c *CachedStore) CreateTask(ctx context.Context, t *Task) error {
	return c.store.CreateTask(ctx, t)
}

func (c *CachedStore) AddDependencies(ctx context.Context, taskID string, dependsOn []string) error {
	return c.store.AddDependencies(ctx, taskID, dependsOn)
}

func (c *CachedStore) CreateGraph(ctx context.Context, graph *Graph) error {
	if err := c.store.CreateGraph(ctx, graph); err != nil {
		return err
	}
	c.InvalidateGraph(ctx, graph.ID.String())
	return nil
}

func (c *CachedStore) Transition(ctx context.Context, taskID string, from, to Status, eventPayload json.RawMessage) error {
	if err := c.store.Transition(ctx, taskID, from, to, eventPayload); err != nil {
		return err
	}
	c.InvalidateTask(ctx, taskID)
	return nil
}

func (c *CachedStore) CompleteTask(ctx context.Context, taskID string, result []byte) error {
	if err := c.store.CompleteTask(ctx, taskID, result); err != nil {
		return err
	}
	c.InvalidateTask(ctx, taskID)
	return nil
}

func (c *CachedStore) FailTask(ctx context.Context, taskID string, errMsg string) (bool, error) {
	moved, err := c.store.FailTask(ctx, taskID, errMsg)
	if err != nil {
		return false, err
	}
	c.InvalidateTask(ctx, taskID)
	return moved, nil
}

func (c *CachedStore) FindReadyTasks(ctx context.Context, limit int) ([]string, error) {
	return c.store.FindReadyTasks(ctx, limit)
}

func (c *CachedStore) SetWorkerID(ctx context.Context, taskID, workerID string) error {
	if err := c.store.SetWorkerID(ctx, taskID, workerID); err != nil {
		return err
	}
	c.InvalidateTask(ctx, taskID)
	return nil
}

func (c *CachedStore) FindOrphanedRunningTasks(ctx context.Context) ([]string, error) {
	return c.store.FindOrphanedRunningTasks(ctx)
}

func (c *CachedStore) ListTasksByGoalID(ctx context.Context, goalID string) ([]*Task, error) {
	return c.store.ListTasksByGoalID(ctx, goalID)
}

func (c *CachedStore) RequeueTask(ctx context.Context, taskID string) error {
	if err := c.store.RequeueTask(ctx, taskID); err != nil {
		return err
	}
	c.InvalidateTask(ctx, taskID)
	return nil
}

func (c *CachedStore) CancelTask(ctx context.Context, taskID string) error {
	if err := c.store.CancelTask(ctx, taskID); err != nil {
		return err
	}
	c.InvalidateTask(ctx, taskID)
	return nil
}
