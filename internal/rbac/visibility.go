package rbac

import "context"

// AgentInfo carries the fields required for visibility and ownership checks.
type AgentInfo struct {
	ID         string
	OrgID      string // empty for global agents
	OwnerID    string
	TeamID     string
	Visibility string // global, public, team, private
}

// CollaboratorChecker abstracts the collaborator lookup so the RBAC layer
// does not depend on a concrete database implementation.
type CollaboratorChecker interface {
	IsCollaborator(ctx context.Context, userID, agentID string) (bool, error)
	IsCollaboratorViaTeam(ctx context.Context, teamIDs []string, agentID string) (bool, error)
}

// CanAccessAgent implements the PRD agent-visibility hierarchy.
//
// Visibility precedence:
//
//	global   → every user
//	super_admin → metadata only (redaction applied elsewhere)
//	org isolation → agent.OrgID must match claims.OrgID
//	org_admin → full access within own org
//	public   → any member of the same org
//	team     → members of agent.TeamID + collaborator grants
//	private  → owner + collaborator grants
//
// checker may be nil for backward compatibility; collaborator checks are
// skipped when nil.
func CanAccessAgent(c Claims, agent AgentInfo, checker CollaboratorChecker) bool {
	if agent.Visibility == "global" {
		return true
	}
	if c.IsSuperAdmin {
		return true
	}
	if agent.OrgID != c.OrgID {
		return false
	}
	if c.OrgRole == "admin" {
		return true
	}
	switch agent.Visibility {
	case "public":
		return true
	case "team":
		if teamContains(c.TeamIDs, agent.TeamID) {
			return true
		}
		return checkCollaborator(checker, c, agent)
	case "private":
		if agent.OwnerID == c.UserID {
			return true
		}
		return checkCollaborator(checker, c, agent)
	}
	return false
}

// CanEditAgent returns true when the caller may modify the agent definition.
// Owners, org-admins (same org), and super-admins (for global agents) qualify.
// Collaborators with edit/admin permission are checked via checker (if non-nil).
func CanEditAgent(c Claims, agent AgentInfo, checker CollaboratorChecker) bool {
	if c.IsSuperAdmin {
		return true
	}
	if agent.OrgID != "" && agent.OrgID == c.OrgID && c.OrgRole == "admin" {
		return true
	}
	if agent.OwnerID == c.UserID {
		return true
	}
	return checkCollaborator(checker, c, agent)
}

// FilterAgentList returns only agents the caller is allowed to see.
func FilterAgentList(c Claims, agents []AgentInfo, checker CollaboratorChecker) []AgentInfo {
	out := make([]AgentInfo, 0, len(agents))
	for _, a := range agents {
		if CanAccessAgent(c, a, checker) {
			out = append(out, a)
		}
	}
	return out
}

// checkCollaborator returns true if the checker is non-nil and the user (or
// any of the user's teams) is listed as a collaborator on the agent.
func checkCollaborator(checker CollaboratorChecker, c Claims, agent AgentInfo) bool {
	if checker == nil {
		return false
	}
	ctx := context.Background()
	if ok, err := checker.IsCollaborator(ctx, c.UserID, agent.ID); err == nil && ok {
		return true
	}
	if len(c.TeamIDs) > 0 {
		if ok, err := checker.IsCollaboratorViaTeam(ctx, c.TeamIDs, agent.ID); err == nil && ok {
			return true
		}
	}
	return false
}

// teamContains checks whether teamID exists in the caller's team list.
func teamContains(teamIDs []string, teamID string) bool {
	for _, t := range teamIDs {
		if t == teamID {
			return true
		}
	}
	return false
}
