package goals

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"astra/internal/messaging"
)

// Store holds the database and messaging bus for goal dependency operations.
type Store struct {
	db  *sql.DB
	bus *messaging.Bus
}

// NewStore creates a Store backed by the given database connection and messaging bus.
func NewStore(db *sql.DB, bus *messaging.Bus) *Store {
	return &Store{db: db, bus: bus}
}

// CheckAndActivateBlocked finds all goals whose depends_on_goal_ids contains
// completedGoalID, checks whether all of their dependencies are now completed,
// and sets status = 'active' for any that are fully unblocked.
func (s *Store) CheckAndActivateBlocked(ctx context.Context, completedGoalID uuid.UUID) error {
	// Find all goals that list completedGoalID in their depends_on_goal_ids and
	// are still in 'blocked' or 'pending' state awaiting dependencies.
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, depends_on_goal_ids
		FROM goals
		WHERE $1::uuid = ANY(depends_on_goal_ids)
		  AND status IN ('blocked', 'pending')
	`, completedGoalID.String())
	if err != nil {
		return fmt.Errorf("goals.CheckAndActivateBlocked: query dependents: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		id   uuid.UUID
		deps []uuid.UUID
	}
	var candidates []candidate
	for rows.Next() {
		var idStr string
		var arrayLiteral string
		if err := rows.Scan(&idStr, &arrayLiteral); err != nil {
			return fmt.Errorf("goals.CheckAndActivateBlocked: scan row: %w", err)
		}
		goalID, err := uuid.Parse(idStr)
		if err != nil {
			return fmt.Errorf("goals.CheckAndActivateBlocked: parse goal id %q: %w", idStr, err)
		}
		depStrs, err := parseTextArrayLiteral(arrayLiteral)
		if err != nil {
			return fmt.Errorf("goals.CheckAndActivateBlocked: parse deps for goal %s: %w", goalID, err)
		}
		c := candidate{id: goalID}
		for _, ds := range depStrs {
			parsed, err := uuid.Parse(ds)
			if err != nil {
				return fmt.Errorf("goals.CheckAndActivateBlocked: parse dep uuid %q: %w", ds, err)
			}
			c.deps = append(c.deps, parsed)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("goals.CheckAndActivateBlocked: rows iteration: %w", err)
	}

	for _, c := range candidates {
		allDone, err := s.allDepsCompleted(ctx, c.deps)
		if err != nil {
			return fmt.Errorf("goals.CheckAndActivateBlocked: check deps for goal %s: %w", c.id, err)
		}
		if !allDone {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `
			UPDATE goals SET status = 'active' WHERE id = $1 AND status IN ('blocked', 'pending')
		`, c.id.String()); err != nil {
			return fmt.Errorf("goals.CheckAndActivateBlocked: activate goal %s: %w", c.id, err)
		}
		slog.Info("goal activated after dependencies completed",
			"goal_id", c.id,
			"triggered_by", completedGoalID,
		)
	}
	return nil
}

// allDepsCompleted returns true when every goal in depIDs has status = 'completed'.
func (s *Store) allDepsCompleted(ctx context.Context, depIDs []uuid.UUID) (bool, error) {
	if len(depIDs) == 0 {
		return true, nil
	}
	// Pass the dep list as a PostgreSQL UUID array literal cast inline so we
	// avoid importing lib/pq while still using a single parameterised query.
	var allDone bool
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) = 0
		FROM goals
		WHERE id = ANY($1::uuid[])
		  AND status != 'completed'
	`, uuidSliceToArrayLiteral(depIDs)).Scan(&allDone)
	if err != nil {
		return false, fmt.Errorf("goals.allDepsCompleted: %w", err)
	}
	return allDone, nil
}

// PublishGoalCompleted publishes a GoalCompleted event to the Redis stream
// "astra:goals:completed" and then triggers dependency activation.
func (s *Store) PublishGoalCompleted(ctx context.Context, goalID, cascadeID uuid.UUID, status, resultSummary string) error {
	fields := map[string]interface{}{
		"goal_id":        goalID.String(),
		"cascade_id":     cascadeID.String(),
		"status":         status,
		"result_summary": resultSummary,
		"timestamp":      time.Now().Unix(),
	}
	if err := s.bus.Publish(ctx, "astra:goals:completed", fields); err != nil {
		return fmt.Errorf("goals.PublishGoalCompleted: publish: %w", err)
	}
	slog.Info("goal completed event published",
		"goal_id", goalID,
		"cascade_id", cascadeID,
		"status", status,
	)
	if status == "completed" {
		if err := s.CheckAndActivateBlocked(ctx, goalID); err != nil {
			// Log but do not fail the publish — activation is best-effort and
			// will be retried by a reconciliation loop.
			slog.Error("failed to activate blocked goals after completion",
				"goal_id", goalID,
				"err", err,
			)
		}
	}
	return nil
}

// parseTextArrayLiteral parses a PostgreSQL text array literal of the form
// {elem1,elem2,...} into a []string. Handles NULL and empty arrays.
func parseTextArrayLiteral(s string) ([]string, error) {
	if s == "" || s == "NULL" {
		return nil, nil
	}
	if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
		return nil, fmt.Errorf("not a valid array literal: %q", s)
	}
	inner := s[1 : len(s)-1]
	if inner == "" {
		return []string{}, nil
	}
	// Simple split on comma — sufficient for UUID values which contain no commas.
	var result []string
	start := 0
	for i := 0; i <= len(inner); i++ {
		if i == len(inner) || inner[i] == ',' {
			result = append(result, inner[start:i])
			start = i + 1
		}
	}
	return result, nil
}

// uuidSliceToArrayLiteral converts a []uuid.UUID to a PostgreSQL array literal
// string (e.g. "{uuid1,uuid2}") suitable for use in $N::uuid[] casts.
func uuidSliceToArrayLiteral(ids []uuid.UUID) string {
	if len(ids) == 0 {
		return "{}"
	}
	out := make([]byte, 0, 2+len(ids)*37)
	out = append(out, '{')
	for i, id := range ids {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, id.String()...)
	}
	out = append(out, '}')
	return string(out)
}
