package slack

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestWorkspaceStruct_JSONSerialization(t *testing.T) {
	agentID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	w := Workspace{
		ID:                    uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		SlackWorkspaceID:      "T12345",
		BotTokenRef:           "ref/bot-token",
		RefreshTokenRef:       "ref/refresh-token",
		NotificationChannelID: "C99999",
		DefaultAgentID:        &agentID,
		CreatedAt:             "2024-01-01T00:00:00Z",
		UpdatedAt:             "2024-06-01T00:00:00Z",
	}

	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got Workspace
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got.SlackWorkspaceID != w.SlackWorkspaceID {
		t.Errorf("SlackWorkspaceID: got %q, want %q", got.SlackWorkspaceID, w.SlackWorkspaceID)
	}
	if got.BotTokenRef != w.BotTokenRef {
		t.Errorf("BotTokenRef: got %q, want %q", got.BotTokenRef, w.BotTokenRef)
	}
	if got.NotificationChannelID != w.NotificationChannelID {
		t.Errorf("NotificationChannelID: got %q", got.NotificationChannelID)
	}
	if got.DefaultAgentID == nil || *got.DefaultAgentID != agentID {
		t.Errorf("DefaultAgentID: got %v, want %v", got.DefaultAgentID, agentID)
	}
}

func TestWorkspaceStruct_OmitEmptyBotTokenRef(t *testing.T) {
	w := Workspace{
		ID:               uuid.New(),
		SlackWorkspaceID: "T123",
	}
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	// bot_token_ref has omitempty tag — should not appear when empty
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}
	if _, ok := m["bot_token_ref"]; ok {
		t.Error("bot_token_ref should be omitted when empty")
	}
}

func TestWorkspaceStruct_NilDefaultAgentID(t *testing.T) {
	w := Workspace{
		ID:               uuid.New(),
		SlackWorkspaceID: "T456",
		DefaultAgentID:   nil,
	}
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var got Workspace
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got.DefaultAgentID != nil {
		t.Errorf("DefaultAgentID should be nil, got %v", got.DefaultAgentID)
	}
}

func TestConfigKeyConstants(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{"signing secret", ConfigKeySigningSecret, "signing_secret"},
		{"client id", ConfigKeyClientID, "client_id"},
		{"client secret", ConfigKeyClientSecret, "client_secret"},
		{"oauth redirect url", ConfigKeyOAuthRedirectURL, "oauth_redirect_url"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.key != tc.want {
				t.Errorf("got %q, want %q", tc.key, tc.want)
			}
		})
	}
}

func TestRootThreadTSConstant(t *testing.T) {
	if RootThreadTS != "" {
		t.Errorf("RootThreadTS should be empty string, got %q", RootThreadTS)
	}
}

func TestNullString_NonEmpty(t *testing.T) {
	got := nullString("hello")
	if got != "hello" {
		t.Errorf("nullString(\"hello\"): got %v, want hello", got)
	}
}

func TestNullString_Empty(t *testing.T) {
	got := nullString("")
	if got != nil {
		t.Errorf("nullString(\"\"): got %v, want nil", got)
	}
}

func TestNullUUID_NonNil(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	got := nullUUID(&id)
	if got != id.String() {
		t.Errorf("nullUUID(&id): got %v, want %v", got, id.String())
	}
}

func TestNullUUID_Nil(t *testing.T) {
	got := nullUUID(nil)
	if got != nil {
		t.Errorf("nullUUID(nil): got %v, want nil", got)
	}
}
