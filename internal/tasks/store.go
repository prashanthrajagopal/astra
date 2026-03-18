package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var ErrInvalidTransition = fmt.Errorf("invalid task state transition")

func nullableUUID(id uuid.UUID) interface{} {
	if id != uuid.Nil {
		return id
	}
	return nil
}

func parseNullableUUID(ns sql.NullString) uuid.UUID {
	if ns.Valid {
		id, _ := uuid.Parse(ns.String)
		return id
	}
	return uuid.Nil
}

func scanTaskRow(scanner interface{ Scan(...interface{}) error }) (*Task, error) {
	var t Task
	var goalID sql.NullString
	var idStr, graphIDStr, agentIDStr string
	err := scanner.Scan(&idStr, &graphIDStr, &goalID, &agentIDStr,
		&t.Type, &t.Status, &t.Payload, &t.Result,
		&t.Priority, &t.Retries, &t.MaxRetries, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	t.ID, _ = uuid.Parse(idStr)
	t.GraphID, _ = uuid.Parse(graphIDStr)
	t.AgentID, _ = uuid.Parse(agentIDStr)
	t.GoalID = parseNullableUUID(goalID)
	return &t, nil
}

// TaskStore is the interface used by the gRPC server. Implemented by Store and CachedStore.
type TaskStore interface {
	GetTask(ctx context.Context, taskID string) (*Task, error)
	GetGraph(ctx context.Context, graphID string) (*Graph, []Dependency, error)
	ListTasksByGoalID(ctx context.Context, goalID string) ([]*Task, error)
	CreateTask(ctx context.Context, t *Task) error
	AddDependencies(ctx context.Context, taskID string, dependsOn []string) error
	CreateGraph(ctx context.Context, graph *Graph) error
	Transition(ctx context.Context, taskID string, from, to Status, eventPayload json.RawMessage) error
	CompleteTask(ctx context.Context, taskID string, result []byte) error
	FailTask(ctx context.Context, taskID string, errMsg string) (movedToDeadLetter bool, err error)
	FindReadyTasks(ctx context.Context, limit int) ([]string, error)
	SetWorkerID(ctx context.Context, taskID, workerID string) error
	FindOrphanedRunningTasks(ctx context.Context) ([]string, error)
	RequeueTask(ctx context.Context, taskID string) error
	CancelTask(ctx context.Context, taskID string) error
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Transition(ctx context.Context, taskID string, from, to Status, eventPayload json.RawMessage) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tasks.Transition: begin: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE tasks SET status = $1, updated_at = now() WHERE id = $2 AND status = $3`,
		to, taskID, from)
	if err != nil {
		return fmt.Errorf("tasks.Transition: update: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrInvalidTransition
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO events (event_type, actor_id, payload, created_at) VALUES ($1, $2, $3, now())`,
		"Task"+string(to), taskID, eventPayload)
	if err != nil {
		return fmt.Errorf("tasks.Transition: event: %w", err)
	}

	return tx.Commit()
}

func (s *Store) CreateTask(ctx context.Context, t *Task) error {
	payload := t.Payload
	if payload == nil {
		payload = []byte("{}")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, graph_id, goal_id, agent_id, type, status, payload, priority, retries, max_retries)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)`,
		t.ID, t.GraphID, nullableUUID(t.GoalID), t.AgentID,
		t.Type, t.Status, payload, t.Priority, t.Retries, t.MaxRetries)
	if err != nil {
		return fmt.Errorf("tasks.CreateTask: %w", err)
	}
	return nil
}

func (s *Store) AddDependencies(ctx context.Context, taskID string, dependsOn []string) error {
	for _, depID := range dependsOn {
		depUUID, err := uuid.Parse(depID)
		if err != nil {
			return fmt.Errorf("tasks.AddDependencies: invalid depends_on %q: %w", depID, err)
		}
		taskUUID, err := uuid.Parse(taskID)
		if err != nil {
			return fmt.Errorf("tasks.AddDependencies: invalid task_id: %w", err)
		}
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO task_dependencies (task_id, depends_on) VALUES ($1, $2)`,
			taskUUID, depUUID)
		if err != nil {
			return fmt.Errorf("tasks.AddDependencies: %w", err)
		}
	}
	return nil
}

