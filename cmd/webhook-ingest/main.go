package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"astra/internal/messaging"
	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/health"
	"astra/pkg/httpx"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type webhookSource struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	HMACSecret     string `json:"hmac_secret"`
	ExpectedSchema string `json:"expected_schema"`
	Enabled        bool   `json:"enabled"`
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}

	database, err := db.Connect(cfg.PostgresDSN())
	if err != nil {
		slog.Error("database connect failed", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Error("redis connect failed", "err", err)
		os.Exit(1)
	}
	defer rdb.Close()

	if err := migrate(context.Background(), database); err != nil {
		slog.Error("migration failed", "err", err)
		os.Exit(1)
	}

	bus := messaging.New(cfg.RedisAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /ready", health.ReadyHandler(database, rdb))
	mux.HandleFunc("POST /webhooks/{source_id}", func(w http.ResponseWriter, r *http.Request) {
		handleWebhook(w, r, database, bus)
	})
	mux.HandleFunc("POST /webhook-sources", func(w http.ResponseWriter, r *http.Request) {
		handleCreateSource(w, r, database)
	})
	mux.HandleFunc("GET /webhook-sources", func(w http.ResponseWriter, r *http.Request) {
		handleListSources(w, r, database)
	})
	mux.HandleFunc("DELETE /webhook-sources/{id}", func(w http.ResponseWriter, r *http.Request) {
		handleDeleteSource(w, r, database)
	})

	port := getEnv("WEBHOOK_INGEST_PORT", "8099")
	srv := &http.Server{Addr: ":" + port, Handler: mux}

	go func() {
		slog.Info("webhook-ingest listening", "port", port)
		if err := httpx.ListenAndServe(srv, cfg); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}

func migrate(ctx context.Context, database *sql.DB) error {
	_, err := database.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS webhook_sources (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			source_id TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			hmac_secret TEXT NOT NULL,
			expected_schema TEXT DEFAULT '',
			enabled BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ DEFAULT now(),
			updated_at TIMESTAMPTZ DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("migrate webhook_sources: %w", err)
	}
	return nil
}

func handleWebhook(w http.ResponseWriter, r *http.Request, database *sql.DB, bus *messaging.Bus) {
	sourceID := r.PathValue("source_id")
	if sourceID == "" {
		http.Error(w, `{"error":"missing source_id"}`, http.StatusBadRequest)
		return
	}

	var source webhookSource
	err := database.QueryRowContext(r.Context(),
		`SELECT source_id, name, hmac_secret, expected_schema, enabled FROM webhook_sources WHERE source_id = $1`,
		sourceID).Scan(&source.ID, &source.Name, &source.HMACSecret, &source.ExpectedSchema, &source.Enabled)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"unknown source"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		slog.Error("webhook source lookup failed", "source_id", sourceID, "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if !source.Enabled {
		http.Error(w, `{"error":"source disabled"}`, http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, `{"error":"read body failed"}`, http.StatusBadRequest)
		return
	}

	signature := r.Header.Get("X-Signature-256")
	if signature == "" {
		signature = r.Header.Get("X-Hub-Signature-256")
	}
	if source.HMACSecret != "" {
		if !validateHMAC(body, signature, source.HMACSecret) {
			http.Error(w, `{"error":"invalid signature"}`, http.StatusUnauthorized)
			return
		}
	}

	triggerID := uuid.New().String()
	err = bus.Publish(r.Context(), "olympus:triggers:raw", map[string]interface{}{
		"trigger_id":  triggerID,
		"source_id":   sourceID,
		"source_name": source.Name,
		"payload":     string(body),
		"received_at": time.Now().Unix(),
	})
	if err != nil {
		slog.Error("publish webhook event failed", "source_id", sourceID, "err", err)
		http.Error(w, `{"error":"publish failed"}`, http.StatusInternalServerError)
		return
	}

	slog.Info("webhook received", "source_id", sourceID, "trigger_id", triggerID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"trigger_id": triggerID,
		"status":     "accepted",
	})
}

func validateHMAC(body []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func handleCreateSource(w http.ResponseWriter, r *http.Request, database *sql.DB) {
	var req struct {
		SourceID       string `json:"source_id"`
		Name           string `json:"name"`
		HMACSecret     string `json:"hmac_secret"`
		ExpectedSchema string `json:"expected_schema"`
		Enabled        *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.SourceID == "" || req.Name == "" {
		http.Error(w, `{"error":"source_id and name are required"}`, http.StatusBadRequest)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	var id string
	err := database.QueryRowContext(r.Context(),
		`INSERT INTO webhook_sources (source_id, name, hmac_secret, expected_schema, enabled)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (source_id) DO UPDATE
		   SET name = EXCLUDED.name,
		       hmac_secret = EXCLUDED.hmac_secret,
		       expected_schema = EXCLUDED.expected_schema,
		       enabled = EXCLUDED.enabled,
		       updated_at = now()
		 RETURNING id`,
		req.SourceID, req.Name, req.HMACSecret, req.ExpectedSchema, enabled,
	).Scan(&id)
	if err != nil {
		slog.Error("create webhook source failed", "source_id", req.SourceID, "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	slog.Info("webhook source created", "source_id", req.SourceID, "id", id)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":        id,
		"source_id": req.SourceID,
		"status":    "created",
	})
}

func handleListSources(w http.ResponseWriter, r *http.Request, database *sql.DB) {
	rows, err := database.QueryContext(r.Context(),
		`SELECT id, source_id, name, hmac_secret, expected_schema, enabled, created_at, updated_at
		 FROM webhook_sources ORDER BY created_at DESC`)
	if err != nil {
		slog.Error("list webhook sources failed", "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type sourceRow struct {
		ID             string    `json:"id"`
		SourceID       string    `json:"source_id"`
		Name           string    `json:"name"`
		HMACSecret     string    `json:"hmac_secret"`
		ExpectedSchema string    `json:"expected_schema"`
		Enabled        bool      `json:"enabled"`
		CreatedAt      time.Time `json:"created_at"`
		UpdatedAt      time.Time `json:"updated_at"`
	}
	sources := make([]sourceRow, 0)
	for rows.Next() {
		var s sourceRow
		if err := rows.Scan(&s.ID, &s.SourceID, &s.Name, &s.HMACSecret, &s.ExpectedSchema, &s.Enabled, &s.CreatedAt, &s.UpdatedAt); err != nil {
			slog.Error("scan webhook source row failed", "err", err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		sources = append(sources, s)
	}
	if err := rows.Err(); err != nil {
		slog.Error("webhook sources rows error", "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"sources": sources})
}

func handleDeleteSource(w http.ResponseWriter, r *http.Request, database *sql.DB) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}

	res, err := database.ExecContext(r.Context(),
		`DELETE FROM webhook_sources WHERE id = $1`, id)
	if err != nil {
		slog.Error("delete webhook source failed", "id", id, "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	slog.Info("webhook source deleted", "id", id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
