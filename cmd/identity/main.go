package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"astra/internal/identity"
	"astra/pkg/config"
	"astra/pkg/db"
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
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
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
	OrgID        string   `json:"org_id,omitempty"`
	OrgRole      string   `json:"org_role,omitempty"`
	TeamIDs      []string `json:"team_ids,omitempty"`
	IsSuperAdmin bool     `json:"is_super_admin,omitempty"`
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgID    string `json:"org_id,omitempty"`
}

type loginResp struct {
	Token     string    `json:"token"`
	ExpiresAt int64     `json:"expires_at"`
	User      loginUser `json:"user"`
	Org       *loginOrg `json:"org,omitempty"`
}

type loginUser struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	IsSuperAdmin bool   `json:"is_super_admin"`
}

type loginOrg struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
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

type userDetailJSON struct {
	userJSON
	OrgMemberships  []orgMembershipJSON  `json:"org_memberships"`
	TeamMemberships []teamMembershipJSON `json:"team_memberships"`
}

type orgMembershipJSON struct {
	OrgID   string `json:"org_id"`
	OrgName string `json:"org_name"`
	OrgSlug string `json:"org_slug"`
	Role    string `json:"role"`
}

type teamMembershipJSON struct {
	TeamID   string `json:"team_id"`
	TeamName string `json:"team_name"`
	OrgID    string `json:"org_id"`
	Role     string `json:"role"`
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("POST /tokens", handleIssueServiceToken(cfg.JWTSecret))
	mux.HandleFunc("POST /validate", handleValidate(cfg.JWTSecret))
	mux.HandleFunc("POST /users/login", handleLogin(cfg.JWTSecret, store))
	mux.HandleFunc("POST /users", handleCreateUser(store))
	mux.HandleFunc("GET /users", handleListUsers(store))
	mux.HandleFunc("GET /users/{id}", handleGetUser(store))
	mux.HandleFunc("PATCH /users/{id}", handleUpdateUser(store))
	mux.HandleFunc("POST /users/{id}/reset-password", handleResetPassword(store))

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
	w.Write([]byte("ok"))
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

func handleValidate(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req validateReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, errInvalidBody+err.Error(), http.StatusBadRequest)
			return
		}
		tok := strings.TrimSpace(req.Token)
		if tok == "" {
			writeJSON(w, http.StatusUnauthorized, validateResp{Valid: false})
			return
		}
		result, err := identity.ValidateToken(secret, tok)
		if err != nil || !result.Valid {
			writeJSON(w, http.StatusUnauthorized, validateResp{Valid: false})
			return
		}
		writeJSON(w, http.StatusOK, validateResp{
			Valid:        true,
			Subject:      result.Subject,
			Scopes:       result.Scopes,
			UserID:       result.UserID,
			Email:        result.Email,
			OrgID:        result.OrgID,
			OrgRole:      result.OrgRole,
			TeamIDs:      result.TeamIDs,
			IsSuperAdmin: result.IsSuperAdmin,
		})
	}
}

