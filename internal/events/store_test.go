package events

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"astra/pkg/db"

	"github.com/google/uuid"
)

func TestEventStore_AppendAndReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("skipping: TEST_DB_DSN not set")
	}

	conn, err := db.Connect(dsn)
	if err != nil {
		t.Skipf("skipping: cannot connect to DB: %v", err)
	}
	defer conn.Close()

	store := NewStore(conn)
	ctx := context.Background()
	actorID := uuid.New().String()

	ids := make([]int64, 3)
	for i := 0; i < 3; i++ {
		payload := json.RawMessage(fmt.Sprintf(`{"seq":%d}`, i))
		id, err := store.Append(ctx, "test_event", actorID, payload)
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		ids[i] = id
	}

	replayed, err := store.Replay(ctx, actorID, ids[0]-1)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(replayed) != 3 {
		t.Fatalf("expected 3 events, got %d", len(replayed))
	}
	for i, e := range replayed {
		if e.ActorID != actorID {
			t.Errorf("event %d: ActorID %s != %s", i, e.ActorID, actorID)
		}
		if e.EventType != "test_event" {
			t.Errorf("event %d: EventType %s != test_event", i, e.EventType)
		}
		if e.ID != ids[i] {
			t.Errorf("event %d: ID %d != %d", i, e.ID, ids[i])
		}
	}
}
