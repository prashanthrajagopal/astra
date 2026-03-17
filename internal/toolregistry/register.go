package toolregistry

import (
	"context"
	"database/sql"
)

// SeedKnownTools upserts built-in tool_definitions (idempotent).
func SeedKnownTools(ctx context.Context, db *sql.DB) error {
	tools := []struct {
		name, ver, tier, desc string
		sandbox               bool
	}{
		{"file_write", "1", "medium", "Write file under workspace", false},
		{"file_read", "1", "low", "Read file from workspace", false},
		{"shell_exec", "1", "high", "Execute shell in workspace", true},
		{"list_files", "1", "low", "List workspace files", false},
		{"code_generate", "1", "medium", "LLM codegen task", false},
		{"browser:screenshot", "1", "medium", "Browser screenshot", true},
	}
	for _, t := range tools {
		_, err := db.ExecContext(ctx, `
			INSERT INTO tool_definitions (name, version, risk_tier, sandbox, description)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (name, version) DO UPDATE SET risk_tier = EXCLUDED.risk_tier, sandbox = EXCLUDED.sandbox, description = EXCLUDED.description`,
			t.name, t.ver, t.tier, t.sandbox, t.desc)
		if err != nil {
			return err
		}
	}
	return nil
}
