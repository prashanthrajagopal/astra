package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadKV_MissingConfig(t *testing.T) {
	tests := []struct {
		name  string
		addr  string
		token string
		path  string
	}{
		{"missing_addr", "", "tok", "secret/data/app"},
		{"missing_token", "http://vault", "", "secret/data/app"},
		{"missing_path", "http://vault", "tok", ""},
		{"all_empty", "", "", ""},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadKV(context.Background(), tc.addr, tc.token, tc.path)
			if err == nil {
				t.Error("expected error for incomplete config, got nil")
			}
		})
	}
}

func TestLoadKV_KV2Response(t *testing.T) {
	// Vault KV v2 wraps secrets under data.data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "my-token" {
			t.Errorf("X-Vault-Token: got %q", r.Header.Get("X-Vault-Token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"data": map[string]any{
					"DB_PASSWORD": "s3cr3t",
					"API_KEY":     "key123",
				},
			},
		})
	}))
	defer server.Close()

	kv, err := LoadKV(context.Background(), server.URL, "my-token", "v1/secret/data/app")
	if err != nil {
		t.Fatalf("LoadKV: %v", err)
	}
	if kv["DB_PASSWORD"] != "s3cr3t" {
		t.Errorf("DB_PASSWORD: got %q", kv["DB_PASSWORD"])
	}
	if kv["API_KEY"] != "key123" {
		t.Errorf("API_KEY: got %q", kv["API_KEY"])
	}
}

func TestLoadKV_KV1Response(t *testing.T) {
	// Vault KV v1 puts secrets directly under data
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"MY_SECRET": "value1",
				"OTHER_KEY": "value2",
			},
		})
	}))
	defer server.Close()

	kv, err := LoadKV(context.Background(), server.URL, "tok", "v1/secret/app")
	if err != nil {
		t.Fatalf("LoadKV: %v", err)
	}
	if kv["MY_SECRET"] != "value1" {
		t.Errorf("MY_SECRET: got %q", kv["MY_SECRET"])
	}
}

func TestLoadKV_HTTPError(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"forbidden", 403},
		{"not_found", 404},
		{"server_error", 500},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer server.Close()

			_, err := LoadKV(context.Background(), server.URL, "tok", "v1/secret/app")
			if err == nil {
				t.Errorf("expected error for status %d", tc.status)
			}
		})
	}
}

func TestLoadKV_URLConstruction(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"KEY": "val",
			},
		})
	}))
	defer server.Close()

	// The source builds: addr (trailing slash stripped) + "/v1/" + TrimPrefix(path, "/")
	// So path "secret/myapp" → request path "/v1/secret/myapp"
	// Leading slash on path is trimmed, so "/secret/myapp" gives the same result.
	_, err := LoadKV(context.Background(), server.URL+"/", "tok", "/secret/myapp")
	if err != nil {
		t.Fatalf("LoadKV: %v", err)
	}
	if gotPath != "/v1/secret/myapp" {
		t.Errorf("unexpected path: %q", gotPath)
	}
}

func TestLoadKV_EmptyDataError(t *testing.T) {
	// Response with data key but empty map should return an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{},
		})
	}))
	defer server.Close()

	_, err := LoadKV(context.Background(), server.URL, "tok", "v1/secret/empty")
	if err == nil {
		t.Error("expected error for empty data, got nil")
	}
}

func TestLoadKV_TokenInHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("X-Vault-Token")
		if tok != "secret-token-abc" {
			t.Errorf("X-Vault-Token: got %q", tok)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"K": "V"},
		})
	}))
	defer server.Close()

	_, err := LoadKV(context.Background(), server.URL, "secret-token-abc", "v1/secret/app")
	if err != nil {
		t.Fatalf("LoadKV: %v", err)
	}
}
