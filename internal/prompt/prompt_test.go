package prompt

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
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

func TestStore_GetPrompt_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	db := testDB(t)
	defer db.Close()
	store := NewStore(db)
	ctx := context.Background()

	p, err := store.GetPrompt(ctx, "nonexistent", "v1")
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if p != nil {
		t.Fatalf("expected nil prompt, got %+v", p)
	}
}

func TestStore_SavePrompt_GetPrompt_ListByName(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	db := testDB(t)
	defer db.Close()
	store := NewStore(db)
	ctx := context.Background()

	name, version := "test-prompt", "v1"
	body := "Hello {{.user}}"
	schema := []byte(`{"type":"object"}`)
	p := &Prompt{
		Name:            name,
		Version:         version,
		Body:            body,
		VariablesSchema: schema,
	}

	if err := store.SavePrompt(ctx, p); err != nil {
		t.Fatalf("SavePrompt: %v", err)
	}

	got, err := store.GetPrompt(ctx, name, version)
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if got == nil {
		t.Fatal("expected prompt, got nil")
	}
	if got.Name != name || got.Version != version || got.Body != body {
		t.Errorf("got Name=%q Version=%q Body=%q", got.Name, got.Version, got.Body)
	}
	if string(got.VariablesSchema) != string(schema) {
		t.Errorf("VariablesSchema: got %q", got.VariablesSchema)
	}

	list, err := store.ListByName(ctx, name)
	if err != nil {
		t.Fatalf("ListByName: %v", err)
	}
	if len(list) < 1 {
		t.Fatalf("ListByName: expected at least 1, got %d", len(list))
	}
	found := false
	for _, item := range list {
		if item.Version == version {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ListByName: did not find version %q", version)
	}

	// Cleanup
	_, _ = db.ExecContext(ctx, "DELETE FROM prompts WHERE name = $1 AND version = $2", name, version)
}

func TestStore_SavePrompt_Conflict(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Postgres")
	}
	db := testDB(t)
	defer db.Close()
	store := NewStore(db)
	ctx := context.Background()

	name, version := "conflict-prompt", "v1"
	p1 := &Prompt{Name: name, Version: version, Body: "first", VariablesSchema: []byte("{}")}
	if err := store.SavePrompt(ctx, p1); err != nil {
		t.Fatalf("SavePrompt first: %v", err)
	}

	p2 := &Prompt{Name: name, Version: version, Body: "second", VariablesSchema: []byte(`{"x":1}`)}
	if err := store.SavePrompt(ctx, p2); err != nil {
		t.Fatalf("SavePrompt second: %v", err)
	}

	got, err := store.GetPrompt(ctx, name, version)
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if got == nil || got.Body != "second" {
		t.Errorf("expected body 'second' after conflict update, got %+v", got)
	}

	_, _ = db.ExecContext(ctx, "DELETE FROM prompts WHERE name = $1 AND version = $2", name, version)
}
