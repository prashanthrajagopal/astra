package dashboard

import (
	"testing"
)

func TestSnapshot_ZeroValue(t *testing.T) {
	var s Snapshot
	if s.GeneratedAt != "" {
		t.Errorf("GeneratedAt = %q, want empty", s.GeneratedAt)
	}
	if s.Services != nil {
		t.Errorf("Services should be nil for zero value")
	}
	if s.AgentCount != 0 {
		t.Errorf("AgentCount = %d, want 0", s.AgentCount)
	}
}

func TestSnapshot_FieldAssignment(t *testing.T) {
	s := Snapshot{
		GeneratedAt: "2026-04-02T00:00:00Z",
		AgentCount:  5,
		Services: []ServiceStatus{
			{Name: "api-gateway", Port: 8080, Type: "http", Healthy: true},
		},
		Workers:   []map[string]any{{"id": "w1"}},
		Approvals: []map[string]any{},
		Jobs:      map[string]any{"total": 10},
		Cost:      map[string]any{"rows": []any{}},
		Logs:      map[string][]string{"task-service": {"log line 1"}},
		PIDs:      map[string]int{"task-service": 12345},
	}

	if s.GeneratedAt != "2026-04-02T00:00:00Z" {
		t.Errorf("GeneratedAt = %q", s.GeneratedAt)
	}
	if s.AgentCount != 5 {
		t.Errorf("AgentCount = %d, want 5", s.AgentCount)
	}
	if len(s.Services) != 1 {
		t.Fatalf("Services len = %d, want 1", len(s.Services))
	}
	if s.Services[0].Name != "api-gateway" {
		t.Errorf("Services[0].Name = %q, want api-gateway", s.Services[0].Name)
	}
	if len(s.Workers) != 1 {
		t.Errorf("Workers len = %d, want 1", len(s.Workers))
	}
}

func TestServiceStatus_Fields(t *testing.T) {
	tests := []struct {
		name      string
		status    ServiceStatus
		wantName  string
		wantPort  int
		wantType  string
		wantOK    bool
	}{
		{
			name:     "healthy http",
			status:   ServiceStatus{Name: "api-gateway", Port: 8080, Type: "http", Healthy: true},
			wantName: "api-gateway", wantPort: 8080, wantType: "http", wantOK: true,
		},
		{
			name:     "unhealthy grpc",
			status:   ServiceStatus{Name: "task-service", Port: 9090, Type: "grpc", Healthy: false, Error: "connection refused"},
			wantName: "task-service", wantPort: 9090, wantType: "grpc", wantOK: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.status.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", tc.status.Name, tc.wantName)
			}
			if tc.status.Port != tc.wantPort {
				t.Errorf("Port = %d, want %d", tc.status.Port, tc.wantPort)
			}
			if tc.status.Type != tc.wantType {
				t.Errorf("Type = %q, want %q", tc.status.Type, tc.wantType)
			}
			if tc.status.Healthy != tc.wantOK {
				t.Errorf("Healthy = %v, want %v", tc.status.Healthy, tc.wantOK)
			}
		})
	}
}

func TestServiceNames_NotEmpty(t *testing.T) {
	names := serviceNames()
	if len(names) == 0 {
		t.Error("serviceNames() returned empty slice")
	}
	seen := make(map[string]bool)
	for _, n := range names {
		if n == "" {
			t.Error("serviceNames() contains empty string")
		}
		if seen[n] {
			t.Errorf("duplicate service name: %q", n)
		}
		seen[n] = true
	}
}

func TestNormalizeWorkers_EmptyInput(t *testing.T) {
	result := normalizeWorkers(nil)
	if len(result) != 0 {
		t.Errorf("normalizeWorkers(nil) len = %d, want 0", len(result))
	}
}

func TestNormalizeWorkers_MapsFields(t *testing.T) {
	input := []map[string]any{
		{"id": "w1", "hostname": "host1", "status": "active", "capabilities": []string{"gpu"}},
		{"ID": "w2", "Hostname": "host2", "Status": "offline"},
	}
	result := normalizeWorkers(input)
	if len(result) != 2 {
		t.Fatalf("normalizeWorkers len = %d, want 2", len(result))
	}
	if result[0]["id"] != "w1" {
		t.Errorf("result[0][id] = %v, want w1", result[0]["id"])
	}
	if result[0]["hostname"] != "host1" {
		t.Errorf("result[0][hostname] = %v, want host1", result[0]["hostname"])
	}
	if result[1]["id"] != "w2" {
		t.Errorf("result[1][id] = %v, want w2 (from ID key)", result[1]["id"])
	}
	if result[1]["hostname"] != "host2" {
		t.Errorf("result[1][hostname] = %v, want host2 (from Hostname key)", result[1]["hostname"])
	}
}

func TestNormalizeWorkers_MissingKeys(t *testing.T) {
	input := []map[string]any{
		{"unknown_key": "val"},
	}
	result := normalizeWorkers(input)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0]["id"] != nil {
		t.Errorf("id should be nil for missing key, got %v", result[0]["id"])
	}
}

func TestFirstAny_ReturnsFirstMatch(t *testing.T) {
	m := map[string]any{"hostname": "host1", "Hostname": "HOST1"}

	tests := []struct {
		name string
		keys []string
		want any
	}{
		{"first key matches", []string{"hostname", "Hostname"}, "host1"},
		{"second key matches", []string{"missing", "hostname"}, "host1"},
		{"no match", []string{"x", "y"}, nil},
		{"empty map keys", []string{}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := firstAny(m, tc.keys...)
			if got != tc.want {
				t.Errorf("firstAny(..., %v) = %v, want %v", tc.keys, got, tc.want)
			}
		})
	}
}
