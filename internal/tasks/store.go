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
	var goalID interface{}
	if t.GoalID != uuid.Nil {
		goalID = t.GoalID
	} else {
		goalID = nil
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, graph_id, goal_id, agent_id, type, status, payload, priority, retries, max_retries)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)`,
		t.ID, t.GraphID, goalID, t.AgentID, t.Type, t.Status, payload, t.Priority, t.Retries, t.MaxRetries)
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
		var goalID interface{}
		if t.GoalID != uuid.Nil {
			goalID = t.GoalID
		} else {
			goalID = nil
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO tasks (id, graph_id, goal_id, agent_id, type, status, payload, priority, retries, max_retries)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)`,
			t.ID, t.GraphID, goalID, t.AgentID, t.Type, t.Status, payload, t.Priority, t.Retries, t.MaxRetries)
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
	var t Task
	var goalID sql.NullString
	var idStr, graphIDStr, agentIDStr string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, graph_id, goal_id, agent_id, type, status, payload, result, priority, retries, max_retries, created_at, updated_at
		FROM tasks WHERE id = $1`, taskID).Scan(
		&idStr, &graphIDStr, &goalID, &agentIDStr, &t.Type, &t.Status, &t.Payload, &t.Result,
		&t.Priority, &t.Retries, &t.MaxRetries, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("tasks.GetTask: %w", err)
	}
	t.ID, _ = uuid.Parse(idStr)
	t.GraphID, _ = uuid.Parse(graphIDStr)
	t.AgentID, _ = uuid.Parse(agentIDStr)
	if goalID.Valid {
		t.GoalID, _ = uuid.Parse(goalID.String)
	}
	return &t, nil
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
		var t Task
		var goalID sql.NullString
		var idStr, graphIDStr, agentIDStr string
		err := rows.Scan(&idStr, &graphIDStr, &goalID, &agentIDStr, &t.Type, &t.Status, &t.Payload, &t.Result,
			&t.Priority, &t.Retries, &t.MaxRetries, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			return nil, nil, fmt.Errorf("tasks.GetGraph: scan: %w", err)
		}
		t.ID, _ = uuid.Parse(idStr)
		t.GraphID, _ = uuid.Parse(graphIDStr)
		t.AgentID, _ = uuid.Parse(agentIDStr)
		if goalID.Valid {
			t.GoalID, _ = uuid.Parse(goalID.String)
		}
		graphIDUUID = t.GraphID
		tasks = append(tasks, t)
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

	return tx.Commit()
}

func (s *Store) FailTask(ctx context.Context, taskID string, errMsg string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("tasks.FailTask: begin: %w", err)
	}
	defer tx.Rollback()

	var retries, maxRetries int
	err = tx.QueryRowContext(ctx, `SELECT retries, max_retries FROM tasks WHERE id = $1 AND status = $2`, taskID, StatusRunning).Scan(&retries, &maxRetries)
	if err == sql.ErrNoRows {
		return ErrInvalidTransition
	}
	if err != nil {
		return fmt.Errorf("tasks.FailTask: select: %w", err)
	}

	eventPayload := json.RawMessage(fmt.Sprintf(`{"error":%q}`, errMsg))
	if retries < maxRetries {
		_, err = tx.ExecContext(ctx,
			`UPDATE tasks SET status = $1, retries = retries + 1, updated_at = now() WHERE id = $2 AND status = $3`,
			StatusQueued, taskID, StatusRunning)
		if err != nil {
			return fmt.Errorf("tasks.FailTask: retry update: %w", err)
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO events (event_type, actor_id, payload, created_at) VALUES ($1, $2, $3, now())`,
			"TaskRetry", taskID, eventPayload)
		if err != nil {
			return fmt.Errorf("tasks.FailTask: event: %w", err)
		}
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE tasks SET status = $1, updated_at = now() WHERE id = $2 AND status = $3`,
			StatusFailed, taskID, StatusRunning)
		if err != nil {
			return fmt.Errorf("tasks.FailTask: fail update: %w", err)
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO events (event_type, actor_id, payload, created_at) VALUES ($1, $2, $3, now())`,
			"Task"+string(StatusFailed), taskID, eventPayload)
		if err != nil {
			return fmt.Errorf("tasks.FailTask: event: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) FindReadyTasks(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id FROM tasks t
		WHERE t.status = 'pending'
		AND NOT EXISTS (
			SELECT 1 FROM task_dependencies d
			JOIN tasks td ON td.id = d.depends_on
			WHERE d.task_id = t.id AND td.status != 'completed'
		)
		FOR UPDATE SKIP LOCKED
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("tasks.FindReady: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("tasks.FindReady: scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
