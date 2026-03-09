package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bradfitz/gomemcache/memcache"

	"astra/internal/prompt"
	"astra/pkg/config"
	"astra/pkg/db"
	"astra/pkg/logger"
)

const (
	cacheTTLSeconds       = 300
	headerContentType    = "Content-Type"
	contentTypeJSON      = "application/json"
	msgMethodNotAllowed  = "method not allowed"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logger.New(cfg.LogLevel))

	dbConn, err := db.Connect(cfg.PostgresDSN())
	if err != nil {
		slog.Error("failed to connect to database", "err", err)
		os.Exit(1)
	}
	defer dbConn.Close()

	mc := memcache.New(cfg.MemcachedAddr)
	store := prompt.NewStore(dbConn)
	server := &server{store: store, mc: mc}

	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/prompts/", server.handlePromptsPath)
	http.HandleFunc("/prompts", server.handlePromptsPost)

	addr := fmt.Sprintf(":%d", cfg.PromptManagerPort)
	slog.Info("prompt manager started", "addr", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, msgMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// handlePromptsPath serves GET /prompts/:name/:version (cache-aside).
func (s *server) handlePromptsPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, msgMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/prompts/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "path must be /prompts/{name}/{version}", http.StatusBadRequest)
		return
	}
	name, version := parts[0], parts[1]

	cacheKey := fmt.Sprintf("prompt:%s:%s", name, version)
	if s.mc != nil {
		item, err := s.mc.Get(cacheKey)
	if err == nil {
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusOK)
		w.Write(item.Value)
		return
	}
		if err != memcache.ErrCacheMiss {
			slog.Warn("memcached get failed", "key", cacheKey, "err", err)
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	p, err := s.store.GetPrompt(ctx, name, version)
	if err != nil {
		slog.Error("GetPrompt failed", "name", name, "version", version, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if p == nil {
		http.NotFound(w, r)
		return
	}

	resp := promptResponse(p)
	body, _ := json.Marshal(resp)
	if s.mc != nil {
		_ = s.mc.Set(&memcache.Item{
			Key:        cacheKey,
			Value:      body,
			Expiration: cacheTTLSeconds,
		})
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// handlePromptsPost serves POST /prompts (write-through: DB then cache).
func (s *server) handlePromptsPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, msgMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name            string          `json:"name"`
		Version         string          `json:"version"`
		Body            string          `json:"body"`
		VariablesSchema json.RawMessage `json:"variables_schema"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Version == "" {
		http.Error(w, "name and version required", http.StatusBadRequest)
		return
	}

	p := &prompt.Prompt{
		Name:            req.Name,
		Version:         req.Version,
		Body:            req.Body,
		VariablesSchema: []byte(req.VariablesSchema),
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.store.SavePrompt(ctx, p); err != nil {
		slog.Error("SavePrompt failed", "name", req.Name, "version", req.Version, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Write-through: set cache
	cacheKey := fmt.Sprintf("prompt:%s:%s", req.Name, req.Version)
	resp := map[string]interface{}{
		"name":             req.Name,
		"version":          req.Version,
		"body":             req.Body,
		"variables_schema": req.VariablesSchema,
	}
	body, _ := json.Marshal(resp)
	if s.mc != nil {
		_ = s.mc.Set(&memcache.Item{
			Key:        cacheKey,
			Value:      body,
			Expiration: cacheTTLSeconds,
		})
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

type server struct {
	store *prompt.Store
	mc    *memcache.Client
}

func promptResponse(p *prompt.Prompt) map[string]interface{} {
	var schema json.RawMessage
	if len(p.VariablesSchema) > 0 {
		schema = p.VariablesSchema
	} else {
		schema = []byte("null")
	}
	return map[string]interface{}{
		"id":               p.ID.String(),
		"name":             p.Name,
		"version":          p.Version,
		"body":             p.Body,
		"variables_schema": schema,
		"created_at":       p.CreatedAt,
		"updated_at":       p.UpdatedAt,
	}
}
