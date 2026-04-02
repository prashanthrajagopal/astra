package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func computeExpectedSig(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestValidateHMAC(t *testing.T) {
	secret := "my-secret-key"

	tests := []struct {
		name      string
		body      []byte
		signature string
		secret    string
		want      bool
	}{
		{
			name:      "valid signature",
			body:      []byte(`{"event":"push","ref":"refs/heads/main"}`),
			signature: computeExpectedSig([]byte(`{"event":"push","ref":"refs/heads/main"}`), secret),
			secret:    secret,
			want:      true,
		},
		{
			name:      "invalid signature wrong value",
			body:      []byte(`{"event":"push"}`),
			signature: "sha256=deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			secret:    secret,
			want:      false,
		},
		{
			name:      "empty signature string",
			body:      []byte(`{"event":"push"}`),
			signature: "",
			secret:    secret,
			want:      false,
		},
		{
			name:      "empty body valid signature",
			body:      []byte{},
			signature: computeExpectedSig([]byte{}, secret),
			secret:    secret,
			want:      true,
		},
		{
			name:      "empty body wrong signature",
			body:      []byte{},
			signature: "sha256=wronghash",
			secret:    secret,
			want:      false,
		},
		{
			name:      "signature missing sha256= prefix",
			body:      []byte("hello"),
			signature: hex.EncodeToString(func() []byte { mac := hmac.New(sha256.New, []byte(secret)); mac.Write([]byte("hello")); return mac.Sum(nil) }()),
			secret:    secret,
			want:      false,
		},
		{
			name:      "signature with correct prefix but wrong hash",
			body:      []byte("hello"),
			signature: "sha256=0000000000000000000000000000000000000000000000000000000000000000",
			secret:    secret,
			want:      false,
		},
		{
			name:      "valid signature with different secret",
			body:      []byte(`{"data":"value"}`),
			signature: computeExpectedSig([]byte(`{"data":"value"}`), "different-secret"),
			secret:    "different-secret",
			want:      true,
		},
		{
			name:      "body mismatch: valid sig for different body",
			body:      []byte(`{"data":"value"}`),
			signature: computeExpectedSig([]byte(`{"data":"other"}`), secret),
			secret:    secret,
			want:      false,
		},
		{
			name:      "large body valid signature",
			body:      make([]byte, 1024),
			signature: computeExpectedSig(make([]byte, 1024), secret),
			secret:    secret,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateHMAC(tt.body, tt.signature, tt.secret)
			if got != tt.want {
				t.Errorf("validateHMAC() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetEnvWebhook(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envVal   string
		setEnv   bool
		fallback string
		want     string
	}{
		{
			name:     "returns env value when set",
			key:      "WEBHOOK_TEST_KEY",
			envVal:   "custom-port",
			setEnv:   true,
			fallback: "8099",
			want:     "custom-port",
		},
		{
			name:     "returns fallback when not set",
			key:      "WEBHOOK_UNSET_KEY",
			setEnv:   false,
			fallback: "8099",
			want:     "8099",
		},
		{
			name:     "empty env value returns fallback",
			key:      "WEBHOOK_EMPTY_KEY",
			envVal:   "",
			setEnv:   true,
			fallback: "8099",
			want:     "8099",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.key, tt.envVal)
			}
			got := getEnv(tt.key, tt.fallback)
			if got != tt.want {
				t.Errorf("getEnv(%q, %q) = %q, want %q", tt.key, tt.fallback, got, tt.want)
			}
		})
	}
}
