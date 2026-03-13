package rbac

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const ClaimsContextKey contextKey = "rbac_claims"

// ClaimsFromContext extracts RBAC claims from the request context.
func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(ClaimsContextKey).(Claims)
	return c, ok
}

// InjectClaimsFromHeaders reads multi-tenant headers set by the auth middleware
// and injects Claims into the request context. Headers:
// X-User-Id, X-Org-Id, X-Org-Role, X-Team-Ids (comma-separated),
// X-Is-Super-Admin, X-Email
func InjectClaimsFromHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := Claims{
			UserID:       r.Header.Get("X-User-Id"),
			Email:        r.Header.Get("X-Email"),
			OrgID:        r.Header.Get("X-Org-Id"),
			OrgRole:      r.Header.Get("X-Org-Role"),
			IsSuperAdmin: r.Header.Get("X-Is-Super-Admin") == "true",
		}
		if teamIDs := r.Header.Get("X-Team-Ids"); teamIDs != "" {
			c.TeamIDs = strings.Split(teamIDs, ",")
		}
		ctx := context.WithValue(r.Context(), ClaimsContextKey, c)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireSuperAdmin rejects requests that are not from a super admin.
func RequireSuperAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := ClaimsFromContext(r.Context())
		if !ok || !c.IsSuperAdmin {
			http.Error(w, `{"error":"super admin required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireOrgAdmin rejects requests that are not from an org admin (or super admin).
func RequireOrgAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := ClaimsFromContext(r.Context())
		if !ok || (!c.IsSuperAdmin && c.OrgRole != "admin") {
			http.Error(w, `{"error":"org admin required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireOrgMember rejects requests that have no org context.
func RequireOrgMember(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := ClaimsFromContext(r.Context())
		if !ok || (c.OrgID == "" && !c.IsSuperAdmin) {
			http.Error(w, `{"error":"org membership required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SetOrgHeaders sets X-Org-Id and other headers on an outgoing request
// for downstream service calls (e.g., to goal-service).
func SetOrgHeaders(r *http.Request, c Claims) {
	r.Header.Set("X-User-Id", c.UserID)
	r.Header.Set("X-Org-Id", c.OrgID)
	r.Header.Set("X-Org-Role", c.OrgRole)
	r.Header.Set("X-Email", c.Email)
	if c.IsSuperAdmin {
		r.Header.Set("X-Is-Super-Admin", "true")
	}
	if len(c.TeamIDs) > 0 {
		r.Header.Set("X-Team-Ids", strings.Join(c.TeamIDs, ","))
	}
}
