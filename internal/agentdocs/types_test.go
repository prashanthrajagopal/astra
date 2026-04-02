package agentdocs

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestDocTypeConstants verifies each DocType constant has the expected string value
// and all values are distinct.
func TestDocTypeConstants(t *testing.T) {
	tests := []struct {
		name  string
		dt    DocType
		want  string
	}{
		{"DocTypeRule", DocTypeRule, "rule"},
		{"DocTypeSkill", DocTypeSkill, "skill"},
		{"DocTypeContextDoc", DocTypeContextDoc, "context_doc"},
		{"DocTypeReference", DocTypeReference, "reference"},
	}
	seen := make(map[DocType]bool)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.dt) != tc.want {
				t.Errorf("want %q, got %q", tc.want, string(tc.dt))
			}
			if seen[tc.dt] {
				t.Errorf("duplicate DocType value: %q", tc.dt)
			}
			seen[tc.dt] = true
		})
	}
}

// TestCacheKeyPrefixConstants verifies each cache key prefix is non-empty and distinct.
func TestCacheKeyPrefixConstants(t *testing.T) {
	prefixes := []struct {
		name  string
		value string
	}{
		{"profileKeyPrefix", profileKeyPrefix},
		{"docsKeyPrefix", docsKeyPrefix},
		{"chatCapableKeyPrefix", chatCapableKeyPrefix},
		{"agentPromptKeyPrefix", agentPromptKeyPrefix},
	}
	seen := make(map[string]bool)
	for _, p := range prefixes {
		t.Run(p.name, func(t *testing.T) {
			if p.value == "" {
				t.Errorf("%s must not be empty", p.name)
			}
			if seen[p.value] {
				t.Errorf("duplicate cache key prefix value: %q", p.value)
			}
			seen[p.value] = true
		})
	}
}

// TestDocument_JSONMarshal verifies Document round-trips through JSON correctly,
// including optional pointer fields.
func TestDocument_JSONMarshal(t *testing.T) {
	id := uuid.New()
	agentID := uuid.New()
	goalID := uuid.New()
	content := "test content"
	uri := "https://example.com/doc"
	now := time.Now().UTC().Truncate(time.Second)

	doc := Document{
		ID:        id,
		AgentID:   agentID,
		GoalID:    &goalID,
		DocType:   DocTypeRule,
		Name:      "my-rule",
		Content:   &content,
		URI:       &uri,
		Metadata:  json.RawMessage(`{"key":"value"}`),
		Priority:  10,
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got Document
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.ID != id {
		t.Errorf("ID: want %v, got %v", id, got.ID)
	}
	if got.AgentID != agentID {
		t.Errorf("AgentID: want %v, got %v", agentID, got.AgentID)
	}
	if got.GoalID == nil || *got.GoalID != goalID {
		t.Errorf("GoalID: want %v, got %v", goalID, got.GoalID)
	}
	if got.DocType != DocTypeRule {
		t.Errorf("DocType: want %q, got %q", DocTypeRule, got.DocType)
	}
	if got.Name != "my-rule" {
		t.Errorf("Name: want %q, got %q", "my-rule", got.Name)
	}
	if got.Content == nil || *got.Content != content {
		t.Errorf("Content: want %q, got %v", content, got.Content)
	}
	if got.URI == nil || *got.URI != uri {
		t.Errorf("URI: want %q, got %v", uri, got.URI)
	}
	if got.Priority != 10 {
		t.Errorf("Priority: want 10, got %d", got.Priority)
	}
	if string(got.Metadata) != `{"key":"value"}` {
		t.Errorf("Metadata: want %q, got %q", `{"key":"value"}`, string(got.Metadata))
	}
}

// TestDocument_JSONMarshal_NilOptionals verifies that nil optional fields are omitted.
func TestDocument_JSONMarshal_NilOptionals(t *testing.T) {
	doc := Document{
		ID:      uuid.New(),
		AgentID: uuid.New(),
		DocType: DocTypeSkill,
		Name:    "no-content",
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	// goal_id, content, uri should be omitted
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal into map: %v", err)
	}
	for _, field := range []string{"goal_id", "content", "uri"} {
		if v, ok := raw[field]; ok {
			t.Errorf("field %q should be omitted, got %s", field, string(v))
		}
	}
}

// TestAgentProfile_JSONMarshal verifies AgentProfile round-trips through JSON.
func TestAgentProfile_JSONMarshal(t *testing.T) {
	id := uuid.New()
	profile := AgentProfile{
		ID:                        id,
		Name:                      "test-agent",
		ActorType:                 "worker",
		SystemPrompt:              "you are helpful",
		Config:                    json.RawMessage(`{"temperature":0.7}`),
		ChatCapable:               true,
		IngestSourceType:          "slack",
		IngestSourceConfig:        json.RawMessage(`{"channel":"#general"}`),
		SlackNotificationsEnabled: true,
	}

	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got AgentProfile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.ID != id {
		t.Errorf("ID: want %v, got %v", id, got.ID)
	}
	if got.Name != "test-agent" {
		t.Errorf("Name: want %q, got %q", "test-agent", got.Name)
	}
	if got.ActorType != "worker" {
		t.Errorf("ActorType: want %q, got %q", "worker", got.ActorType)
	}
	if got.SystemPrompt != "you are helpful" {
		t.Errorf("SystemPrompt: want %q, got %q", "you are helpful", got.SystemPrompt)
	}
	if !got.ChatCapable {
		t.Error("ChatCapable: want true, got false")
	}
	if got.IngestSourceType != "slack" {
		t.Errorf("IngestSourceType: want %q, got %q", "slack", got.IngestSourceType)
	}
	if !got.SlackNotificationsEnabled {
		t.Error("SlackNotificationsEnabled: want true, got false")
	}
}

// TestAgentProfile_ZeroValue verifies a zero-value AgentProfile can be marshaled without error.
func TestAgentProfile_ZeroValue(t *testing.T) {
	var p AgentProfile
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal zero AgentProfile: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON for zero AgentProfile")
	}
}

