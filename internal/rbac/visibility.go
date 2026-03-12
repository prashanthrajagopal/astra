package rbac

// AgentInfo carries the fields required for visibility and ownership checks.
type AgentInfo struct {
	ID         string
	OrgID      string // empty for global agents
	OwnerID    string
	TeamID     string
	Visibility string // global, public, team, private
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
//	team     → members of agent.TeamID (collaborator check deferred)
//	private  → owner only (collaborator check deferred)
func CanAccessAgent(c Claims, agent AgentInfo) bool {
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
		return teamContains(c.TeamIDs, agent.TeamID)
	case "private":
		return agent.OwnerID == c.UserID
	}
	return false
}

// CanEditAgent returns true when the caller may modify the agent definition.
// Owners, org-admins (same org), and super-admins (for global agents) qualify.
func CanEditAgent(c Claims, agent AgentInfo) bool {
	if c.IsSuperAdmin {
		return true
	}
	if agent.OrgID != "" && agent.OrgID == c.OrgID && c.OrgRole == "admin" {
		return true
	}
	return agent.OwnerID == c.UserID
}

// FilterAgentList returns only agents the caller is allowed to see.
func FilterAgentList(c Claims, agents []AgentInfo) []AgentInfo {
	out := make([]AgentInfo, 0, len(agents))
	for _, a := range agents {
		if CanAccessAgent(c, a) {
			out = append(out, a)
		}
	}
	return out
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
