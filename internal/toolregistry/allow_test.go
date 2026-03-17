package toolregistry

import "testing"

func TestToolAllowed(t *testing.T) {
	if !ToolAllowed("shell_exec", nil) {
		t.Fatal("nil = all")
	}
	if !ToolAllowed("shell_exec", []byte("[]")) {
		t.Fatal("empty array = all")
	}
	raw := []byte(`["shell_exec@1","file_read@1"]`)
	if !ToolAllowed("shell_exec", raw) {
		t.Fatal("shell_exec should match")
	}
	if ToolAllowed("browser:screenshot", raw) {
		t.Fatal("browser should not match")
	}
	if !ToolAllowed("anything", []byte(`["*"]`)) {
		t.Fatal("wildcard")
	}
}
