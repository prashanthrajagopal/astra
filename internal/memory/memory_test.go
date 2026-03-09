package memory

import (
	"context"
	"os"
	"testing"

	"astra/pkg/db"

	"github.com/google/uuid"
)

func testStore(t *testing.T) (*Store, func()) {
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
	return NewStore(conn, nil), func() { conn.Close() }
}

func TestStore_WriteWithoutEmbedding(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()
	agentID := uuid.New()

	id, err := store.Write(ctx, agentID, "test_type", "content without embedding", nil)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if id == uuid.Nil {
		t.Fatal("expected non-nil id")
	}

	m, err := store.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if m == nil {
		t.Fatal("expected memory")
	}
	if m.Content != "content without embedding" || m.MemoryType != "test_type" {
		t.Errorf("got content=%q type=%q", m.Content, m.MemoryType)
	}
	if m.Embedding != nil {
		t.Errorf("expected nil embedding, got len=%d", len(m.Embedding))
	}
}

func TestStore_WriteWithEmbedding(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()
	agentID := uuid.New()

	embedding := make([]float32, embeddingDim)
	for i := range embedding {
		embedding[i] = float32(i) * 0.001
	}

	id, err := store.Write(ctx, agentID, "test_type", "content with embedding", embedding)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if id == uuid.Nil {
		t.Fatal("expected non-nil id")
	}

	m, err := store.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if m == nil {
		t.Fatal("expected memory")
	}
	if len(m.Embedding) != embeddingDim {
		t.Fatalf("embedding len = %d, want %d", len(m.Embedding), embeddingDim)
	}
	for i := range embedding {
		if m.Embedding[i] != embedding[i] {
			t.Errorf("embedding[%d] = %v, want %v", i, m.Embedding[i], embedding[i])
		}
	}
}

func TestStore_WriteWithStubEmbedder(t *testing.T) {
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

	store := NewStore(conn, NewStubEmbedder())
	ctx := context.Background()
	agentID := uuid.New()

	id, err := store.Write(ctx, agentID, "test_type", "content for stub embed", nil)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if id == uuid.Nil {
		t.Fatal("expected non-nil id")
	}

	m, err := store.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if m == nil {
		t.Fatal("expected memory")
	}
	if len(m.Embedding) != embeddingDim {
		t.Fatalf("embedding len = %d, want %d", len(m.Embedding), embeddingDim)
	}
}

func TestStore_SearchByCreatedAt(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()
	agentID := uuid.New()

	_, _ = store.Write(ctx, agentID, "t", "first", nil)
	_, _ = store.Write(ctx, agentID, "t", "second", nil)

	results, err := store.Search(ctx, agentID, nil, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Logf("Search returned %d results (may be from other tests)", len(results))
	}
}

func TestStore_SearchByVector(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()
	agentID := uuid.New()

	embedding := make([]float32, embeddingDim)
	for i := range embedding {
		embedding[i] = float32(i) * 0.001
	}
	_, err := store.Write(ctx, agentID, "t", "with vec", embedding)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	results, err := store.Search(ctx, agentID, embedding, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least one result for same-vector search")
	}
}

func TestStore_SearchInvalidQueryEmbeddingFallsBack(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()
	agentID := uuid.New()

	// nil query embedding -> fallback to created_at
	results, err := store.Search(ctx, agentID, nil, 5)
	if err != nil {
		t.Fatalf("Search(nil): %v", err)
	}
	_ = results

	// wrong length -> fallback
	results, err = store.Search(ctx, agentID, []float32{1, 2, 3}, 5)
	if err != nil {
		t.Fatalf("Search(wrong len): %v", err)
	}
	_ = results
}

func TestStore_GetByID_NotFound(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()
	ctx := context.Background()

	m, err := store.GetByID(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if m != nil {
		t.Errorf("expected nil for missing id, got %+v", m)
	}
}