func (s *Store) CreateGraph(ctx context.Context, graph *Graph) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tasks.CreateGraph: begin: %w", err)
	}
	defer tx.Rollback()

	for i := range graph.Tasks {
		t := &graph.Tasks[i]
		payload := t.Payload
		if payload == nil {
			payload = []byte("{}")
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO tasks (id, graph_id, goal_id, agent_id, type, status, payload, priority, retries, max_retries)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)`,
			t.ID, t.GraphID, nullableUUID(t.GoalID), t.AgentID,
			t.Type, t.Status, payload, t.Priority, t.Retries, t.MaxRetries)
		if err != nil {
			return fmt.Errorf("tasks.CreateGraph: insert task %s: %w", t.ID, err)
		}
	}

	for _, d := range graph.Dependencies {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO task_dependencies (task_id, depends_on) VALUES ($1, $2)`,
			d.TaskID, d.DependsOn)
		if err != nil {
			return fmt.Errorf("tasks.CreateGraph: insert dep: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE tasks t SET status = $1, updated_at = now()
		WHERE t.graph_id = $2 AND t.status = $3
		AND NOT EXISTS (
			SELECT 1 FROM task_dependencies d WHERE d.task_id = t.id
		)`, StatusPending, graph.ID, StatusCreated)
	if err != nil {
		return fmt.Errorf("tasks.CreateGraph: transition: %w", err)
	}

	return tx.Commit()
}

func (s *Store) GetTask(ctx context.Context, taskID string) (*Task, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, graph_id, goal_id, agent_id, type, status, payload, result, priority, retries, max_retries, created_at, updated_at
		FROM tasks WHERE id = $1`, taskID)
	t, err := scanTaskRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("tasks.GetTask: %w", err)
	}
	return t, nil
}

func (s *Store) GetGraph(ctx context.Context, graphID string) (*Graph, []Dependency, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, graph_id, goal_id, agent_id, type, status, payload, result, priority, retries, max_retries, created_at, updated_at
		FROM tasks WHERE graph_id = $1`, graphID)
	if err != nil {
		return nil, nil, fmt.Errorf("tasks.GetGraph: tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	var graphIDUUID uuid.UUID
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, nil, fmt.Errorf("tasks.GetGraph: scan: %w", err)
		}
		graphIDUUID = t.GraphID
		tasks = append(tasks, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("tasks.GetGraph: %w", err)
	}

	if len(tasks) == 0 {
		return nil, nil, nil
	}

	placeholders := make([]string, len(tasks))
	args := make([]interface{}, len(tasks))
	for i := range tasks {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = tasks[i].ID
	}
	depRows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT task_id, depends_on FROM task_dependencies
		WHERE task_id IN (%s)`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, nil, fmt.Errorf("tasks.GetGraph: deps: %w", err)
	}
	defer depRows.Close()

	var deps []Dependency
	for depRows.Next() {
		var d Dependency
		var taskIDStr, dependsOnStr string
		if err := depRows.Scan(&taskIDStr, &dependsOnStr); err != nil {
			return nil, nil, fmt.Errorf("tasks.GetGraph: dep scan: %w", err)
		}
		d.TaskID, _ = uuid.Parse(taskIDStr)
		d.DependsOn, _ = uuid.Parse(dependsOnStr)
		deps = append(deps, d)
	}
	if err := depRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("tasks.GetGraph: %w", err)
	}

	return &Graph{ID: graphIDUUID, Tasks: tasks, Dependencies: deps}, deps, nil
}

// ListTasksByGoalID returns all tasks for a goal (for dashboard goal-detail modal).
func (s *Store) ListTasksByGoalID(ctx context.Context, goalID string) ([]*Task, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, graph_id, goal_id, agent_id, type, status, payload, result, priority, retries, max_retries, created_at, updated_at
		FROM tasks WHERE goal_id = $1 ORDER BY created_at`,
		goalID)
	if err != nil {
		return nil, fmt.Errorf("tasks.ListTasksByGoalID: %w", err)
	}
	defer rows.Close()
	var out []*Task
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, fmt.Errorf("tasks.ListTasksByGoalID scan: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks.ListTasksByGoalID: %w", err)
	}
	return out, nil
}

func (s *Store) CompleteTask(ctx context.Context, taskID string, result []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tasks.CompleteTask: begin: %w", err)
	}
	defer tx.Rollback()

	resultVal := result
	if resultVal == nil {
		resultVal = []byte("{}")
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE tasks SET status = $1, result = $2::jsonb, updated_at = now() WHERE id = $3 AND status = $4`,
		StatusCompleted, resultVal, taskID, StatusRunning)
	if err != nil {
		return fmt.Errorf("tasks.CompleteTask: update: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrInvalidTransition
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO events (event_type, actor_id, payload, created_at) VALUES ($1, $2, $3, now())`,
		"Task"+string(StatusCompleted), taskID, json.RawMessage(`{}`))
	if err != nil {
		return fmt.Errorf("tasks.CompleteTask: event: %w", err)
	}

	if err := promoteUnblockedTasks(tx, ctx, taskID); err != nil {
		return fmt.Errorf("tasks.CompleteTask: promote: %w", err)
	}

	return tx.Commit()
}

// promoteUnblockedTasks moves tasks from 'created' to 'pending' when all their
// dependencies are now completed. Called within the CompleteTask transaction.
func promoteUnblockedTasks(tx *sql.Tx, ctx context.Context, completedTaskID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE tasks SET status = 'pending', updated_at = now()
		WHERE status = 'created'
		AND id IN (
			SELECT d.task_id FROM task_dependencies d
			WHERE d.depends_on = $1
		)
		AND NOT EXISTS (
			SELECT 1 FROM task_dependencies d2
			JOIN tasks dep ON dep.id = d2.depends_on
			WHERE d2.task_id = tasks.id AND dep.status != 'completed'
		)`, completedTaskID)
	return err
}

