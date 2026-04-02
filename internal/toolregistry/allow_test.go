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

func TestToolAllowed_Table(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		allowed []byte
		want    bool
	}{
		// nil / empty → always allowed
		{"nil_allowed", "tool_a", nil, true},
		{"empty_json", "tool_a", []byte("[]"), true},
		{"empty_bytes", "tool_a", []byte(""), true},

		// malformed JSON → fail-open (allow)
		{"malformed_json", "tool_a", []byte("not-json"), true},

		// global wildcard
		{"global_wildcard", "anything", []byte(`["*"]`), true},
		{"global_wildcard_at", "anything", []byte(`["*@*"]`), true},

		// exact name match (no version suffix)
		{"exact_match", "file_read", []byte(`["file_read"]`), true},
		{"exact_no_match", "file_write", []byte(`["file_read"]`), false},

		// versioned entry "tool@1" – tool name prefix matches
		{"versioned_entry_match", "shell_exec", []byte(`["shell_exec@1"]`), true},
		{"versioned_entry_no_match", "file_read", []byte(`["shell_exec@1"]`), false},

		// version wildcard "tool@*"
		{"version_wildcard_match", "shell_exec", []byte(`["shell_exec@*"]`), true},
		{"version_wildcard_no_match", "file_read", []byte(`["shell_exec@*"]`), false},

		// specific version exact "tool@1"
		{"specific_version_match", "browser:screenshot", []byte(`["browser:screenshot@1"]`), true},
		{"specific_version_other_tool", "file_read", []byte(`["browser:screenshot@1"]`), false},

		// unlisted tool
		{"unlisted_tool", "unknown_tool", []byte(`["file_read","shell_exec@1"]`), false},

		// empty string entry in list (treated as allow-all)
		{"empty_entry_in_list", "anything", []byte(`[""]`), true},

		// multiple entries, second matches
		{"second_entry_matches", "list_files", []byte(`["file_read","list_files"]`), true},

		// entry with whitespace trimming — TrimSpace makes "  shell_exec  " == "shell_exec"
		{"whitespace_entry", "shell_exec", []byte(`["  shell_exec  "]`), true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := ToolAllowed(tc.tool, tc.allowed)
			if got != tc.want {
				t.Errorf("ToolAllowed(%q, %s) = %v, want %v", tc.tool, tc.allowed, got, tc.want)
			}
		})
	}
}

func TestToolAllowed_WhitespaceTrimming(t *testing.T) {
	// Entry "  shell_exec  " after TrimSpace becomes "shell_exec" which matches toolName
	allowed := []byte(`["  shell_exec  "]`)
	if !ToolAllowed("shell_exec", allowed) {
		t.Error("whitespace-trimmed exact match should return true")
	}
	// A different tool should not match
	if ToolAllowed("file_read", allowed) {
		t.Error("file_read should not match shell_exec entry")
	}
}

func TestToolAllowed_EmptyEntryAllowsAll(t *testing.T) {
	// An empty string entry (after trimming) in the list means allow-all
	allowed := []byte(`["file_read","","shell_exec"]`)
	if !ToolAllowed("any_random_tool", allowed) {
		t.Error("empty entry in list should act as allow-all")
	}
}

func TestToolAllowed_VersionWildcardDoesNotMatchOtherTool(t *testing.T) {
	allowed := []byte(`["file_read@*"]`)
	if ToolAllowed("file_write", allowed) {
		t.Error("file_read@* should not match file_write")
	}
	if !ToolAllowed("file_read", allowed) {
		t.Error("file_read@* should match file_read")
	}
}

func TestToolAllowed_MultipleVersionedEntries(t *testing.T) {
	allowed := []byte(`["tool_a@1","tool_b@2","tool_c@*"]`)
	tests := []struct {
		tool string
		want bool
	}{
		{"tool_a", true},
		{"tool_b", true},
		{"tool_c", true},
		{"tool_d", false},
	}
	for _, tc := range tests {
		if got := ToolAllowed(tc.tool, allowed); got != tc.want {
			t.Errorf("ToolAllowed(%q) = %v, want %v", tc.tool, got, tc.want)
		}
	}
}
