package scheduler

import (
	"context"
	"database/sql"
	"log/slog"
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

	ready, err := s.store.FindReadyTasks(ctx, 100)
	if err != nil {
		return err
	}
	for _, taskID := range ready {
		if err := s.dispatch(ctx, taskID); err != nil {
			slog.Error("dispatch failed", "task_id", taskID, "err", err)
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

func (s *Scheduler) dispatch(ctx context.Context, taskID string) error {
	if err := s.store.Transition(ctx, taskID, tasks.StatusPending, tasks.StatusQueued, nil); err != nil {
		return err
	}
	return s.bus.Publish(ctx, "astra:tasks:shard:0", map[string]interface{}{
		"task_id": taskID,
	})
}
