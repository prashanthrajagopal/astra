package toolregistry

import (
	"encoding/json"
	"strings"
)

// ToolAllowed returns true if allowedTools is nil/empty (all tools) or name matches an entry.
// Entries: "tool@1", "tool@*", "*", wildcard.
func ToolAllowed(toolName string, allowedToolsJSON []byte) bool {
	if len(allowedToolsJSON) == 0 {
		return true
	}
	var arr []string
	if json.Unmarshal(allowedToolsJSON, &arr) != nil || len(arr) == 0 {
		return true
	}
	for _, e := range arr {
		e = strings.TrimSpace(e)
		if e == "" || e == "*" || e == "*@*" {
			return true
		}
		if strings.HasSuffix(e, "@*") {
			if strings.TrimSuffix(e, "@*") == toolName {
				return true
			}
			continue
		}
		if e == toolName {
			return true
		}
		if strings.HasPrefix(e, toolName+"@") {
			return true
		}
	}
	return false
}
