package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"astra/internal/identity"

	"github.com/google/uuid"
)

type identityServer struct {
	store     *identity.Store
	jwtSecret string
}

func (s *identityServer) handleLogin(w http.ResponseWriter, r *http.Request) {
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
	user, authErr := authenticateUser(ctx, s.store, req.Email, req.Password)
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

	signed, expiresAt, err := identity.IssueToken(s.jwtSecret, claims, 86400)
	if err != nil {
		slog.Error("issue token failed", "err", err)
		jsonError(w, errInternalError, http.StatusInternalServerError)
		return
	}

	if loginErr := s.store.UpdateLastLogin(ctx, user.ID); loginErr != nil {
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
	})
}

func (s *identityServer) handleValidateToken(w http.ResponseWriter, r *http.Request) {
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
	result, err := identity.ValidateToken(s.jwtSecret, tok)
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
		IsSuperAdmin: result.IsSuperAdmin,
	})
}

func (s *identityServer) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, errInvalidBody+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Name == "" || req.Password == "" {
		jsonError(w, "email, name, and password required", http.StatusBadRequest)
		return
	}

	user, err := s.store.CreateUser(r.Context(), req.Email, req.Name, req.Password, req.IsSuperAdmin)
	if err != nil {
		slog.Error("create user failed", "err", err)
		jsonError(w, "failed to create user", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, toUserJSON(user))
}

func (s *identityServer) handleListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := queryInt(q.Get("page"), 1)
	perPage := queryInt(q.Get("per_page"), 20)
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage

	users, total, err := s.store.ListUsers(r.Context(), q.Get("status"), q.Get("q"), perPage, offset)
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

func (s *identityServer) handleGetUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		jsonError(w, errInvalidUserID, http.StatusBadRequest)
		return
	}

	user, err := s.store.GetUserByID(r.Context(), id)
	if err != nil {
		slog.Error("get user failed", "err", err)
		jsonError(w, errInternalError, http.StatusInternalServerError)
		return
	}
	if user == nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, toUserJSON(user))
}

func (s *identityServer) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
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
	if err := s.store.UpdateUser(r.Context(), id, req.Name, req.Email, req.Status, req.IsSuperAdmin); err != nil {
		slog.Error("update user failed", "err", err)
		jsonError(w, "failed to update user", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *identityServer) handleResetPassword(w http.ResponseWriter, r *http.Request) {
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
	if err := s.store.UpdatePassword(r.Context(), id, req.NewPassword); err != nil {
		slog.Error("reset password failed", "err", err)
		jsonError(w, "failed to reset password", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
