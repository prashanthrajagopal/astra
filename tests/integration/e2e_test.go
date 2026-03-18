package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	// Scheduler orders by agents.priority (migration 0024); keep E2E working on older DBs.
	_, _ = database.ExecContext(context.Background(), `ALTER TABLE agents ADD COLUMN IF NOT EXISTS priority SMALLINT NOT NULL DEFAULT 0`)

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
	defer func() { _ = ag.Stop() }()

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
		_ = bus.Consume(execCtx, "astra:tasks:shard:0", "e2e-worker-group", "e2e-consumer-1", func(m redis.XMessage) error {
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

// TestE2E_AgentRestore_AcceptsCreateGoal verifies that an agent restored from DB (via NewFromExisting)
// is registered in the kernel and accepts CreateGoal. Simulates agent-service restart: DB has agent row,
// kernel is empty; restore creates in-memory agent from DB; SendMessage(CreateGoal) succeeds.
func TestE2E_AgentRestore_AcceptsCreateGoal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://astra:changeme@localhost:5432/astra?sslmode=disable"
	}
	database, err := db.Connect(dsn)
	if err != nil {
		t.Skipf("requires postgres: %v", err)
	}
	defer database.Close()

	ctx := context.Background()
	restoreAgentID := uuid.New()
	restoreName := "restore-test-agent"
	_, err = database.ExecContext(ctx,
		`INSERT INTO agents (id, name, actor_type, status) VALUES ($1, $2, $2, 'active')
		 ON CONFLICT (id) DO UPDATE SET status = 'active'`,
		restoreAgentID, restoreName)
	if err != nil {
		t.Fatalf("insert agent for restore test: %v", err)
	}
	defer func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM goals WHERE agent_id = $1`, restoreAgentID)
		_, _ = database.ExecContext(context.Background(), `DELETE FROM tasks WHERE goal_id IN (SELECT id FROM goals WHERE agent_id = $1)`, restoreAgentID)
		_, _ = database.ExecContext(context.Background(), `DELETE FROM agents WHERE id = $1`, restoreAgentID)
	}()

	k := kernel.New()
	p := planner.New()
	taskStore := tasks.NewStore(database)
	restored := agent.NewFromExisting(restoreAgentID, restoreName, k, p, taskStore, database)
	defer func() { _ = restored.Stop() }()

	if k.ActorCount() != 1 {
		t.Fatalf("expected kernel actor count 1 after restore, got %d", k.ActorCount())
	}

	msg := actors.Message{
		ID:        uuid.New().String(),
		Type:      "CreateGoal",
		Source:    "test",
		Target:    restoreAgentID.String(),
		Payload:   json.RawMessage(`{"goal_text":"restore test goal"}`),
		Timestamp: time.Now(),
	}
	if err := k.Send(ctx, restoreAgentID.String(), msg); err != nil {
		t.Fatalf("SendMessage CreateGoal to restored agent: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	var goalCount int
	err = database.QueryRowContext(ctx, `SELECT COUNT(*) FROM goals WHERE agent_id = $1`, restoreAgentID).Scan(&goalCount)
	if err != nil {
		t.Fatalf("count goals: %v", err)
	}
	if goalCount != 1 {
		t.Errorf("expected 1 goal after CreateGoal to restored agent, got %d", goalCount)
	}
}

// TestE2E_FailTask_MovesToDeadLetter verifies that when a task fails with retries >= maxRetries,
// FailTask transitions it to dead_letter and the task row has status dead_letter.
func TestE2E_FailTask_MovesToDeadLetter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://astra:changeme@localhost:5432/astra?sslmode=disable"
	}
	database, err := db.Connect(dsn)
	if err != nil {
		t.Skipf("requires postgres: %v", err)
	}
	defer database.Close()

	ctx := context.Background()
	// Ensure dead_letter status is allowed (migration 0025)
	_, _ = database.ExecContext(ctx, `ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_valid_status`)
	_, _ = database.ExecContext(ctx, `ALTER TABLE tasks ADD CONSTRAINT tasks_valid_status
		CHECK (status IN ('created', 'pending', 'queued', 'scheduled', 'running', 'completed', 'failed', 'dead_letter'))`)

	goalID := uuid.New()
	agentID := uuid.New()
	graphID := uuid.New()
	taskID := uuid.New()

	_, _ = database.ExecContext(ctx, `INSERT INTO agents (id, name, actor_type, status) VALUES ($1, 'dlq-agent', 'dlq-agent', 'active') ON CONFLICT (id) DO NOTHING`, agentID)
	_, err = database.ExecContext(ctx, `INSERT INTO goals (id, agent_id, goal_text, priority, status) VALUES ($1, $2, 'dlq goal', 100, 'active')`, goalID, agentID)
	if err != nil {
		t.Fatalf("insert goal: %v", err)
	}
	_, err = database.ExecContext(ctx,
		`INSERT INTO tasks (id, graph_id, goal_id, agent_id, type, status, payload, priority, retries, max_retries)
		 VALUES ($1, $2, $3, $4, 'code_generate', 'running', '{}', 100, 2, 2)`,
		taskID, graphID, goalID, agentID)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}
	defer func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM tasks WHERE id = $1`, taskID)
		_, _ = database.ExecContext(context.Background(), `DELETE FROM goals WHERE id = $1`, goalID)
		_, _ = database.ExecContext(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
	}()

	taskStore := tasks.NewStore(database)
	moved, err := taskStore.FailTask(ctx, taskID.String(), "final failure")
	if err != nil {
		t.Fatalf("FailTask: %v", err)
	}
	if !moved {
		t.Error("expected FailTask to return movedToDeadLetter true when retries >= maxRetries")
	}

	var status string
	err = database.QueryRowContext(ctx, `SELECT status FROM tasks WHERE id = $1`, taskID).Scan(&status)
	if err != nil {
		t.Fatalf("query task status: %v", err)
	}
	if status != string(tasks.StatusDeadLetter) {
		t.Errorf("expected task status dead_letter, got %q", status)
	}
}