// FailTask transitions a running task to either queued (retry) or dead_letter (final failure).
// It returns (true, nil) when the task was moved to dead_letter so callers can e.g. publish to astra:dead_letter.
func (s *Store) FailTask(ctx context.Context, taskID string, errMsg string) (movedToDeadLetter bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("tasks.FailTask: begin: %w", err)
	}
	defer tx.Rollback()

	var retries, maxRetries int
	err = tx.QueryRowContext(ctx, `SELECT retries, max_retries FROM tasks WHERE id = $1 AND status = $2`, taskID, StatusRunning).Scan(&retries, &maxRetries)
	if err == sql.ErrNoRows {
		return false, ErrInvalidTransition
	}
	if err != nil {
		return false, fmt.Errorf("tasks.FailTask: select: %w", err)
	}

	eventPayload := json.RawMessage(fmt.Sprintf(`{"error":%q}`, errMsg))
	if retries < maxRetries {
		_, err = tx.ExecContext(ctx,
			`UPDATE tasks SET status = $1, retries = retries + 1, updated_at = now() WHERE id = $2 AND status = $3`,
			StatusQueued, taskID, StatusRunning)
		if err != nil {
			return false, fmt.Errorf("tasks.FailTask: retry update: %w", err)
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO events (event_type, actor_id, payload, created_at) VALUES ($1, $2, $3, now())`,
			"TaskRetry", taskID, eventPayload)
		if err != nil {
			return false, fmt.Errorf("tasks.FailTask: event: %w", err)
		}
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE tasks SET status = $1, updated_at = now() WHERE id = $2 AND status = $3`,
			StatusDeadLetter, taskID, StatusRunning)
		if err != nil {
			return false, fmt.Errorf("tasks.FailTask: fail update: %w", err)
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO events (event_type, actor_id, payload, created_at) VALUES ($1, $2, $3, now())`,
			"TaskDeadLetter", taskID, eventPayload)
		if err != nil {
			return false, fmt.Errorf("tasks.FailTask: event: %w", err)
		}
		movedToDeadLetter = true
	}

	return movedToDeadLetter, tx.Commit()
}

func (s *Store) SetWorkerID(ctx context.Context, taskID, workerID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET worker_id = $1::uuid WHERE id = $2::uuid`,
		workerID, taskID)
	if err != nil {
		return fmt.Errorf("tasks.SetWorkerID: %w", err)
	}
	return nil
}

