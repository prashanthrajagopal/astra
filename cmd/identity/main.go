package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"astra/internal/identity"
	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/health"
	"astra/pkg/httpx"
	"astra/pkg/logger"
)

const (
	errInvalidBody   = "invalid body: "
	errInternalError = "internal error"
	errInvalidUserID = "invalid user id"
	timeFormat       = "2006-01-02T15:04:05Z"
)

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

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
	Valid        bool     `json:"valid"`
	Subject      string   `json:"subject"`
	Scopes       []string `json:"scopes"`
	UserID       string   `json:"user_id,omitempty"`
	Email        string   `json:"email,omitempty"`
	IsSuperAdmin bool     `json:"is_super_admin,omitempty"`
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResp struct {
	Token     string    `json:"token"`
	ExpiresAt int64     `json:"expires_at"`
	User      loginUser `json:"user"`
}

type loginUser struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	IsSuperAdmin bool   `json:"is_super_admin"`
}

type createUserReq struct {
	Email        string `json:"email"`
	Name         string `json:"name"`
	Password     string `json:"password"`
	IsSuperAdmin bool   `json:"is_super_admin"`
}

type userJSON struct {
	ID           string  `json:"id"`
	Email        string  `json:"email"`
	Name         string  `json:"name"`
	Status       string  `json:"status"`
	IsSuperAdmin bool    `json:"is_super_admin"`
	LastLoginAt  *string `json:"last_login_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type updateUserReq struct {
	Name         *string `json:"name,omitempty"`
	Email        *string `json:"email,omitempty"`
	Status       *string `json:"status,omitempty"`
	IsSuperAdmin *bool   `json:"is_super_admin,omitempty"`
}

type resetPasswordReq struct {
	NewPassword string `json:"new_password"`
}

type listUsersResp struct {
	Users   []userJSON `json:"users"`
	Total   int        `json:"total"`
	Page    int        `json:"page"`
	PerPage int        `json:"per_page"`
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logger.New(cfg.LogLevel))

	database, err := db.Connect(cfg.PostgresDSN())
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	store := identity.NewStore(database)
	isrv := &identityServer{store: store, jwtSecret: cfg.JWTSecret}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /ready", health.ReadyHandler(database, nil))
	mux.HandleFunc("POST /tokens", handleIssueServiceToken(cfg.JWTSecret))
	mux.HandleFunc("POST /validate", isrv.handleValidateToken)
	mux.HandleFunc("POST /users/login", isrv.handleLogin)
	mux.HandleFunc("POST /users", isrv.handleCreateUser)
	mux.HandleFunc("GET /users", isrv.handleListUsers)
	mux.HandleFunc("GET /users/{id}", isrv.handleGetUser)
	mux.HandleFunc("PATCH /users/{id}", isrv.handleUpdateUser)
	mux.HandleFunc("POST /users/{id}/reset-password", isrv.handleResetPassword)

	addr := fmt.Sprintf(":%d", cfg.IdentityPort)
	slog.Info("identity service started", "addr", addr)
	srv := &http.Server{Addr: addr, Handler: mux}
	if err := httpx.ListenAndServe(srv, cfg); err != nil {
		slog.Error("identity server failed", "err", err)
		os.Exit(1)
	}
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleIssueServiceToken(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req tokenReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, errInvalidBody+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Subject == "" {
			jsonError(w, "subject required", http.StatusBadRequest)
			return
		}
		ttl := 3600
		if req.TTLSeconds > 0 {
			ttl = req.TTLSeconds
		}
		signed, expiresAt, err := identity.IssueServiceToken(secret, req.Subject, req.Scopes, ttl)
		if err != nil {
			slog.Error("sign token failed", "err", err)
			jsonError(w, "failed to sign token", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, tokenResp{Token: signed, ExpiresAt: expiresAt})
	}
}

func authenticateUser(ctx context.Context, store *identity.Store, email, password string) (*identity.User, *loginError) {
	user, err := store.Authenticate(ctx, email, password)
	if err != nil {
		slog.Error("authenticate failed", "err", err)
		return nil, &loginError{msg: errInternalError, status: http.StatusInternalServerError}
	}
	if user == nil {
		return nil, &loginError{msg: "invalid credentials", status: http.StatusUnauthorized}
	}
	if user.Status != "active" {
		return nil, &loginError{msg: "account is " + user.Status, status: http.StatusForbidden}
	}
	return user, nil
}

type loginError struct {
	msg    string
	status int
}

func (e *loginError) Error() string { return e.msg }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func toUserJSON(u *identity.User) userJSON {
	j := userJSON{
		ID:           u.ID.String(),
		Email:        u.Email,
		Name:         u.Name,
		Status:       u.Status,
		IsSuperAdmin: u.IsSuperAdmin,
		CreatedAt:    u.CreatedAt.Format(timeFormat),
		UpdatedAt:    u.UpdatedAt.Format(timeFormat),
	}
	if u.LastLoginAt != nil {
		s := u.LastLoginAt.Format(timeFormat)
		j.LastLoginAt = &s
	}
	return j
}

func queryInt(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return fallback
	}
	return v
}
