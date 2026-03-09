package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
