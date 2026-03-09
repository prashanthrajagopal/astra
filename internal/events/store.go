package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Append(ctx context.Context, eventType string, actorID string, payload json.RawMessage) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO events (event_type, actor_id, payload, created_at) VALUES ($1, $2, $3, now()) RETURNING id`,
		eventType, actorID, payload).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("events.Append: %w", err)
	}
	return id, nil
}

func (s *Store) Replay(ctx context.Context, actorID string, fromID int64) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, event_type, actor_id, payload, created_at FROM events WHERE actor_id = $1 AND id > $2 ORDER BY id`,
		actorID, fromID)
	if err != nil {
		return nil, fmt.Errorf("events.Replay: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.EventType, &e.ActorID, &e.Payload, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("events.Replay: scan: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

type Event struct {
	ID        int64
	EventType string
	ActorID   string
	Payload   json.RawMessage
	CreatedAt string
}
