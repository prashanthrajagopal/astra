package agentdocs

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://astra:changeme@localhost:5432/astra?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skip("Postgres unavailable:", err)
	}
	if err := db.Ping(); err != nil {
		t.Skip("Postgres unavailable:", err)
	}
	return db
}

func TestStore_NilRedis_GetProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	db := testDB(t)
	defer db.Close()

	store := NewStore(db, nil)
	ctx := context.Background()
	agentID := uuid.New()

	profile, err := store.GetProfile(ctx, agentID)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if profile != nil {
		t.Errorf("expected nil profile for nonexistent agent, got %v", profile)
	}
}

func TestStore_NilRedis_ListDocuments(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	db := testDB(t)
	defer db.Close()

	store := NewStore(db, nil)
	ctx := context.Background()
	agentID := uuid.New()

	docs, err := store.ListDocuments(ctx, agentID, ListOptions{})
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected empty docs for new agent, got %d", len(docs))
	}
}

func TestStore_DocumentCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	db := testDB(t)
	defer db.Close()

	store := NewStore(db, nil)
	ctx := context.Background()
	agentID := uuid.New()

	// Ensure agent exists for FK
	_, err := db.ExecContext(ctx, `INSERT INTO agents (id, name, status) VALUES ($1, 'test-agent', 'active') ON CONFLICT (id) DO NOTHING`, agentID)
	if err != nil {
		t.Fatalf("insert agent: %v", err)
	}

	content := "test rule content"
	doc := &Document{
		AgentID:  agentID,
		DocType:  DocTypeRule,
		Name:     "test-rule",
		Content:  &content,
		Priority: 50,
	}
	if err := store.CreateDocument(ctx, doc); err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if doc.ID == uuid.Nil {
		t.Error("CreateDocument should set ID")
	}

	opts := ListOptions{GlobalOnly: true}
	docs, err := store.ListDocuments(ctx, agentID, opts)
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if docs[0].Name != "test-rule" || string(docs[0].DocType) != "rule" {
		t.Errorf("unexpected doc: %+v", docs[0])
	}

	dt := DocTypeSkill
	optsFilter := ListOptions{DocType: &dt}
	skillDocs, err := store.ListDocuments(ctx, agentID, optsFilter)
	if err != nil {
		t.Fatalf("ListDocuments with filter: %v", err)
	}
	if len(skillDocs) != 0 {
		t.Errorf("expected 0 skill docs, got %d", len(skillDocs))
	}

	if err := store.DeleteDocument(ctx, doc.ID); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	docsAfter, _ := store.ListDocuments(ctx, agentID, ListOptions{})
	if len(docsAfter) != 0 {
		t.Errorf("after delete expected 0 docs, got %d", len(docsAfter))
	}

	_, _ = db.ExecContext(ctx, `DELETE FROM agents WHERE id = $1`, agentID)
}

func TestStore_ProfileUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	db := testDB(t)
	defer db.Close()

	store := NewStore(db, nil)
	ctx := context.Background()
	agentID := uuid.New()

	_, err := db.ExecContext(ctx, `INSERT INTO agents (id, name, status) VALUES ($1, 'profile-test', 'active') ON CONFLICT (id) DO NOTHING`, agentID)
	if err != nil {
		t.Fatalf("insert agent: %v", err)
	}

	sp := "you are helpful"
	if err := store.UpdateProfile(ctx, agentID, &sp, nil); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	profile, err := store.GetProfile(ctx, agentID)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if profile == nil {
		t.Fatal("expected profile")
	}
	if profile.SystemPrompt != "you are helpful" {
		t.Errorf("system_prompt: want %q, got %q", "you are helpful", profile.SystemPrompt)
	}

	_, _ = db.ExecContext(ctx, `DELETE FROM agents WHERE id = $1`, agentID)
}

func TestStore_WithRedis(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres and Redis")
	}
	db := testDB(t)
	defer db.Close()

	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skip("Redis unavailable:", err)
	}
	defer rdb.Close()

	store := NewStore(db, rdb)
	store.ttl = 10 * time.Second
	ctx := context.Background()
	agentID := uuid.New()

	_, err := db.ExecContext(ctx, `INSERT INTO agents (id, name, status) VALUES ($1, 'redis-test', 'active') ON CONFLICT (id) DO NOTHING`, agentID)
	if err != nil {
		t.Fatalf("insert agent: %v", err)
	}

	profile, err := store.GetProfile(ctx, agentID)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if profile == nil {
		t.Fatal("expected profile")
	}

	// Second call should hit cache
	profile2, err := store.GetProfile(ctx, agentID)
	if err != nil {
		t.Fatalf("GetProfile (cached): %v", err)
	}
	if profile2.ID != profile.ID {
		t.Error("cached profile should match")
	}

	_, _ = db.ExecContext(ctx, `DELETE FROM agents WHERE id = $1`, agentID)
}
