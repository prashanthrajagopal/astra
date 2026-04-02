package rbac

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInjectClaimsFromHeaders_ExtractsAllHeaders(t *testing.T) {
	var captured Claims
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := ClaimsFromContext(r.Context())
		if !ok {
			t.Error("claims not found in context")
		}
		captured = c
	})

	handler := InjectClaimsFromHeaders(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-User-Id", "user-42")
	req.Header.Set("X-Email", "alice@example.com")
	req.Header.Set("X-Is-Super-Admin", "true")

	handler.ServeHTTP(httptest.NewRecorder(), req)

	if captured.UserID != "user-42" {
		t.Errorf("UserID: got %q", captured.UserID)
	}
	if captured.Email != "alice@example.com" {
		t.Errorf("Email: got %q", captured.Email)
	}
	if !captured.IsSuperAdmin {
		t.Error("IsSuperAdmin should be true")
	}
}

func TestInjectClaimsFromHeaders_SuperAdminFalseWhenNotTrue(t *testing.T) {
	var captured Claims
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := ClaimsFromContext(r.Context())
		captured = c
	})

	handler := InjectClaimsFromHeaders(next)

	tests := []struct {
		headerVal string
	}{
		{"false"},
		{"1"},
		{"True"},
		{""},
	}
	for _, tc := range tests {
		t.Run("header="+tc.headerVal, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.headerVal != "" {
				req.Header.Set("X-Is-Super-Admin", tc.headerVal)
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)
			if captured.IsSuperAdmin {
				t.Errorf("IsSuperAdmin should be false for header value %q", tc.headerVal)
			}
		})
	}
}

func TestInjectClaimsFromHeaders_EmptyHeaders(t *testing.T) {
	var captured Claims
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := ClaimsFromContext(r.Context())
		if !ok {
			t.Error("claims should always be injected")
		}
		captured = c
	})

	handler := InjectClaimsFromHeaders(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if captured.UserID != "" {
		t.Errorf("UserID should be empty, got %q", captured.UserID)
	}
	if captured.IsSuperAdmin {
		t.Error("IsSuperAdmin should be false")
	}
}

func TestClaimsFromContext_ReturnsNilWhenNotSet(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, ok := ClaimsFromContext(req.Context())
	if ok {
		t.Error("expected ok=false when no claims in context")
	}
}

func TestClaimsFromContext_ReturnsClaimsWhenSet(t *testing.T) {
	var got Claims
	var gotOk bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, gotOk = ClaimsFromContext(r.Context())
	})

	handler := InjectClaimsFromHeaders(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-User-Id", "u1")
	req.Header.Set("X-Email", "u1@test.com")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !gotOk {
		t.Error("expected ok=true")
	}
	if got.UserID != "u1" {
		t.Errorf("UserID: got %q, want u1", got.UserID)
	}
}

func TestRequireSuperAdmin_BlocksNonSuperAdmin(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	handler := InjectClaimsFromHeaders(RequireSuperAdmin(next))

	tests := []struct {
		name    string
		userID  string
		isAdmin string
	}{
		{"no headers", "", ""},
		{"user only", "user-1", ""},
		{"user not admin", "user-1", "false"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.userID != "" {
				req.Header.Set("X-User-Id", tc.userID)
			}
			if tc.isAdmin != "" {
				req.Header.Set("X-Is-Super-Admin", tc.isAdmin)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if called {
				t.Error("next handler should not be called")
			}
			if w.Code != http.StatusForbidden {
				t.Errorf("status: got %d, want %d", w.Code, http.StatusForbidden)
			}
		})
	}
}

func TestRequireSuperAdmin_AllowsSuperAdmin(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := InjectClaimsFromHeaders(RequireSuperAdmin(next))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-User-Id", "admin-1")
	req.Header.Set("X-Is-Super-Admin", "true")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("next handler should have been called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRequireSuperAdmin_WithoutInjectMiddleware_Blocks(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	// No InjectClaimsFromHeaders — context has no claims
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	RequireSuperAdmin(next).ServeHTTP(w, req)

	if called {
		t.Error("next should not be called without claims")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRequireOrgAdmin_IsSameAsRequireSuperAdmin(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	// OrgAdmin allows super-admin
	handler := InjectClaimsFromHeaders(RequireOrgAdmin(next))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Is-Super-Admin", "true")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("RequireOrgAdmin should allow super-admin")
	}

	// OrgAdmin blocks non-admin
	called = false
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("X-User-Id", "user-1")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if called {
		t.Error("RequireOrgAdmin should block non-admin")
	}
}

func TestRequireOrgMember_AllowsAnyRequest(t *testing.T) {
	tests := []struct {
		name   string
		userID string
	}{
		{"with user", "user-1"},
		{"no user", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})
			handler := RequireOrgMember(next)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.userID != "" {
				req.Header.Set("X-User-Id", tc.userID)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if !called {
				t.Error("RequireOrgMember should call next handler")
			}
		})
	}
}

func TestSetOrgHeaders_SetsAllHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := Claims{
		UserID:       "user-99",
		Email:        "user99@example.com",
		IsSuperAdmin: true,
	}
	SetOrgHeaders(req, c)

	if req.Header.Get("X-User-Id") != "user-99" {
		t.Errorf("X-User-Id: got %q", req.Header.Get("X-User-Id"))
	}
	if req.Header.Get("X-Email") != "user99@example.com" {
		t.Errorf("X-Email: got %q", req.Header.Get("X-Email"))
	}
	if req.Header.Get("X-Is-Super-Admin") != "true" {
		t.Errorf("X-Is-Super-Admin: got %q", req.Header.Get("X-Is-Super-Admin"))
	}
}

func TestSetOrgHeaders_SuperAdminNotSetWhenFalse(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := Claims{
		UserID:       "user-1",
		Email:        "u@example.com",
		IsSuperAdmin: false,
	}
	SetOrgHeaders(req, c)

	if v := req.Header.Get("X-Is-Super-Admin"); v != "" {
		t.Errorf("X-Is-Super-Admin should not be set when false, got %q", v)
	}
}

func TestSetOrgHeaders_EmptyClaims(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	SetOrgHeaders(req, Claims{})

	// Should set empty strings for user/email, not set admin header
	if req.Header.Get("X-Is-Super-Admin") != "" {
		t.Errorf("X-Is-Super-Admin should not be set, got %q", req.Header.Get("X-Is-Super-Admin"))
	}
}
