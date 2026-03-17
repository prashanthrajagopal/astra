package rbac

// Role represents a tenant-scoped authorization role in Astra.
type Role string

const (
	RoleSuperAdmin Role = "super_admin"
	RoleOrgAdmin   Role = "org_admin"
	RoleOrgMember  Role = "org_member"
	RoleTeamAdmin  Role = "team_admin"
	RoleTeamMember Role = "team_member"
	RoleAgentAdmin Role = "agent_admin"
)

// Claims carries JWT claims through the authorization layer (single-platform: no org/team).
type Claims struct {
	UserID       string
	Email        string
	IsSuperAdmin bool
	Scopes       []string
}

// Decision is the result of an authorization check.
type Decision struct {
	Allowed          bool
	ApprovalRequired bool
	Reason           string
}

// IsSuperAdmin returns true when the claims carry super-admin privilege.
func IsSuperAdmin(c Claims) bool {
	return c.IsSuperAdmin
}

// redactedKeys is the set of keys stripped from data visible to super-admins.
var redactedKeys = map[string]struct{}{
	"system_prompt": {},
	"config":        {},
	"payload":       {},
	"result":        {},
	"goal_text":     {},
	"content":       {},
	"tool_calls":    {},
	"tool_results":  {},
}

// RedactForSuperAdmin returns a deep copy of data with sensitive execution
// details replaced by "[REDACTED]". Nested maps and slices of maps are handled
// recursively. The original map is never mutated.
func RedactForSuperAdmin(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}
	out := make(map[string]interface{}, len(data))
	for k, v := range data {
		if _, sensitive := redactedKeys[k]; sensitive {
			out[k] = redactValue(v)
			continue
		}
		out[k] = deepRedact(v)
	}
	return out
}

// redactValue replaces strings with "[REDACTED]" and recursively redacts maps
// and slices so that nested content is also stripped.
func redactValue(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		return "[REDACTED]"
	case map[string]interface{}:
		r := make(map[string]interface{}, len(val))
		for k, inner := range val {
			r[k] = redactValue(inner)
		}
		return r
	case []interface{}:
		r := make([]interface{}, len(val))
		for i, inner := range val {
			r[i] = redactValue(inner)
		}
		return r
	default:
		return "[REDACTED]"
	}
}

// deepRedact walks non-sensitive values so that sensitive keys inside nested
// structures are still caught.
func deepRedact(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return RedactForSuperAdmin(val)
	case []interface{}:
		r := make([]interface{}, len(val))
		for i, inner := range val {
			r[i] = deepRedact(inner)
		}
		return r
	default:
		return v
	}
}
