package rbac

import (
	"testing"
)

func TestRoleConstants(t *testing.T) {
	tests := []struct {
		name string
		role Role
		want string
	}{
		{"super admin", RoleSuperAdmin, "super_admin"},
		{"org admin", RoleOrgAdmin, "org_admin"},
		{"org member", RoleOrgMember, "org_member"},
		{"team admin", RoleTeamAdmin, "team_admin"},
		{"team member", RoleTeamMember, "team_member"},
		{"agent admin", RoleAgentAdmin, "agent_admin"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.role) != tc.want {
				t.Errorf("got %q, want %q", tc.role, tc.want)
			}
		})
	}
}

func TestClaimsStruct(t *testing.T) {
	c := Claims{
		UserID:       "user-1",
		Email:        "user@example.com",
		IsSuperAdmin: true,
		Scopes:       []string{"read", "write"},
	}
	if c.UserID != "user-1" {
		t.Errorf("UserID: got %q", c.UserID)
	}
	if c.Email != "user@example.com" {
		t.Errorf("Email: got %q", c.Email)
	}
	if !c.IsSuperAdmin {
		t.Error("IsSuperAdmin should be true")
	}
	if len(c.Scopes) != 2 {
		t.Errorf("Scopes len: got %d", len(c.Scopes))
	}
}

func TestDecisionStruct(t *testing.T) {
	d := Decision{
		Allowed:          true,
		ApprovalRequired: false,
		Reason:           "user is authorized",
	}
	if !d.Allowed {
		t.Error("Allowed should be true")
	}
	if d.ApprovalRequired {
		t.Error("ApprovalRequired should be false")
	}
	if d.Reason != "user is authorized" {
		t.Errorf("Reason: got %q", d.Reason)
	}
}

