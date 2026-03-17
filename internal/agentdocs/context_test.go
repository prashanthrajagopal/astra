package agentdocs

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestSerializeContext_RoundTrip(t *testing.T) {
	ac := &AgentContext{
		SystemPrompt: "You are a helpful assistant.",
		Rules:        []Document{{Name: "r1", Content: ptrString("rule1")}},
		Skills:       []Document{{Name: "s1", Content: ptrString("skill1")}},
		ContextDocs:  []Document{{Name: "c1", Content: ptrString("ctx1")}},
	}
	data, err := SerializeContext(ac)
	if err != nil {
		t.Fatalf("SerializeContext: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty JSON")
	}
	var decoded AgentContext
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.SystemPrompt != ac.SystemPrompt {
		t.Errorf("system_prompt: want %q, got %q", ac.SystemPrompt, decoded.SystemPrompt)
	}
	if len(decoded.Rules) != 1 || decoded.Rules[0].Name != "r1" {
		t.Errorf("rules: want 1 rule named r1, got %+v", decoded.Rules)
	}
}

func TestSerializeContext_Nil(t *testing.T) {
	data, err := SerializeContext(nil)
	if err != nil {
		t.Fatalf("SerializeContext(nil): %v", err)
	}
	if data != nil {
		t.Errorf("expected nil, got %v", data)
	}
}

func TestAssembleContext_NoDocuments(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	db := setupTestDB(t)
	defer db.Close()

	store := NewStore(db, nil)
	ctx := context.Background()
	agentID := uuid.New()

	_, err := db.ExecContext(ctx, `INSERT INTO agents (id, name, status, system_prompt) VALUES ($1, 'empty-ctx', 'active', 'custom') ON CONFLICT (id) DO UPDATE SET system_prompt = 'custom'`, agentID)
	if err != nil {
		t.Fatalf("insert agent: %v", err)
	}

	ac, err := store.AssembleContext(ctx, agentID, nil)
	if err != nil {
		t.Fatalf("AssembleContext: %v", err)
	}
	if ac.SystemPrompt != "custom" {
		t.Errorf("system_prompt: want custom, got %q", ac.SystemPrompt)
	}
	if len(ac.Rules) != 0 || len(ac.Skills) != 0 || len(ac.ContextDocs) != 0 {
		t.Errorf("expected empty slices, got rules=%d skills=%d context=%d", len(ac.Rules), len(ac.Skills), len(ac.ContextDocs))
	}

	_, _ = db.ExecContext(ctx, `DELETE FROM agents WHERE id = $1`, agentID)
}

func TestAssembleContext_PrioritySorting(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	db := setupTestDB(t)
	defer db.Close()

	store := NewStore(db, nil)
	ctx := context.Background()
	agentID := uuid.New()

	_, err := db.ExecContext(ctx, `INSERT INTO agents (id, name, status) VALUES ($1, 'sort-test', 'active') ON CONFLICT (id) DO NOTHING`, agentID)
	if err != nil {
		t.Fatalf("insert agent: %v", err)
	}

	c1, c2, c3 := "first", "second", "third"
	for _, d := range []Document{
		{AgentID: agentID, DocType: DocTypeRule, Name: "r3", Content: &c3, Priority: 300},
		{AgentID: agentID, DocType: DocTypeRule, Name: "r1", Content: &c1, Priority: 100},
		{AgentID: agentID, DocType: DocTypeRule, Name: "r2", Content: &c2, Priority: 200},
	} {
		if err := store.CreateDocument(ctx, &d); err != nil {
			t.Fatalf("CreateDocument: %v", err)
		}
	}

	ac, err := store.AssembleContext(ctx, agentID, nil)
	if err != nil {
		t.Fatalf("AssembleContext: %v", err)
	}
	if len(ac.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(ac.Rules))
	}
	if ac.Rules[0].Name != "r1" || ac.Rules[1].Name != "r2" || ac.Rules[2].Name != "r3" {
		t.Errorf("rules should be sorted by priority: got %s, %s, %s", ac.Rules[0].Name, ac.Rules[1].Name, ac.Rules[2].Name)
	}

	for _, d := range ac.Rules {
		_ = store.DeleteDocument(ctx, d.ID)
	}
	_, _ = db.ExecContext(ctx, `DELETE FROM agents WHERE id = $1`, agentID)
}

func ptrString(s string) *string { return &s }

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "postgres://astra:changeme@localhost:5432/astra?sslmode=disable"
	if v := os.Getenv("POSTGRES_DSN"); v != "" {
		dsn = v
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
