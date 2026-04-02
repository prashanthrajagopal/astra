package identity

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-key-for-unit-tests"

func TestIssueTokenCreatesValidJWT(t *testing.T) {
	c := Claims{
		UserID:       "user-1",
		Email:        "user@example.com",
		IsSuperAdmin: false,
		Scopes:       []string{"read", "write"},
	}
	tokenStr, exp, err := IssueToken(testSecret, c, 3600)
	if err != nil {
		t.Fatalf("IssueToken error: %v", err)
	}
	if tokenStr == "" {
		t.Error("token string is empty")
	}
	if exp <= time.Now().Unix() {
		t.Errorf("expiry %d should be in the future", exp)
	}
}

func TestValidateTokenSucceedsWithValidToken(t *testing.T) {
	c := Claims{
		UserID:       "user-42",
		Email:        "test@example.com",
		IsSuperAdmin: true,
		Scopes:       []string{"admin"},
	}
	tokenStr, _, err := IssueToken(testSecret, c, 3600)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	result, err := ValidateToken(testSecret, tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken error: %v", err)
	}
	if !result.Valid {
		t.Error("expected Valid=true")
	}
	if result.UserID != "user-42" {
		t.Errorf("UserID: got %q, want %q", result.UserID, "user-42")
	}
	if result.Email != "test@example.com" {
		t.Errorf("Email: got %q, want %q", result.Email, "test@example.com")
	}
	if !result.IsSuperAdmin {
		t.Error("expected IsSuperAdmin=true")
	}
	if len(result.Scopes) != 1 || result.Scopes[0] != "admin" {
		t.Errorf("Scopes: got %v, want [admin]", result.Scopes)
	}
}

func TestValidateTokenFailsWithExpiredToken(t *testing.T) {
	c := Claims{
		UserID: "user-1",
		Email:  "x@x.com",
	}
	// issue token that expired 1 second ago
	tokenStr, _, err := IssueToken(testSecret, c, -1)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	result, err := ValidateToken(testSecret, tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken returned unexpected error: %v", err)
	}
	if result.Valid {
		t.Error("expected Valid=false for expired token")
	}
}

func TestValidateTokenFailsWithInvalidSignature(t *testing.T) {
	c := Claims{UserID: "user-1"}
	tokenStr, _, err := IssueToken(testSecret, c, 3600)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	result, err := ValidateToken("wrong-secret", tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken returned unexpected error: %v", err)
	}
	if result.Valid {
		t.Error("expected Valid=false for wrong secret")
	}
}

func TestValidateTokenFailsWithGarbageToken(t *testing.T) {
	result, err := ValidateToken(testSecret, "not.a.jwt")
	if err != nil {
		t.Fatalf("ValidateToken returned unexpected error: %v", err)
	}
	if result.Valid {
		t.Error("expected Valid=false for garbage token")
	}
}

func TestValidateTokenEmptyToken(t *testing.T) {
	result, err := ValidateToken(testSecret, "")
	if err != nil {
		t.Fatalf("ValidateToken returned unexpected error: %v", err)
	}
	if result.Valid {
		t.Error("expected Valid=false for empty token")
	}
}

func TestIssueTokenScopesNilBecomesEmpty(t *testing.T) {
	c := Claims{
		UserID: "user-1",
		Scopes: nil,
	}
	tokenStr, _, err := IssueToken(testSecret, c, 3600)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	result, err := ValidateToken(testSecret, tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if result.Scopes == nil {
		t.Error("expected non-nil empty scopes slice")
	}
}

func TestIssueTokenSetsIssuer(t *testing.T) {
	c := Claims{UserID: "u1"}
	tokenStr, _, err := IssueToken(testSecret, c, 3600)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	claims := &Claims{}
	_, err = jwt.ParseWithClaims(tokenStr, claims, func(_ *jwt.Token) (any, error) {
		return []byte(testSecret), nil
	})
	if err != nil {
		t.Fatalf("ParseWithClaims: %v", err)
	}
	if claims.Issuer != "astra-identity" {
		t.Errorf("Issuer: got %q, want %q", claims.Issuer, "astra-identity")
	}
}

func TestIssueTokenDifferentScopes(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
	}{
		{"no scopes", []string{}},
		{"single scope", []string{"read"}},
		{"multiple scopes", []string{"read", "write", "admin"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := Claims{UserID: "u1", Scopes: tc.scopes}
			tokenStr, _, err := IssueToken(testSecret, c, 3600)
			if err != nil {
				t.Fatalf("IssueToken: %v", err)
			}
			result, err := ValidateToken(testSecret, tokenStr)
			if err != nil {
				t.Fatalf("ValidateToken: %v", err)
			}
			if !result.Valid {
				t.Error("expected Valid=true")
			}
			if len(result.Scopes) != len(tc.scopes) {
				t.Errorf("scopes length: got %d, want %d", len(result.Scopes), len(tc.scopes))
			}
		})
	}
}

func TestIssueServiceToken(t *testing.T) {
	tokenStr, exp, err := IssueServiceToken(testSecret, "worker-service", []string{"tasks:read"}, 3600)
	if err != nil {
		t.Fatalf("IssueServiceToken: %v", err)
	}
	if tokenStr == "" {
		t.Error("token string is empty")
	}
	if exp <= time.Now().Unix() {
		t.Errorf("expiry %d should be in the future", exp)
	}
}

func TestIssueServiceTokenNilScopes(t *testing.T) {
	tokenStr, _, err := IssueServiceToken(testSecret, "svc", nil, 3600)
	if err != nil {
		t.Fatalf("IssueServiceToken: %v", err)
	}
	if tokenStr == "" {
		t.Error("token string is empty")
	}
}
