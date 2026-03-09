package integration

import (
	"context"
	"encoding/json"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"astra/internal/actors"
	"astra/internal/agent"
	"astra/internal/events"
	"astra/internal/kernel"
	"astra/internal/messaging"
	"astra/internal/planner"
	"astra/internal/scheduler"
	"astra/internal/tasks"
	"astra/pkg/db"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestE2E_SpawnGoalScheduleComplete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// 1. Check env vars or try to connect to Postgres and Redis
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://astra:changeme@localhost:5432/astra?sslmode=disable"
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	// 2. Connect to Postgres
	database, err := db.Connect(dsn)
	if err != nil {
		t.Skipf("requires postgres and redis: postgres connect failed: %v", err)
	}
	defer database.Close()

	// 3. Connect to Redis
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("requires postgres and redis: redis connect failed: %v", err)
	}
	rdb.Close()

	bus := messaging.New(redisAddr)
	defer bus.Close()

	// 4. Create internal components
	eventStore := events.NewStore(database)
	_ = eventStore // used implicitly via tasks.Store which writes to events
	taskStore := tasks.NewStore(database)
	sched := scheduler.New(database, bus)
	k := kernel.New()
	p := planner.New()

	// 5. Create agent with all deps
	ag := agent.New("test-agent", k, p, taskStore, database)
	defer ag.Stop()

	// Insert agent into agents table (goals FK requires it)
	_, err = database.ExecContext(context.Background(),
		`INSERT INTO agents (id, name, status) VALUES ($1, $2, 'active') ON CONFLICT (id) DO NOTHING`,
		ag.ID, ag.Name)
	if err != nil {
		t.Fatalf("insert agent: %v", err)
	}

	// 6. Send CreateGoal message via kernel.Send
	msg := actors.Message{
		ID:        uuid.New().String(),
		Type:      "CreateGoal",
		Source:    "test",
		Target:    ag.ID.String(),
		Payload:   json.RawMessage(`{"goal_text":"test goal"}`),
		Timestamp: time.Now(),
	}
	if err := k.Send(context.Background(), ag.ID.String(), msg); err != nil {
		t.Fatalf("kernel.Send: %v", err)
	}

	// 7. Wait briefly for agent to process
	time.Sleep(150 * time.Millisecond)

	// 8. Verify goals table has 1 row for this agent
	var goalCount int
	err = database.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM goals WHERE agent_id = $1`, ag.ID).Scan(&goalCount)
	if err != nil {
		t.Fatalf("count goals: %v", err)
	}
	if goalCount != 1 {
		t.Errorf("expected 1 goal, got %d", goalCount)
	}

	// 9. Verify tasks table has 2 rows with status pending or created
	var taskCount int
	err = database.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM tasks WHERE agent_id = $1`, ag.ID).Scan(&taskCount)
	if err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if taskCount != 2 {
		t.Errorf("expected 2 tasks, got %d", taskCount)
	}

	// 10. Run one scheduler tick (Run with short-lived context)
	ctx, cancelSched := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancelSched()
	go func() {
		_ = sched.Run(ctx)
	}()
	<-ctx.Done()

	// 11. Wait briefly, verify at least one task is queued
	time.Sleep(50 * time.Millisecond)
	var queuedCount int
	err = database.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM tasks WHERE agent_id = $1 AND status = 'queued'`, ag.ID).Scan(&queuedCount)
	if err != nil {
		t.Fatalf("count queued tasks: %v", err)
	}
	if queuedCount < 1 {
		t.Errorf("expected at least 1 queued task, got %d", queuedCount)
	}

	// 12. Simulate execution-worker: consume from Redis stream, transition queued→scheduled→running→completed
	var processed atomic.Int32
	execCtx, cancelExec := context.WithCancel(context.Background())
	defer cancelExec()

	go func() {
		bus.Consume(execCtx, "astra:tasks:shard:0", "e2e-worker-group", "e2e-consumer-1", func(m redis.XMessage) error {
			taskIDVal, ok := m.Values["task_id"]
			if !ok {
				return nil
			}
			taskID, ok := taskIDVal.(string)
			if !ok || taskID == "" {
				return nil
			}

			ctx := context.Background()
			if err := taskStore.Transition(ctx, taskID, tasks.StatusQueued, tasks.StatusScheduled, nil); err != nil {
				return err
			}
			if err := taskStore.Transition(ctx, taskID, tasks.StatusScheduled, tasks.StatusRunning, nil); err != nil {
				return err
			}
			if err := taskStore.CompleteTask(ctx, taskID, []byte("{}")); err != nil {
				return err
			}
			if processed.Add(1) >= 2 {
				cancelExec()
			}
			return nil
		})
	}()

	deadline := time.Now().Add(5 * time.Second)
	for processed.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if processed.Load() < 2 {
		t.Fatalf("expected 2 tasks completed, got %d", processed.Load())
	}

	// 13. Verify all tasks are completed in Postgres
	var completedCount int
	err = database.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM tasks WHERE agent_id = $1 AND status = 'completed'`, ag.ID).Scan(&completedCount)
	if err != nil {
		t.Fatalf("count completed tasks: %v", err)
	}
	if completedCount != 2 {
		t.Errorf("expected 2 completed tasks, got %d", completedCount)
	}

	// 14. Verify events table has entries for task transitions
	var eventCount int
	err = database.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM events WHERE actor_id IN (SELECT id FROM tasks WHERE agent_id = $1)`, ag.ID).Scan(&eventCount)
	if err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount < 2 {
		t.Errorf("expected at least 2 events for task transitions, got %d", eventCount)
	}

	// 15. Clean up: delete test data
	_, _ = database.ExecContext(context.Background(),
		`DELETE FROM task_dependencies WHERE task_id IN (SELECT id FROM tasks WHERE agent_id = $1)
		 OR depends_on IN (SELECT id FROM tasks WHERE agent_id = $1)`, ag.ID, ag.ID)
	_, _ = database.ExecContext(context.Background(),
		`DELETE FROM events WHERE actor_id IN (SELECT id FROM tasks WHERE agent_id = $1)`, ag.ID)
	_, _ = database.ExecContext(context.Background(),
		`DELETE FROM tasks WHERE agent_id = $1`, ag.ID)
	_, _ = database.ExecContext(context.Background(),
		`DELETE FROM goals WHERE agent_id = $1`, ag.ID)
	_, _ = database.ExecContext(context.Background(),
		`DELETE FROM agents WHERE id = $1`, ag.ID)
}
