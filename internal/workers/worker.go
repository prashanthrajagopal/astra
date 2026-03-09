package workers

import (
	"context"
	"log/slog"
	"time"

	"astra/internal/messaging"

	"github.com/google/uuid"
)

type Worker struct {
	ID       uuid.UUID
	Hostname string
	bus      *messaging.Bus
}

func New(hostname string, bus *messaging.Bus) *Worker {
	return &Worker{
		ID:       uuid.New(),
		Hostname: hostname,
		bus:      bus,
	}
}

func (w *Worker) StartHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := w.bus.Publish(ctx, "astra:worker:events", map[string]interface{}{
				"worker_id":  w.ID.String(),
				"event_type": "Heartbeat",
				"timestamp":  time.Now().Format(time.RFC3339),
			})
			if err != nil {
				slog.Error("heartbeat publish failed", "worker_id", w.ID, "err", err)
			}
		}
	}
}
