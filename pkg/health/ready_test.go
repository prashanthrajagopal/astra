package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockDBPinger implements DBPinger for testing.
type mockDBPinger struct {
	err error
}

func (m *mockDBPinger) PingContext(_ context.Context) error {
	return m.err
}

func TestReadyHandlerNilDeps(t *testing.T) {
	// When both db and rdb are nil, ReadyHandler should return 200.
	t.Setenv("READINESS_CHECKS", "db,redis")

	handler := ReadyHandler(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if body != `{"ready":true}` {
		t.Errorf("body = %q, want %q", body, `{"ready":true}`)
	}
}

func TestReadyHandlerDBOnly(t *testing.T) {
	// Only db check enabled (rdb is nil).
	t.Setenv("READINESS_CHECKS", "db")

	t.Run("db ping succeeds", func(t *testing.T) {
		handler := ReadyHandler(&mockDBPinger{err: nil}, nil)
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
		}
	})

	t.Run("db ping fails", func(t *testing.T) {
		handler := ReadyHandler(&mockDBPinger{err: errTest("connection refused")}, nil)
		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["ready"] != false {
			t.Errorf("ready = %v, want false", body["ready"])
		}
		if body["reason"] == "" {
			t.Error("expected non-empty reason")
		}
	})
}

func TestReadyHandlerNoChecks(t *testing.T) {
	// When READINESS_CHECKS is set to something that excludes both db and redis.
	t.Setenv("READINESS_CHECKS", "none")

	handler := ReadyHandler(&mockDBPinger{err: errTest("should not be called")}, nil)
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// errTest is a simple error type for tests.
type errTest string

func (e errTest) Error() string { return string(e) }