// TestIngestBinding_JSONMarshal verifies IngestBinding round-trips through JSON.
func TestIngestBinding_JSONMarshal(t *testing.T) {
	agentID := uuid.New()
	binding := IngestBinding{
		AgentID:            agentID,
		IngestSourceType:   "webhook",
		IngestSourceConfig: json.RawMessage(`{"url":"https://example.com/hook"}`),
	}

	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got IngestBinding
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.AgentID != agentID {
		t.Errorf("AgentID: want %v, got %v", agentID, got.AgentID)
	}
	if got.IngestSourceType != "webhook" {
		t.Errorf("IngestSourceType: want %q, got %q", "webhook", got.IngestSourceType)
	}
	if string(got.IngestSourceConfig) != `{"url":"https://example.com/hook"}` {
		t.Errorf("IngestSourceConfig: unexpected value %s", string(got.IngestSourceConfig))
	}
}

// TestListOptions_Fields verifies ListOptions fields can be set without compile errors.
func TestListOptions_Fields(t *testing.T) {
	dt := DocTypeContextDoc
	goalID := uuid.New()

	opts := ListOptions{
		DocType:    &dt,
		GoalID:     &goalID,
		GlobalOnly: true,
	}

	if opts.DocType == nil || *opts.DocType != DocTypeContextDoc {
		t.Errorf("DocType: want %q, got %v", DocTypeContextDoc, opts.DocType)
	}
	if opts.GoalID == nil || *opts.GoalID != goalID {
		t.Errorf("GoalID: want %v, got %v", goalID, opts.GoalID)
	}
	if !opts.GlobalOnly {
		t.Error("GlobalOnly: want true")
	}
}

// TestListOptions_ZeroValue verifies zero-value ListOptions has expected defaults.
func TestListOptions_ZeroValue(t *testing.T) {
	var opts ListOptions
	if opts.DocType != nil {
		t.Errorf("DocType: want nil, got %v", opts.DocType)
	}
	if opts.GoalID != nil {
		t.Errorf("GoalID: want nil, got %v", opts.GoalID)
	}
	if opts.GlobalOnly {
		t.Error("GlobalOnly: want false")
	}
}

