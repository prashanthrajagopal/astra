package scheduler

import (
	"context"
	"database/sql"
	"hash/fnv"
	"log/slog"
	"os"
	"strconv"
	"time"

	"astra/internal/messaging"
	"astra/internal/tasks"
)

type Scheduler struct {
	store *tasks.Store
	bus   *messaging.Bus
}

func New(db *sql.DB, bus *messaging.Bus) *Scheduler {
	return &Scheduler{
		store: tasks.NewStore(db),
		bus:   bus,
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	slog.Info("scheduler started")
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduler stopped")
			return ctx.Err()
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				slog.Error("scheduler tick failed", "err", err)
			}
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) error {
	if n, err := s.store.FailBlockedTasks(ctx); err != nil {
		slog.Error("fail blocked tasks failed", "err", err)
	} else if n > 0 {
		slog.Info("cascade-failed blocked tasks", "count", n)
	}

	if n, err := s.store.RecoverStaleQueued(ctx); err != nil {
		slog.Error("recover stale queued failed", "err", err)
	} else if n > 0 {
		slog.Info("recovered stale queued tasks", "count", n)
	}

	shardCount := getTaskShardCount()
	ready, err := s.store.FindReadyTasksWithAgentIDs(ctx, 100)
	if err != nil {
		return err
	}
	for _, r := range ready {
		if err := s.dispatchToShard(ctx, r.TaskID, r.AgentID, shardCount); err != nil {
			slog.Error("dispatch failed", "task_id", r.TaskID, "err", err)
		}
	}

	goals, err := s.store.FindGoalsToFinalize(ctx)
	if err != nil {
		slog.Error("find goals to finalize failed", "err", err)
	}
	for _, goalID := range goals {
		if err := s.store.AutoFinalizeGoal(ctx, goalID); err != nil {
			slog.Error("auto-finalize goal failed", "goal_id", goalID, "err", err)
		} else {
			slog.Info("auto-finalized goal", "goal_id", goalID)
		}
	}

	return nil
}

func getTaskShardCount() int {
	if s := os.Getenv("TASK_SHARD_COUNT"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return 1
}

func shardForAgent(agentID string, count int) int {
	if count <= 1 {
		return 0
	}
	h := fnv.New32a()
	h.Write([]byte(agentID))
	return int(h.Sum32()%uint32(count)) % count
}

func taskStreamForShard(shard int) string {
	return "astra:tasks:shard:" + strconv.Itoa(shard)
}

func (s *Scheduler) dispatchToShard(ctx context.Context, taskID, agentID string, shardCount int) error {
	if err := s.store.Transition(ctx, taskID, tasks.StatusPending, tasks.StatusQueued, nil); err != nil {
		return err
	}
	shard := shardForAgent(agentID, shardCount)
	return s.bus.Publish(ctx, taskStreamForShard(shard), map[string]interface{}{
		"task_id": taskID,
	})
}
