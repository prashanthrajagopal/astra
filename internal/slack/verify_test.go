package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"testing"
	"time"
)

// computeSignature builds a valid Slack request signature for testing.
func computeSignature(secret, body string, ts int64) string {
	base := fmt.Sprintf("v0:%d:%s", ts, body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(base))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyRequest_ValidSignature(t *testing.T) {
	secret := "test-signing-secret"
	body := `token=value&payload=stuff`
	ts := time.Now().Unix()
	sig := computeSignature(secret, body, ts)

	err := VerifyRequest(secret, body, sig, strconv.FormatInt(ts, 10))
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestVerifyRequest_InvalidSignature(t *testing.T) {
	secret := "test-signing-secret"
	body := `some body`
	ts := time.Now().Unix()

	err := VerifyRequest(secret, body, "v0=invalidsignature", strconv.FormatInt(ts, 10))
	if err == nil {
		t.Error("expected error for invalid signature")
	}
}

func TestVerifyRequest_WrongSecret(t *testing.T) {
	body := `some body`
	ts := time.Now().Unix()
	sig := computeSignature("correct-secret", body, ts)

	err := VerifyRequest("wrong-secret", body, sig, strconv.FormatInt(ts, 10))
	if err == nil {
		t.Error("expected error when secret is wrong")
	}
}

func TestVerifyRequest_EmptySecret(t *testing.T) {
	ts := time.Now().Unix()
	sig := computeSignature("", "body", ts)
	err := VerifyRequest("", "body", sig, strconv.FormatInt(ts, 10))
	if err == nil {
		t.Error("expected error for empty signing secret")
	}
}

func TestVerifyRequest_EmptyBody(t *testing.T) {
	secret := "mysecret"
	ts := time.Now().Unix()
	sig := computeSignature(secret, "", ts)

	err := VerifyRequest(secret, "", sig, strconv.FormatInt(ts, 10))
	if err != nil {
		t.Errorf("empty body with valid sig: unexpected error: %v", err)
	}
}

func TestVerifyRequest_TimestampTooOld(t *testing.T) {
	secret := "test-signing-secret"
	body := `payload=data`
	oldTS := time.Now().Unix() - 301 // older than 5 minutes
	sig := computeSignature(secret, body, oldTS)

	err := VerifyRequest(secret, body, sig, strconv.FormatInt(oldTS, 10))
	if err == nil {
		t.Error("expected error for old timestamp")
	}
}

func TestVerifyRequest_InvalidTimestamp(t *testing.T) {
	err := VerifyRequest("secret", "body", "v0=abc", "not-a-number")
	if err == nil {
		t.Error("expected error for non-numeric timestamp")
	}
}

func TestVerifyRequest_EmptyTimestamp(t *testing.T) {
	err := VerifyRequest("secret", "body", "v0=abc", "")
	if err == nil {
		t.Error("expected error for empty timestamp")
	}
}

func TestVerifyRequest_SignatureWithLeadingSpace(t *testing.T) {
	// VerifyRequest calls strings.TrimSpace on the signature
	secret := "test-signing-secret"
	body := `data`
	ts := time.Now().Unix()
	sig := computeSignature(secret, body, ts)

	err := VerifyRequest(secret, body, " "+sig+" ", strconv.FormatInt(ts, 10))
	if err != nil {
		t.Errorf("trimmed signature should verify: %v", err)
	}
}

func TestVerifyRequest_WrongBody(t *testing.T) {
	secret := "signing-secret"
	ts := time.Now().Unix()
	sig := computeSignature(secret, "original body", ts)

	err := VerifyRequest(secret, "tampered body", sig, strconv.FormatInt(ts, 10))
	if err == nil {
		t.Error("expected error when body is tampered")
	}
}
