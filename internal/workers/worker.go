package workers

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"astra/internal/messaging"
	"astra/pkg/metrics"

	"github.com/google/uuid"
)

type Worker struct {
	ID       uuid.UUID
	Hostname string
	bus      *messaging.Bus
	registry *Registry
	db       *sql.DB
}

// New creates a Worker without DB-backed registration.
func New(hostname string, bus *messaging.Bus) *Worker {
	return &Worker{
		ID:       uuid.New(),
		Hostname: hostname,
		bus:      bus,
	}
}

// NewWithDB creates a Worker with DB-backed registration. When db is present,
// StartHeartbeat will Register on first heartbeat and UpdateHeartbeat on each tick.
func NewWithDB(hostname string, bus *messaging.Bus, db *sql.DB) *Worker {
	return &Worker{
		ID:       uuid.New(),
		Hostname: hostname,
		bus:      bus,
		registry: NewRegistry(db),
		db:       db,
	}
}

func (w *Worker) StartHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	firstTick := true
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.doHeartbeat(ctx, &firstTick)
		}
	}
}

func (w *Worker) doHeartbeat(ctx context.Context, firstTick *bool) {
	if w.registry != nil {
		if *firstTick {
			if err := w.registry.Register(ctx, w.ID.String(), w.Hostname, nil); err != nil {
				slog.Error("registry register failed", "worker_id", w.ID, "err", err)
			}
			*firstTick = false
		} else {
			if err := w.registry.UpdateHeartbeat(ctx, w.ID.String()); err != nil {
				slog.Error("registry heartbeat update failed", "worker_id", w.ID, "err", err)
			}
		}
	}

	if err := w.bus.Publish(ctx, "astra:worker:events", map[string]interface{}{
		"worker_id":  w.ID.String(),
		"event_type": "Heartbeat",
		"timestamp":  time.Now().Format(time.RFC3339),
	}); err != nil {
		slog.Error("heartbeat publish failed", "worker_id", w.ID, "err", err)
	} else {
		metrics.WorkerHeartbeatTotal.Inc()
	}
}
