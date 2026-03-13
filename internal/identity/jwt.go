package identity

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	jwt.RegisteredClaims
	UserID       string   `json:"user_id"`
	Email        string   `json:"email"`
	OrgID        string   `json:"org_id,omitempty"`
	OrgRole      string   `json:"org_role,omitempty"`
	TeamIDs      []string `json:"team_ids,omitempty"`
	IsSuperAdmin bool     `json:"is_super_admin"`
	Scopes       []string `json:"scopes"`
}

type ValidateResult struct {
	Valid        bool     `json:"valid"`
	UserID       string   `json:"user_id"`
	Email        string   `json:"email"`
	OrgID        string   `json:"org_id,omitempty"`
	OrgRole      string   `json:"org_role,omitempty"`
	TeamIDs      []string `json:"team_ids,omitempty"`
	IsSuperAdmin bool     `json:"is_super_admin"`
	Scopes       []string `json:"scopes"`
	Subject      string   `json:"subject"`
}

func IssueToken(secret string, c Claims, ttlSeconds int) (string, int64, error) {
	now := time.Now()
	exp := now.Add(time.Duration(ttlSeconds) * time.Second)

	c.IssuedAt = jwt.NewNumericDate(now)
	c.ExpiresAt = jwt.NewNumericDate(exp)
	c.Issuer = "astra-identity"

	if c.Scopes == nil {
		c.Scopes = []string{}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", 0, fmt.Errorf("identity.IssueToken: %w", err)
	}
	return signed, exp.Unix(), nil
}

func ValidateToken(secret, tokenStr string) (*ValidateResult, error) {
	c := &Claims{}
	tok, err := jwt.ParseWithClaims(tokenStr, c, func(_ *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil || !tok.Valid {
		return &ValidateResult{Valid: false}, nil
	}

	sub := c.Subject
	if sub == "" {
		sub = c.RegisteredClaims.Subject
	}

	return &ValidateResult{
		Valid:        true,
		UserID:       c.UserID,
		Email:        c.Email,
		OrgID:        c.OrgID,
		OrgRole:      c.OrgRole,
		TeamIDs:      c.TeamIDs,
		IsSuperAdmin: c.IsSuperAdmin,
		Scopes:       c.Scopes,
		Subject:      sub,
	}, nil
}

type serviceClaims struct {
	jwt.RegisteredClaims
	Subject string   `json:"sub"`
	Scopes  []string `json:"scopes"`
}

func IssueServiceToken(secret, subject string, scopes []string, ttlSeconds int) (string, int64, error) {
	now := time.Now()
	exp := now.Add(time.Duration(ttlSeconds) * time.Second)

	if scopes == nil {
		scopes = []string{}
	}

	c := serviceClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			Issuer:    "astra-identity",
		},
		Subject: subject,
		Scopes:  scopes,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", 0, fmt.Errorf("identity.IssueServiceToken: %w", err)
	}
	return signed, exp.Unix(), nil
}