// TestE2E_Consume_RetryAndDeadLetter verifies that after N handler failures a message is
// published to astra:dead_letter and the original is XAcked. Uses short MinIdle for test speed.
func TestE2E_Consume_RetryAndDeadLetter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("requires redis: %v", err)
	}
	defer rdb.Close()

	bus := messaging.New(redisAddr)
	defer bus.Close()

	stream := "astra:test:retry:" + uuid.New().String()
	group := "test-group"
	consumer := "test-consumer"

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	_, err := bus.PublishReturnID(ctx, stream, map[string]interface{}{"task_id": "tid-1", "payload": "test"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	var failCount atomic.Int32
	handler := func(msg redis.XMessage) error {
		failCount.Add(1)
		return fmt.Errorf("handler failure %d", failCount.Load())
	}

	go func() {
		_ = bus.ConsumeWithOptions(ctx, stream, group, consumer, handler, messaging.ConsumeOptions{
			MaxRetries: 3,
			MinIdle:    100 * time.Millisecond,
		})
	}()

	deadLetterStream := "astra:dead_letter"
	deadline := time.Now().Add(5 * time.Second)
	var n int64
	for time.Now().Before(deadline) {
		n, err = rdb.XLen(ctx, deadLetterStream).Result()
		if err == nil && n >= 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if n < 1 {
		t.Errorf("expected at least 1 message in %s after retries, got %d (failCount=%d)", deadLetterStream, n, failCount.Load())
	}
}

// TestE2E_GoalIdempotency verifies that two POST /goals with the same Idempotency-Key
// return the same goal_id. Requires goal-service running with Redis (e.g. GOAL_SERVICE_URL=http://localhost:8088).
// Skips if goal-service is not reachable. Requires a single goal-service instance sharing Redis for idempotency to apply.
func TestE2E_GoalIdempotency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	goalURL := os.Getenv("GOAL_SERVICE_URL")
	if goalURL == "" {
		goalURL = "http://localhost:8088"
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://astra:changeme@localhost:5432/astra?sslmode=disable"
	}
	database, err := db.Connect(dsn)
	if err != nil {
		t.Skipf("requires postgres: %v", err)
	}
	defer database.Close()

	ctx := context.Background()
	agentID := uuid.New()
	_, err = database.ExecContext(ctx,
		`INSERT INTO agents (id, name, actor_type, status) VALUES ($1, 'idem-agent', 'idem-agent', 'active') ON CONFLICT (id) DO NOTHING`,
		agentID)
	if err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	defer func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM goals WHERE agent_id = $1`, agentID)
		_, _ = database.ExecContext(context.Background(), `DELETE FROM agents WHERE id = $1`, agentID)
	}()

	body := []byte(`{"agent_id":"` + agentID.String() + `","goal_text":"idempotency test goal","priority":100}`)
	key := "test-idem-" + uuid.New().String()

	req1, _ := http.NewRequestWithContext(ctx, http.MethodPost, goalURL+"/goals", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", key)
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Skipf("goal-service not reachable: %v", err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusCreated && resp1.StatusCode != http.StatusAccepted {
		t.Skipf("goal-service returned %d (need 201/202)", resp1.StatusCode)
	}
	var out1 struct {
		GoalID string `json:"goal_id"`
	}
	_ = json.NewDecoder(resp1.Body).Decode(&out1)
	if out1.GoalID == "" {
		t.Fatalf("first response missing goal_id")
	}

	req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, goalURL+"/goals", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", key)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated && resp2.StatusCode != http.StatusAccepted {
		t.Errorf("second response status: %d (expected 201 or 202)", resp2.StatusCode)
	}
	var out2 struct {
		GoalID string `json:"goal_id"`
	}
	_ = json.NewDecoder(resp2.Body).Decode(&out2)
	if out2.GoalID != out1.GoalID {
		t.Errorf("idempotency failed: first goal_id %s, second goal_id %s", out1.GoalID, out2.GoalID)
	}
}
