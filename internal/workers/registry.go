package workers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// WorkerInfo holds worker metadata from the registry.
type WorkerInfo struct {
	ID            string
	Hostname      string
	Status        string
	Capabilities  []string
	LastHeartbeat time.Time
}

// Registry provides DB-backed worker registration and status tracking.
type Registry struct {
	db *sql.DB
}

// NewRegistry creates a new Registry backed by the given database.
func NewRegistry(db *sql.DB) *Registry {
	return &Registry{db: db}
}

// Register inserts or updates a worker in the registry.
func (r *Registry) Register(ctx context.Context, workerID, hostname string, capabilities []string) error {
	if capabilities == nil {
		capabilities = []string{}
	}
	capJSON, err := json.Marshal(capabilities)
	if err != nil {
		return fmt.Errorf("workers.Registry.Register: marshal capabilities: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO workers (id, hostname, status, capabilities, last_heartbeat)
		VALUES ($1::uuid, $2, 'active', $3::jsonb, now())
		ON CONFLICT (id) DO UPDATE SET
			status = 'active',
			hostname = $2,
			capabilities = $3::jsonb,
			last_heartbeat = now()
	`, workerID, hostname, capJSON)
	if err != nil {
		return fmt.Errorf("workers.Registry.Register: %w", err)
	}
	return nil
}

// UpdateHeartbeat updates the last_heartbeat for a worker.
func (r *Registry) UpdateHeartbeat(ctx context.Context, workerID string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE workers SET last_heartbeat = now() WHERE id = $1::uuid`, workerID)
	if err != nil {
		return fmt.Errorf("workers.Registry.UpdateHeartbeat: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("workers.Registry.UpdateHeartbeat: worker not found: %s", workerID)
	}
	return nil
}

// MarkOffline sets a worker's status to offline.
func (r *Registry) MarkOffline(ctx context.Context, workerID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE workers SET status = 'offline' WHERE id = $1::uuid`, workerID)
	if err != nil {
		return fmt.Errorf("workers.Registry.MarkOffline: %w", err)
	}
	return nil
}

// ListActive returns all workers with status 'active'.
func (r *Registry) ListActive(ctx context.Context) ([]WorkerInfo, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, hostname, status, capabilities, last_heartbeat
		FROM workers
		WHERE status = 'active'
	`)
	if err != nil {
		return nil, fmt.Errorf("workers.Registry.ListActive: %w", err)
	}
	defer rows.Close()

	var result []WorkerInfo
	for rows.Next() {
		var info WorkerInfo
		var capJSON []byte
		if err := rows.Scan(&info.ID, &info.Hostname, &info.Status, &capJSON, &info.LastHeartbeat); err != nil {
			return nil, fmt.Errorf("workers.Registry.ListActive: scan: %w", err)
		}
		if len(capJSON) > 0 {
			_ = json.Unmarshal(capJSON, &info.Capabilities)
		}
		result = append(result, info)
	}
	return result, rows.Err()
}

// ListActiveByOrg returns active workers scoped to a specific organization.
func (r *Registry) ListActiveByOrg(ctx context.Context, orgID string) ([]WorkerInfo, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, hostname, status, capabilities, last_heartbeat
		FROM workers
		WHERE status = 'active' AND org_id = $1::uuid
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("workers.Registry.ListActiveByOrg: %w", err)
	}
	defer rows.Close()

	var result []WorkerInfo
	for rows.Next() {
		var info WorkerInfo
		var capJSON []byte
		if err := rows.Scan(&info.ID, &info.Hostname, &info.Status, &capJSON, &info.LastHeartbeat); err != nil {
			return nil, fmt.Errorf("workers.Registry.ListActiveByOrg: scan: %w", err)
		}
		if len(capJSON) > 0 {
			_ = json.Unmarshal(capJSON, &info.Capabilities)
		}
		result = append(result, info)
	}
	return result, rows.Err()
}

// FindStaleWorkers returns worker IDs that are active but whose last heartbeat
// is older than staleDuration.
func (r *Registry) FindStaleWorkers(ctx context.Context, staleDuration time.Duration) ([]string, error) {
	cutoff := time.Now().Add(-staleDuration)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM workers
		WHERE status = 'active' AND last_heartbeat < $1
	`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("workers.Registry.FindStaleWorkers: %w", err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("workers.Registry.FindStaleWorkers: scan: %w", err)
		}
		result = append(result, id)
	}
	return result, rows.Err()
}