func (s *Store) FindOrphanedRunningTasks(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id FROM tasks t
		JOIN workers w ON w.id = t.worker_id
		WHERE t.status = 'running' AND w.status = 'offline'`)
	if err != nil {
		return nil, fmt.Errorf("tasks.FindOrphanedRunning: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("tasks.FindOrphanedRunning: scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) CancelTask(ctx context.Context, taskID string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = 'failed', result = '{"cancelled":true}'::jsonb, updated_at = now()
		 WHERE id = $1::uuid AND status NOT IN ('completed', 'failed')`,
		taskID)
	if err != nil {
		return fmt.Errorf("tasks.CancelTask: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrInvalidTransition
	}
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO events (event_type, actor_id, payload, created_at) VALUES ('TaskCancelled', $1, '{"cancelled":true}'::jsonb, now())`,
		taskID)
	return nil
}

func (s *Store) RequeueTask(ctx context.Context, taskID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tasks.RequeueTask: begin: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE tasks SET status = 'queued', worker_id = NULL, updated_at = now() WHERE id = $1 AND status = 'running'`,
		taskID)
	if err != nil {
		return fmt.Errorf("tasks.RequeueTask: update: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return nil
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO events (event_type, actor_id, payload, created_at) VALUES ('TaskRequeued', $1, '{}', now())`,
		taskID)
	if err != nil {
		return fmt.Errorf("tasks.RequeueTask: event: %w", err)
	}

	return tx.Commit()
}

// FailBlockedTasks cascade-fails tasks in 'created' or 'pending' status
// that have at least one dependency in 'failed' status. Returns count of affected tasks.
func (s *Store) FailBlockedTasks(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET status = 'failed', result = '{"error":"dependency failed"}'::jsonb, updated_at = now()
		WHERE status IN ('created', 'pending')
		AND EXISTS (
			SELECT 1 FROM task_dependencies d
			JOIN tasks dep ON dep.id = d.depends_on
			WHERE d.task_id = tasks.id AND dep.status = 'failed'
		)`)
	if err != nil {
		return 0, fmt.Errorf("tasks.FailBlockedTasks: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// FindGoalsToFinalize returns goal IDs where all tasks are in terminal states
// (completed or failed) but the goal is still 'active'.
func (s *Store) FindGoalsToFinalize(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT g.id FROM goals g
		WHERE g.status = 'active'
		AND NOT EXISTS (
			SELECT 1 FROM tasks t
			WHERE t.goal_id = g.id
			AND t.status NOT IN ('completed', 'failed')
		)
		AND EXISTS (
			SELECT 1 FROM tasks t WHERE t.goal_id = g.id
		)`)
	if err != nil {
		return nil, fmt.Errorf("tasks.FindGoalsToFinalize: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("tasks.FindGoalsToFinalize: scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AutoFinalizeGoal marks a goal as completed or failed based on its task outcomes.
func (s *Store) AutoFinalizeGoal(ctx context.Context, goalID string) error {
	var hasFailed bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM tasks WHERE goal_id = $1 AND status = 'failed')`,
		goalID).Scan(&hasFailed)
	if err != nil {
		return fmt.Errorf("tasks.AutoFinalizeGoal: check: %w", err)
	}
	goalStatus := "completed"
	if hasFailed {
		goalStatus = "failed"
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE goals SET status = $1 WHERE id = $2 AND status = 'active'`,
		goalStatus, goalID)
	if err != nil {
		return fmt.Errorf("tasks.AutoFinalizeGoal: update: %w", err)
	}
	return nil
}

// ReadyTask identifies a task and its agent for shard routing.
type ReadyTask struct {
	TaskID  string
	AgentID string
}

func (s *Store) FindReadyTasks(ctx context.Context, limit int) ([]string, error) {
	pairs, err := s.FindReadyTasksWithAgentIDs(ctx, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(pairs))
	for i := range pairs {
		ids[i] = pairs[i].TaskID
	}
	return ids, nil
}

func (s *Store) FindReadyTasksWithAgentIDs(ctx context.Context, limit int) ([]ReadyTask, error) {
	pairs, err := s.findReadyTasksByAgentPriorityWithAgentID(ctx, limit)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "priority") {
		return s.findReadyTasksFIFOWithAgentID(ctx, limit)
	}
	return pairs, err
}

func (s *Store) findReadyTasksByAgentPriorityWithAgentID(ctx context.Context, limit int) ([]ReadyTask, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.agent_id FROM tasks t
		JOIN agents a ON a.id = t.agent_id
		WHERE t.status = 'pending'
		AND NOT EXISTS (
			SELECT 1 FROM task_dependencies d
			JOIN tasks td ON td.id = d.depends_on
			WHERE d.task_id = t.id AND td.status != 'completed'
		)
		ORDER BY a.priority DESC, t.created_at ASC
		FOR UPDATE OF t SKIP LOCKED
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReadyTasks(rows)
}

func (s *Store) findReadyTasksFIFOWithAgentID(ctx context.Context, limit int) ([]ReadyTask, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.agent_id FROM tasks t
		WHERE t.status = 'pending'
		AND NOT EXISTS (
			SELECT 1 FROM task_dependencies d
			JOIN tasks td ON td.id = d.depends_on
			WHERE d.task_id = t.id AND td.status != 'completed'
		)
		ORDER BY t.created_at ASC
		FOR UPDATE OF t SKIP LOCKED
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("tasks.FindReady: %w", err)
	}
	defer rows.Close()
	return scanReadyTasks(rows)
}

func scanReadyTasks(rows *sql.Rows) ([]ReadyTask, error) {
	var out []ReadyTask
	for rows.Next() {
		var t ReadyTask
		if err := rows.Scan(&t.TaskID, &t.AgentID); err != nil {
			return nil, fmt.Errorf("tasks.scanReadyTasks: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}


// FindStaleQueuedTasks returns tasks stuck in 'queued' for more than 30 seconds
// (their Redis message was likely lost). Resets them to 'pending' for re-dispatch.
func (s *Store) RecoverStaleQueued(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET status = 'pending', updated_at = now()
		WHERE status = 'queued'
		AND updated_at < now() - interval '30 seconds'`)
	if err != nil {
		return 0, fmt.Errorf("tasks.RecoverStaleQueued: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
