package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"astra/pkg/config"
	"astra/pkg/logger"
)

type tokenReq struct {
	Subject    string   `json:"subject"`
	Scopes     []string `json:"scopes"`
	TTLSeconds int      `json:"ttl_seconds"`
}

type tokenResp struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

type validateReq struct {
	Token string `json:"token"`
}

type validateResp struct {
	Valid   bool     `json:"valid"`
	Subject string   `json:"subject"`
	Scopes  []string `json:"scopes"`
}

type claims struct {
	jwt.RegisteredClaims
	Subject string   `json:"sub"`
	Scopes  []string `json:"scopes"`
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logger.New(cfg.LogLevel))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("POST /tokens", makeTokenHandler(cfg.JWTSecret))
	mux.HandleFunc("POST /validate", makeValidateHandler(cfg.JWTSecret))

	addr := fmt.Sprintf(":%d", cfg.IdentityPort)
	slog.Info("identity service started", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("identity server failed", "err", err)
		os.Exit(1)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func makeTokenHandler(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req tokenReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Subject == "" {
			http.Error(w, "subject required", http.StatusBadRequest)
			return
		}
		ttl := 3600
		if req.TTLSeconds > 0 {
			ttl = req.TTLSeconds
		}
		now := time.Now()
		exp := now.Add(time.Duration(ttl) * time.Second)
		c := claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   req.Subject,
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(exp),
				Issuer:    "astra-identity",
			},
			Subject: req.Subject,
			Scopes:  req.Scopes,
		}
		if c.Scopes == nil {
			c.Scopes = []string{}
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
		signed, err := token.SignedString([]byte(secret))
		if err != nil {
			slog.Error("sign token failed", "err", err)
			http.Error(w, "failed to sign token", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResp{Token: signed, ExpiresAt: exp.Unix()})
	}
}

func makeValidateHandler(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req validateReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
			return
		}
		tok := strings.TrimSpace(req.Token)
		if tok == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(validateResp{Valid: false})
			return
		}
		t, err := jwt.ParseWithClaims(tok, &claims{}, func(_ *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		if err != nil || !t.Valid {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(validateResp{Valid: false})
			return
		}
		c, ok := t.Claims.(*claims)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(validateResp{Valid: false})
			return
		}
		sub := c.Subject
		if sub == "" {
			sub = c.RegisteredClaims.Subject
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(validateResp{Valid: true, Subject: sub, Scopes: c.Scopes})
	}
}
