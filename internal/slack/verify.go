package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// VerifyRequest verifies the Slack request signature (X-Slack-Signature and X-Slack-Request-Timestamp).
// Body must be the raw request body. Returns nil if valid.
func VerifyRequest(signingSecret, body string, signature, timestampHeader string) error {
	if signingSecret == "" {
		return fmt.Errorf("slack: signing secret not set")
	}
	ts, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil {
		return fmt.Errorf("slack: invalid timestamp: %w", err)
	}
	if time.Now().Unix()-ts > 300 {
		return fmt.Errorf("slack: request timestamp too old")
	}
	base := "v0:" + timestampHeader + ":" + body
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(base))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(strings.TrimSpace(signature))) {
		return fmt.Errorf("slack: signature mismatch")
	}
	return nil
}
