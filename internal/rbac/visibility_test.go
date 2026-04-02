package rbac

import (
	"testing"
)

func TestCanAccessAgent_AuthenticatedUser(t *testing.T) {
	tests := []struct {
		name   string
		claims Claims
		want   bool
	}{
		{"with user id", Claims{UserID: "user-1"}, true},
		{"super admin no user id", Claims{IsSuperAdmin: true}, true},
		{"super admin with user id", Claims{UserID: "user-1", IsSuperAdmin: true}, true},
		{"empty claims no access", Claims{}, false},
		{"empty user id and not super admin", Claims{UserID: "", IsSuperAdmin: false}, false},
	}
	agent := AgentInfo{ID: "agent-1"}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CanAccessAgent(tc.claims, agent, nil)
			if got != tc.want {
				t.Errorf("CanAccessAgent() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCanEditAgent_AuthenticatedUser(t *testing.T) {
	tests := []struct {
		name   string
		claims Claims
		want   bool
	}{
		{"with user id", Claims{UserID: "user-1"}, true},
		{"super admin only", Claims{IsSuperAdmin: true}, true},
		{"empty claims", Claims{}, false},
	}
	agent := AgentInfo{ID: "agent-1"}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CanEditAgent(tc.claims, agent, nil)
			if got != tc.want {
				t.Errorf("CanEditAgent() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFilterAgentList_ReturnsAll(t *testing.T) {
	agents := []AgentInfo{
		{ID: "a1"},
		{ID: "a2"},
		{ID: "a3"},
	}
	tests := []struct {
		name   string
		claims Claims
	}{
		{"super admin", Claims{UserID: "u1", IsSuperAdmin: true}},
		{"regular user", Claims{UserID: "u2"}},
		{"empty claims", Claims{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FilterAgentList(tc.claims, agents, nil)
			if len(result) != len(agents) {
				t.Errorf("FilterAgentList returned %d agents, want %d", len(result), len(agents))
			}
		})
	}
}

func TestFilterAgentList_EmptyList(t *testing.T) {
	result := FilterAgentList(Claims{UserID: "u1"}, []AgentInfo{}, nil)
	if len(result) != 0 {
		t.Errorf("expected empty list, got %d agents", len(result))
	}
}

func TestFilterAgentList_NilList(t *testing.T) {
	result := FilterAgentList(Claims{UserID: "u1"}, nil, nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestAgentInfo_IDField(t *testing.T) {
	a := AgentInfo{ID: "test-agent-123"}
	if a.ID != "test-agent-123" {
		t.Errorf("AgentInfo.ID: got %q", a.ID)
	}
}