func handleLogin(secret string, store *identity.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, errInvalidBody+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Email == "" || req.Password == "" {
			jsonError(w, "email and password required", http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		user, authErr := authenticateUser(ctx, store, req.Email, req.Password)
		if authErr != nil {
			jsonError(w, authErr.msg, authErr.status)
			return
		}

		claims := identity.Claims{
			UserID:       user.ID.String(),
			Email:        user.Email,
			IsSuperAdmin: user.IsSuperAdmin,
			Scopes:       []string{"user"},
		}
		claims.Subject = user.ID.String()

		org, err := resolveOrgContext(ctx, store, user, req.OrgID, &claims)
		if err != nil {
			writeLoginOrgError(w, err)
			return
		}

		signed, expiresAt, err := identity.IssueToken(secret, claims, 86400)
		if err != nil {
			slog.Error("issue token failed", "err", err)
			jsonError(w, errInternalError, http.StatusInternalServerError)
			return
		}

		if loginErr := store.UpdateLastLogin(ctx, user.ID); loginErr != nil {
			slog.Warn("update last_login_at failed", "user_id", user.ID, "err", loginErr)
		}

		writeJSON(w, http.StatusOK, loginResp{
			Token:     signed,
			ExpiresAt: expiresAt,
			User: loginUser{
				ID:           user.ID.String(),
				Email:        user.Email,
				Name:         user.Name,
				IsSuperAdmin: user.IsSuperAdmin,
			},
			Org: org,
		})
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

func resolveOrgContext(ctx context.Context, store *identity.Store, user *identity.User, rawOrgID string, claims *identity.Claims) (*loginOrg, error) {
	if rawOrgID == "" {
		if !user.IsSuperAdmin {
			return nil, &loginError{msg: "org_id required for non-super-admin users", status: http.StatusBadRequest}
		}
		return nil, nil
	}

	orgUUID, err := uuid.Parse(rawOrgID)
	if err != nil {
		return nil, &loginError{msg: "invalid org_id", status: http.StatusBadRequest}
	}

	memberships, err := store.GetOrgMemberships(ctx, user.ID)
	if err != nil {
		slog.Error("get org memberships failed", "err", err)
		return nil, &loginError{msg: errInternalError, status: http.StatusInternalServerError}
	}

	found := findOrgMembership(memberships, orgUUID)
	if found == nil && !user.IsSuperAdmin {
		return nil, &loginError{msg: "not a member of this organization", status: http.StatusForbidden}
	}

	claims.OrgID = orgUUID.String()
	var org *loginOrg
	if found != nil {
		claims.OrgRole = found.Role
		org = &loginOrg{ID: orgUUID.String(), Name: found.OrgName, Role: found.Role}
	} else {
		claims.OrgRole = "admin"
		org = &loginOrg{ID: orgUUID.String(), Name: "", Role: "admin"}
	}

	teams, err := store.GetTeamMemberships(ctx, user.ID, &orgUUID)
	if err != nil {
		slog.Error("get team memberships failed", "err", err)
		return nil, &loginError{msg: errInternalError, status: http.StatusInternalServerError}
	}
	for _, t := range teams {
		claims.TeamIDs = append(claims.TeamIDs, t.TeamID.String())
	}
	return org, nil
}

func findOrgMembership(memberships []identity.OrgMembership, orgID uuid.UUID) *identity.OrgMembership {
	for i := range memberships {
		if memberships[i].OrgID == orgID {
			return &memberships[i]
		}
	}
	return nil
}

func writeLoginOrgError(w http.ResponseWriter, err error) {
	le, ok := err.(*loginError)
	if !ok {
		jsonError(w, errInternalError, http.StatusInternalServerError)
		return
	}
	jsonError(w, le.msg, le.status)
}

func handleCreateUser(store *identity.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createUserReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, errInvalidBody+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Email == "" || req.Name == "" || req.Password == "" {
			jsonError(w, "email, name, and password required", http.StatusBadRequest)
			return
		}

		user, err := store.CreateUser(r.Context(), req.Email, req.Name, req.Password, req.IsSuperAdmin)
		if err != nil {
			slog.Error("create user failed", "err", err)
			jsonError(w, "failed to create user", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, toUserJSON(user))
	}
}

func handleListUsers(store *identity.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page := queryInt(q.Get("page"), 1)
		perPage := queryInt(q.Get("per_page"), 20)
		if perPage > 100 {
			perPage = 100
		}
		offset := (page - 1) * perPage

		var orgID *uuid.UUID
		if raw := q.Get("org_id"); raw != "" {
			parsed, err := uuid.Parse(raw)
			if err != nil {
				jsonError(w, "invalid org_id", http.StatusBadRequest)
				return
			}
			orgID = &parsed
		}

		users, total, err := store.ListUsers(r.Context(), orgID, q.Get("status"), q.Get("q"), perPage, offset)
		if err != nil {
			slog.Error("list users failed", "err", err)
			jsonError(w, errInternalError, http.StatusInternalServerError)
			return
		}

		items := make([]userJSON, 0, len(users))
		for _, u := range users {
			items = append(items, toUserJSON(u))
		}
		writeJSON(w, http.StatusOK, listUsersResp{Users: items, Total: total, Page: page, PerPage: perPage})
	}
}

func handleGetUser(store *identity.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			jsonError(w, errInvalidUserID, http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		user, err := store.GetUserByID(ctx, id)
		if err != nil {
			slog.Error("get user failed", "err", err)
			jsonError(w, errInternalError, http.StatusInternalServerError)
			return
		}
		if user == nil {
			jsonError(w, "user not found", http.StatusNotFound)
			return
		}

		orgs, err := store.GetOrgMemberships(ctx, id)
		if err != nil {
			slog.Error("get org memberships failed", "err", err)
			jsonError(w, errInternalError, http.StatusInternalServerError)
			return
		}

		teams, err := store.GetTeamMemberships(ctx, id, nil)
		if err != nil {
			slog.Error("get team memberships failed", "err", err)
			jsonError(w, errInternalError, http.StatusInternalServerError)
			return
		}

		detail := userDetailJSON{userJSON: toUserJSON(user)}
		detail.OrgMemberships = make([]orgMembershipJSON, 0, len(orgs))
		for _, o := range orgs {
			detail.OrgMemberships = append(detail.OrgMemberships, orgMembershipJSON{
				OrgID: o.OrgID.String(), OrgName: o.OrgName, OrgSlug: o.OrgSlug, Role: o.Role,
			})
		}
		detail.TeamMemberships = make([]teamMembershipJSON, 0, len(teams))
		for _, t := range teams {
			detail.TeamMemberships = append(detail.TeamMemberships, teamMembershipJSON{
				TeamID: t.TeamID.String(), TeamName: t.TeamName, OrgID: t.OrgID.String(), Role: t.Role,
			})
		}
		writeJSON(w, http.StatusOK, detail)
	}
}

func handleUpdateUser(store *identity.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			jsonError(w, errInvalidUserID, http.StatusBadRequest)
			return
		}
		var req updateUserReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, errInvalidBody+err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.UpdateUser(r.Context(), id, req.Name, req.Email, req.Status, req.IsSuperAdmin); err != nil {
			slog.Error("update user failed", "err", err)
			jsonError(w, "failed to update user", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleResetPassword(store *identity.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			jsonError(w, errInvalidUserID, http.StatusBadRequest)
			return
		}
		var req resetPasswordReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, errInvalidBody+err.Error(), http.StatusBadRequest)
			return
		}
		if req.NewPassword == "" {
			jsonError(w, "new_password required", http.StatusBadRequest)
			return
		}
		if err := store.UpdatePassword(r.Context(), id, req.NewPassword); err != nil {
			slog.Error("reset password failed", "err", err)
			jsonError(w, "failed to reset password", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
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
