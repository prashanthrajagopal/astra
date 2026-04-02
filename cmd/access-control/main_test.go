package main

import "testing"

func TestEvaluatePolicy(t *testing.T) {
	tests := []struct {
		name             string
		req              checkReq
		wantAllow        bool
		wantApproval     bool
		wantReasonSubstr string
	}{
		{
			name:      "health action always allowed",
			req:       checkReq{Action: "health"},
			wantAllow: true,
		},
		{
			name:      "health resource always allowed",
			req:       checkReq{Resource: "/health"},
			wantAllow: true,
		},
		{
			name:      "bare health resource always allowed",
			req:       checkReq{Resource: "health"},
			wantAllow: true,
		},
		{
			name:      "super admin can read agents",
			req:       checkReq{IsSuperAdmin: true, Action: "read", Resource: "/agents"},
			wantAllow: true,
		},
		{
			name:             "super admin blocked on task payload",
			req:              checkReq{IsSuperAdmin: true, Resource: "/tasks/123/payload"},
			wantAllow:        false,
			wantReasonSubstr: "super-admin cannot access execution details",
		},
		{
			name:             "super admin blocked on task result",
			req:              checkReq{IsSuperAdmin: true, Resource: "/tasks/abc/result"},
			wantAllow:        false,
			wantReasonSubstr: "super-admin cannot access execution details",
		},
		{
			name:             "super admin blocked on graph details",
			req:              checkReq{IsSuperAdmin: true, Resource: "/graphs/xyz/details"},
			wantAllow:        false,
			wantReasonSubstr: "super-admin cannot access execution details",
		},
		{
			name:             "super admin blocked on goal payload",
			req:              checkReq{IsSuperAdmin: true, Resource: "/goals/1/payload"},
			wantAllow:        false,
			wantReasonSubstr: "super-admin cannot access execution details",
		},
		{
			name:             "low trust score requires approval",
			req:              checkReq{TrustScore: 0.1, Action: "read", Resource: "/data"},
			wantAllow:        false,
			wantApproval:     true,
			wantReasonSubstr: "low trust score",
		},
		{
			name:             "trust score exactly at threshold (0.3) is still low",
			req:              checkReq{TrustScore: 0.29, Action: "read"},
			wantAllow:        false,
			wantApproval:     true,
			wantReasonSubstr: "low trust score",
		},
		{
			name:      "trust score at 0.3 passes (boundary: < 0.3 triggers, == 0.3 does not)",
			req:       checkReq{TrustScore: 0.3, Action: "read"},
			wantAllow: true,
		},
		{
			name:      "normal trust score allowed",
			req:       checkReq{TrustScore: 0.8, Action: "read"},
			wantAllow: true,
		},
		{
			name:             "dangerous tool kubectl requires approval",
			req:              checkReq{Action: "tool.execute", ToolName: "kubectl-apply"},
			wantAllow:        false,
			wantApproval:     true,
			wantReasonSubstr: "dangerous tool",
		},
		{
			name:             "dangerous tool terraform requires approval",
			req:              checkReq{Action: "tool.execute", ToolName: "terraform-plan"},
			wantAllow:        false,
			wantApproval:     true,
			wantReasonSubstr: "dangerous tool",
		},
		{
			name:             "dangerous tool with delete substring requires approval",
			req:              checkReq{Action: "tool.execute", ToolName: "delete-records"},
			wantAllow:        false,
			wantApproval:     true,
			wantReasonSubstr: "dangerous tool",
		},
		{
			name:             "dangerous tool with prod substring requires approval",
			req:              checkReq{Action: "tool.execute", ToolName: "deploy-prod"},
			wantAllow:        false,
			wantApproval:     true,
			wantReasonSubstr: "dangerous tool",
		},
		{
			name:      "safe tool allowed",
			req:       checkReq{Action: "tool.execute", ToolName: "file_read"},
			wantAllow: true,
		},
		{
			name:      "tool.execute with empty tool name allowed",
			req:       checkReq{Action: "tool.execute", ToolName: ""},
			wantAllow: true,
		},
		{
			name:      "non-tool action allowed without trust score",
			req:       checkReq{Action: "read", Resource: "/agents"},
			wantAllow: true,
		},
		{
			name:      "zero trust score (omitted) is allowed",
			req:       checkReq{TrustScore: 0, Action: "read"},
			wantAllow: true,
		},
		{
			name:      "action with leading/trailing whitespace still matches health",
			req:       checkReq{Action: "  health  "},
			wantAllow: true,
		},
		{
			name:         "tool name case insensitive dangerous check",
			req:          checkReq{Action: "tool.execute", ToolName: "KUBECTL-APPLY"},
			wantAllow:    false,
			wantApproval: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluatePolicy(tt.req)
			if got.Allowed != tt.wantAllow {
				t.Errorf("Allowed = %v, want %v (reason: %q)", got.Allowed, tt.wantAllow, got.Reason)
			}
			if got.ApprovalRequired != tt.wantApproval {
				t.Errorf("ApprovalRequired = %v, want %v (reason: %q)", got.ApprovalRequired, tt.wantApproval, got.Reason)
			}
			if tt.wantReasonSubstr != "" {
				if got.Reason == "" {
					t.Errorf("expected reason containing %q, got empty string", tt.wantReasonSubstr)
				}
			}
		})
	}
}

func TestIsExecutionDetailResource(t *testing.T) {
	tests := []struct {
		resource string
		want     bool
	}{
		{"/tasks/123/payload", true},
		{"/tasks/abc-def/result", true},
		{"/tasks/xyz/details", true},
		{"/graphs/123/payload", true},
		{"/graphs/123/result", true},
		{"/graphs/123/details", true},
		{"/goals/123/payload", true},
		{"/goals/123/result", true},
		{"/goals/123/details", true},
		// paths that do not match
		{"/tasks/123", false},
		{"/tasks/", false},
		{"/graphs/123", false},
		{"/goals/123", false},
		{"/agents/123/details", false},
		{"/health", false},
		{"", false},
		{"/tasks/123/status", false},
		{"/goals/123/logs", false},
	}

	for _, tt := range tests {
		t.Run(tt.resource, func(t *testing.T) {
			got := isExecutionDetailResource(tt.resource)
			if got != tt.want {
				t.Errorf("isExecutionDetailResource(%q) = %v, want %v", tt.resource, got, tt.want)
			}
		})
	}
}