// TestJoinStrings covers all branches of the joinStrings helper.
func TestJoinStrings(t *testing.T) {
	tests := []struct {
		name string
		ss   []string
		sep  string
		want string
	}{
		{"empty", []string{}, ", ", ""},
		{"single", []string{"a"}, ", ", "a"},
		{"two", []string{"a", "b"}, ", ", "a, b"},
		{"three", []string{"x", "y", "z"}, " + ", "x + y + z"},
		{"empty-sep", []string{"foo", "bar"}, "", "foobar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := joinStrings(tc.ss, tc.sep)
			if got != tc.want {
				t.Errorf("joinStrings(%v, %q) = %q, want %q", tc.ss, tc.sep, got, tc.want)
			}
		})
	}
}

// TestNullJSON covers all branches of the nullJSON helper.
func TestNullJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   json.RawMessage
		wantNil bool
	}{
		{"nil input", nil, true},
		{"empty slice", []byte{}, true},
		{"valid json", json.RawMessage(`{}`), false},
		{"non-empty", json.RawMessage(`{"a":1}`), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := nullJSON(tc.input)
			if tc.wantNil && result != nil {
				t.Errorf("want nil, got %v", result)
			}
			if !tc.wantNil && result == nil {
				t.Error("want non-nil, got nil")
			}
		})
	}
}

// TestMergeRevisionIntoContext verifies mergeRevisionIntoContext applies the
// system prompt from a revision payload when present.
func TestMergeRevisionIntoContext(t *testing.T) {
	tests := []struct {
		name         string
		initial      string
		payload      []byte
		wantPrompt   string
	}{
		{
			name:       "applies non-empty system_prompt",
			initial:    "old prompt",
			payload:    []byte(`{"system_prompt":"new prompt"}`),
			wantPrompt: "new prompt",
		},
		{
			name:       "empty system_prompt in payload leaves original",
			initial:    "old prompt",
			payload:    []byte(`{"system_prompt":""}`),
			wantPrompt: "old prompt",
		},
		{
			name:       "nil payload is no-op",
			initial:    "old prompt",
			payload:    nil,
			wantPrompt: "old prompt",
		},
		{
			name:       "invalid JSON is no-op",
			initial:    "old prompt",
			payload:    []byte(`not json`),
			wantPrompt: "old prompt",
		},
		{
			name:       "empty payload slice is no-op",
			initial:    "original",
			payload:    []byte{},
			wantPrompt: "original",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ac := &AgentContext{SystemPrompt: tc.initial}
			mergeRevisionIntoContext(ac, tc.payload)
			if ac.SystemPrompt != tc.wantPrompt {
				t.Errorf("SystemPrompt: want %q, got %q", tc.wantPrompt, ac.SystemPrompt)
			}
		})
	}
}

// TestMergeRevisionIntoContext_NilContext verifies nil AgentContext is safe.
func TestMergeRevisionIntoContext_NilContext(t *testing.T) {
	// Must not panic.
	mergeRevisionIntoContext(nil, []byte(`{"system_prompt":"x"}`))
}

// TestConfigRevision_JSONMarshal verifies ConfigRevision round-trips through JSON.
func TestConfigRevision_JSONMarshal(t *testing.T) {
	id := uuid.New()
	agentID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	rev := ConfigRevision{
		ID:        id,
		AgentID:   agentID,
		Revision:  3,
		Payload:   json.RawMessage(`{"system_prompt":"v3"}`),
		CreatedAt: now,
		CreatedBy: "user-123",
	}

	data, err := json.Marshal(rev)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got ConfigRevision
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.ID != id {
		t.Errorf("ID: want %v, got %v", id, got.ID)
	}
	if got.AgentID != agentID {
		t.Errorf("AgentID: want %v, got %v", agentID, got.AgentID)
	}
	if got.Revision != 3 {
		t.Errorf("Revision: want 3, got %d", got.Revision)
	}
	if got.CreatedBy != "user-123" {
		t.Errorf("CreatedBy: want %q, got %q", "user-123", got.CreatedBy)
	}
}
