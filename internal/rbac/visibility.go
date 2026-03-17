package rbac

import "context"

// AgentInfo carries minimal agent identity for compatibility (single-platform: no org/visibility).
type AgentInfo struct {
	ID string
}

// CollaboratorChecker is no longer used (single-platform; collaborators removed).
// Kept as a no-op interface for backward compatibility if referenced elsewhere.
type CollaboratorChecker interface {
	IsCollaborator(ctx context.Context, userID, agentID string) (bool, error)
	IsCollaboratorViaTeam(ctx context.Context, teamIDs []string, agentID string) (bool, error)
}

// CanAccessAgent returns true for any authenticated user (single-platform: all agents visible).
func CanAccessAgent(c Claims, agent AgentInfo, _ CollaboratorChecker) bool {
	return c.UserID != "" || c.IsSuperAdmin
}

// CanEditAgent returns true for any authenticated user (single-platform: all agents editable).
func CanEditAgent(c Claims, agent AgentInfo, _ CollaboratorChecker) bool {
	return c.UserID != "" || c.IsSuperAdmin
}

// FilterAgentList returns all agents (single-platform: no filtering).
func FilterAgentList(c Claims, agents []AgentInfo, _ CollaboratorChecker) []AgentInfo {
	return agents
}
