package rbac

import (
	"context"
	"database/sql"
)

// DBCollaboratorChecker implements CollaboratorChecker backed by the
// agent_collaborators table in Postgres.
type DBCollaboratorChecker struct {
	db *sql.DB
}

// NewDBCollaboratorChecker returns a checker that queries Postgres for
// collaborator grants.
func NewDBCollaboratorChecker(db *sql.DB) *DBCollaboratorChecker {
	return &DBCollaboratorChecker{db: db}
}

func (c *DBCollaboratorChecker) IsCollaborator(ctx context.Context, userID, agentID string) (bool, error) {
	var exists bool
	err := c.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM agent_collaborators
			WHERE agent_id = $1
			  AND collaborator_type = 'user'
			  AND collaborator_id = $2
		)`,
		agentID, userID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (c *DBCollaboratorChecker) IsCollaboratorViaTeam(ctx context.Context, teamIDs []string, agentID string) (bool, error) {
	if len(teamIDs) == 0 {
		return false, nil
	}
	arr := "{"
	for i, id := range teamIDs {
		if i > 0 {
			arr += ","
		}
		arr += id
	}
	arr += "}"

	var exists bool
	err := c.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM agent_collaborators
			WHERE agent_id = $1
			  AND collaborator_type = 'team'
			  AND collaborator_id = ANY($2::uuid[])
		)`,
		agentID, arr).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
