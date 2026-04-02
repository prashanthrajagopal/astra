package slack

import (
	"testing"
)

// post.go contains PostMessage and RefreshToken which require HTTP and DB dependencies.
// We test the pure structural/constant aspects and URL constants here.

func TestChatPostMessageURL(t *testing.T) {
	if chatPostMessageURL != "https://slack.com/api/chat.postMessage" {
		t.Errorf("chatPostMessageURL: got %q", chatPostMessageURL)
	}
}

func TestOAuthAccessURL(t *testing.T) {
	if oauthAccessURL != "https://slack.com/api/oauth.v2.access" {
		t.Errorf("oauthAccessURL: got %q", oauthAccessURL)
	}
}
