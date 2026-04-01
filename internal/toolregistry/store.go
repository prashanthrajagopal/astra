package toolregistry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ToolDefinition represents a tool registered in the tool_definitions table.
type ToolDefinition struct {
	ID          uuid.UUID       `json:"id,omitempty"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	RiskTier    string          `json:"risk_tier"`
	Sandbox     bool            `json:"sandbox"`
	Description string          `json:"description"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CreatedAt   string          `json:"created_at,omitempty"`
	UpdatedAt   string          `json:"updated_at,omitempty"`
}

// Store provides CRUD operations for tool definitions.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store backed by the given database connection.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new tool definition. Returns an error if (name, version) already exists.
func (s *Store) Create(ctx context.Context, td *ToolDefinition) error {
	if td.Name == "" || td.Version == "" {
		return fmt.Errorf("toolregistry: name and version are required")
	}
	if td.RiskTier == "" {
		td.RiskTier = "low"
	}

	metadata := td.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage("{}")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tool_definitions (name, version, risk_tier, sandbox, description, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb)`,
		td.Name, td.Version, td.RiskTier, td.Sandbox, td.Description, []byte(metadata))
	if err != nil {
		slog.Error("toolregistry: create failed", "name", td.Name, "version", td.Version, "err", err)
		return fmt.Errorf("toolregistry: create %s@%s: %w", td.Name, td.Version, err)
	}
	return nil
}

// Get retrieves a tool definition by name and version.
func (s *Store) Get(ctx context.Context, name, version string) (*ToolDefinition, error) {
	var td ToolDefinition
	var description sql.NullString
	var metadata []byte
	var createdAt, updatedAt sql.NullTime

	err := s.db.QueryRowContext(ctx,
		`SELECT name, version, risk_tier, sandbox, COALESCE(description, ''),
		        COALESCE(metadata, '{}'::jsonb), created_at, updated_at
		 FROM tool_definitions WHERE name = $1 AND version = $2`,
		name, version).Scan(
		&td.Name, &td.Version, &td.RiskTier, &td.Sandbox, &description,
		&metadata, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("toolregistry: tool %s@%s not found", name, version)
	}
	if err != nil {
		slog.Error("toolregistry: get failed", "name", name, "version", version, "err", err)
		return nil, fmt.Errorf("toolregistry: get %s@%s: %w", name, version, err)
	}

	if description.Valid {
		td.Description = description.String
	}
	if len(metadata) > 0 {
		td.Metadata = json.RawMessage(metadata)
	}
	if createdAt.Valid {
		td.CreatedAt = createdAt.Time.UTC().Format(time.RFC3339)
	}
	if updatedAt.Valid {
		td.UpdatedAt = updatedAt.Time.UTC().Format(time.RFC3339)
	}
	return &td, nil
}

// List returns all tool definitions, optionally filtered by risk_tier when riskTier is non-empty.
func (s *Store) List(ctx context.Context, riskTier string) ([]ToolDefinition, error) {
	var (
		rows *sql.Rows
		err  error
	)
	query := `SELECT name, version, risk_tier, sandbox, COALESCE(description, ''),
	                 COALESCE(metadata, '{}'::jsonb), created_at, updated_at
	          FROM tool_definitions`
	if riskTier != "" {
		rows, err = s.db.QueryContext(ctx, query+` WHERE risk_tier = $1 ORDER BY name, version`, riskTier)
	} else {
		rows, err = s.db.QueryContext(ctx, query+` ORDER BY name, version`)
	}
	if err != nil {
		slog.Error("toolregistry: list failed", "risk_tier", riskTier, "err", err)
		return nil, fmt.Errorf("toolregistry: list: %w", err)
	}
	defer rows.Close()

	var tools []ToolDefinition
	for rows.Next() {
		var td ToolDefinition
		var description sql.NullString
		var metadata []byte
		var createdAt, updatedAt sql.NullTime

		if err := rows.Scan(&td.Name, &td.Version, &td.RiskTier, &td.Sandbox, &description,
			&metadata, &createdAt, &updatedAt); err != nil {
			slog.Error("toolregistry: scan row failed", "err", err)
			continue
		}
		if description.Valid {
			td.Description = description.String
		}
		if len(metadata) > 0 {
			td.Metadata = json.RawMessage(metadata)
		}
		if createdAt.Valid {
			td.CreatedAt = createdAt.Time.UTC().Format(time.RFC3339)
		}
		if updatedAt.Valid {
			td.UpdatedAt = updatedAt.Time.UTC().Format(time.RFC3339)
		}
		tools = append(tools, td)
	}
	if err := rows.Err(); err != nil {
		slog.Error("toolregistry: list rows error", "err", err)
		return nil, fmt.Errorf("toolregistry: list rows: %w", err)
	}
	return tools, nil
}

// Update modifies an existing tool definition. Only the keys present in updates are changed.
// Supported keys: risk_tier, sandbox, description, metadata.
func (s *Store) Update(ctx context.Context, name, version string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	setClauses := make([]string, 0, len(updates)+1)
	args := make([]interface{}, 0, len(updates)+2)
	idx := 1

	for k, v := range updates {
		switch k {
		case "risk_tier", "sandbox", "description":
			setClauses = append(setClauses, fmt.Sprintf("%s = $%d", k, idx))
			args = append(args, v)
			idx++
		case "metadata":
			setClauses = append(setClauses, fmt.Sprintf("metadata = $%d::jsonb", idx))
			switch val := v.(type) {
			case json.RawMessage:
				args = append(args, []byte(val))
			case []byte:
				args = append(args, val)
			case string:
				args = append(args, val)
			default:
				b, err := json.Marshal(v)
				if err != nil {
					return fmt.Errorf("toolregistry: marshal metadata: %w", err)
				}
				args = append(args, b)
			}
			idx++
		default:
			return fmt.Errorf("toolregistry: unsupported update field: %s", k)
		}
	}

	if len(setClauses) == 0 {
		return nil
	}

	// Append WHERE args
	args = append(args, name, version)
	query := fmt.Sprintf(
		`UPDATE tool_definitions SET %s WHERE name = $%d AND version = $%d`,
		strings.Join(setClauses, ", "), idx, idx+1,
	)

	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		slog.Error("toolregistry: update failed", "name", name, "version", version, "err", err)
		return fmt.Errorf("toolregistry: update %s@%s: %w", name, version, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("toolregistry: tool %s@%s not found", name, version)
	}
	return nil
}

// Delete removes a tool definition identified by name and version.
func (s *Store) Delete(ctx context.Context, name, version string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM tool_definitions WHERE name = $1 AND version = $2`, name, version)
	if err != nil {
		slog.Error("toolregistry: delete failed", "name", name, "version", version, "err", err)
		return fmt.Errorf("toolregistry: delete %s@%s: %w", name, version, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("toolregistry: tool %s@%s not found", name, version)
	}
	return nil
}

// GetRiskTier returns the highest risk_tier for any version of the given tool name.
// Returns "low" when no matching tool is found.
func (s *Store) GetRiskTier(ctx context.Context, toolName string) (string, error) {
	// Return the most restrictive tier across all versions of the tool.
	var tier string
	err := s.db.QueryRowContext(ctx,
		`SELECT risk_tier FROM tool_definitions
		 WHERE name = $1
		 ORDER BY CASE risk_tier
		   WHEN 'critical' THEN 0
		   WHEN 'high'     THEN 1
		   WHEN 'medium'   THEN 2
		   WHEN 'low'      THEN 3
		   ELSE 4
		 END ASC
		 LIMIT 1`,
		toolName).Scan(&tier)
	if err == sql.ErrNoRows {
		return "low", nil
	}
	if err != nil {
		slog.Error("toolregistry: get risk tier failed", "tool", toolName, "err", err)
		return "", fmt.Errorf("toolregistry: get risk tier %s: %w", toolName, err)
	}
	return tier, nil
}
