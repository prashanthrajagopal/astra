package rbac

import (
	"context"
	"net/http"
)

type contextKey string

const ClaimsContextKey contextKey = "rbac_claims"

// ClaimsFromContext extracts RBAC claims from the request context.
func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(ClaimsContextKey).(Claims)
	return c, ok
}

// InjectClaimsFromHeaders reads headers set by the auth middleware and injects
// Claims into the request context. Headers: X-User-Id, X-Email, X-Is-Super-Admin.
func InjectClaimsFromHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := Claims{
			UserID:       r.Header.Get("X-User-Id"),
			Email:        r.Header.Get("X-Email"),
			IsSuperAdmin: r.Header.Get("X-Is-Super-Admin") == "true",
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

// RequireOrgAdmin rejects requests that are not from a super admin (single-platform: org admin removed).
func RequireOrgAdmin(next http.Handler) http.Handler {
	return RequireSuperAdmin(next)
}

// RequireOrgMember accepts any authenticated user (single-platform: no org).
func RequireOrgMember(next http.Handler) http.Handler {
	return next
}

// SetOrgHeaders sets user and super-admin headers on an outgoing request (no org/team).
func SetOrgHeaders(r *http.Request, c Claims) {
	r.Header.Set("X-User-Id", c.UserID)
	r.Header.Set("X-Email", c.Email)
	if c.IsSuperAdmin {
		r.Header.Set("X-Is-Super-Admin", "true")
	}
}