func TestIsSuperAdmin(t *testing.T) {
	tests := []struct {
		name   string
		claims Claims
		want   bool
	}{
		{"super admin", Claims{IsSuperAdmin: true}, true},
		{"not super admin", Claims{IsSuperAdmin: false}, false},
		{"empty claims", Claims{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsSuperAdmin(tc.claims); got != tc.want {
				t.Errorf("IsSuperAdmin() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRedactForSuperAdmin_NilInput(t *testing.T) {
	result := RedactForSuperAdmin(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestRedactForSuperAdmin_EmptyInput(t *testing.T) {
	result := RedactForSuperAdmin(map[string]interface{}{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestRedactForSuperAdmin_RedactsSensitiveKeys(t *testing.T) {
	sensitiveKeys := []string{
		"system_prompt", "config", "payload", "result",
		"goal_text", "content", "tool_calls", "tool_results",
	}
	for _, key := range sensitiveKeys {
		t.Run(key, func(t *testing.T) {
			data := map[string]interface{}{key: "secret value"}
			result := RedactForSuperAdmin(data)
			if result[key] != "[REDACTED]" {
				t.Errorf("key %q: got %v, want [REDACTED]", key, result[key])
			}
		})
	}
}

func TestRedactForSuperAdmin_PreservesNonSensitiveKeys(t *testing.T) {
	data := map[string]interface{}{
		"id":         "agent-123",
		"name":       "my-agent",
		"created_at": "2024-01-01",
		"status":     "active",
	}
	result := RedactForSuperAdmin(data)
	for k, v := range data {
		if result[k] != v {
			t.Errorf("key %q: got %v, want %v", k, result[k], v)
		}
	}
}

func TestRedactForSuperAdmin_DoesNotMutateOriginal(t *testing.T) {
	original := map[string]interface{}{
		"system_prompt": "secret",
		"name":          "agent",
	}
	_ = RedactForSuperAdmin(original)
	if original["system_prompt"] != "secret" {
		t.Error("original map was mutated")
	}
}

func TestRedactForSuperAdmin_HandlesNestedMaps(t *testing.T) {
	data := map[string]interface{}{
		"metadata": map[string]interface{}{
			"system_prompt": "inner secret",
			"id":            "abc",
		},
	}
	result := RedactForSuperAdmin(data)
	nested, ok := result["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("metadata should remain a map")
	}
	if nested["system_prompt"] != "[REDACTED]" {
		t.Errorf("nested system_prompt: got %v, want [REDACTED]", nested["system_prompt"])
	}
	if nested["id"] != "abc" {
		t.Errorf("nested id: got %v, want abc", nested["id"])
	}
}

func TestRedactForSuperAdmin_HandlesSliceValues(t *testing.T) {
	data := map[string]interface{}{
		"tool_calls": []interface{}{"call1", "call2"},
	}
	result := RedactForSuperAdmin(data)
	sliceVal, ok := result["tool_calls"].([]interface{})
	if !ok {
		t.Fatal("tool_calls should be a slice")
	}
	for i, v := range sliceVal {
		if v != "[REDACTED]" {
			t.Errorf("slice[%d]: got %v, want [REDACTED]", i, v)
		}
	}
}

func TestRedactForSuperAdmin_NonSensitiveSlicePreserved(t *testing.T) {
	data := map[string]interface{}{
		"tags": []interface{}{"tag1", "tag2"},
	}
	result := RedactForSuperAdmin(data)
	sliceVal, ok := result["tags"].([]interface{})
	if !ok {
		t.Fatal("tags should be a slice")
	}
	if sliceVal[0] != "tag1" || sliceVal[1] != "tag2" {
		t.Errorf("tags not preserved: %v", sliceVal)
	}
}

func TestRedactForSuperAdmin_NonSensitiveWithNestedSensitive(t *testing.T) {
	data := map[string]interface{}{
		"outer": map[string]interface{}{
			"config":   "nested-secret",
			"visible":  "yes",
		},
	}
	result := RedactForSuperAdmin(data)
	outer, ok := result["outer"].(map[string]interface{})
	if !ok {
		t.Fatal("outer should remain a map")
	}
	if outer["config"] != "[REDACTED]" {
		t.Errorf("outer.config: got %v, want [REDACTED]", outer["config"])
	}
	if outer["visible"] != "yes" {
		t.Errorf("outer.visible: got %v, want yes", outer["visible"])
	}
}

func TestRedactForSuperAdmin_IntValueRedacted(t *testing.T) {
	data := map[string]interface{}{
		"result": 42,
	}
	result := RedactForSuperAdmin(data)
	if result["result"] != "[REDACTED]" {
		t.Errorf("int result: got %v, want [REDACTED]", result["result"])
	}
}

func TestRedactForSuperAdmin_NilValueRedacted(t *testing.T) {
	data := map[string]interface{}{
		"payload": nil,
	}
	result := RedactForSuperAdmin(data)
	if result["payload"] != "[REDACTED]" {
		t.Errorf("nil payload: got %v, want [REDACTED]", result["payload"])
	}
}

func TestRedactForSuperAdmin_SensitiveKeyWithMapValue(t *testing.T) {
	// Covers the map[string]interface{} branch inside redactValue.
	data := map[string]interface{}{
		"config": map[string]interface{}{
			"db_password": "secret",
			"host":        "localhost",
		},
	}
	result := RedactForSuperAdmin(data)
	inner, ok := result["config"].(map[string]interface{})
	if !ok {
		t.Fatal("config should be a map")
	}
	if inner["db_password"] != "[REDACTED]" {
		t.Errorf("db_password: got %v, want [REDACTED]", inner["db_password"])
	}
	if inner["host"] != "[REDACTED]" {
		t.Errorf("host: got %v, want [REDACTED]", inner["host"])
	}
}

func TestRedactForSuperAdmin_SensitiveKeyWithSliceOfMaps(t *testing.T) {
	// Covers the []interface{} branch inside redactValue with nested maps.
	data := map[string]interface{}{
		"tool_calls": []interface{}{
			map[string]interface{}{"name": "exec", "args": "rm -rf"},
			"plain string",
		},
	}
	result := RedactForSuperAdmin(data)
	slice, ok := result["tool_calls"].([]interface{})
	if !ok {
		t.Fatal("tool_calls should be a slice")
	}
	// Each element should be redacted
	inner, ok := slice[0].(map[string]interface{})
	if !ok {
		t.Fatal("first element should be a map")
	}
	if inner["name"] != "[REDACTED]" {
		t.Errorf("nested map name: got %v, want [REDACTED]", inner["name"])
	}
	if slice[1] != "[REDACTED]" {
		t.Errorf("string element: got %v, want [REDACTED]", slice[1])
	}
}
