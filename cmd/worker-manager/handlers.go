package main

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"astra/internal/workers"
)

type server struct {
	registry *workers.Registry
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *server) handleWorkers(w http.ResponseWriter, r *http.Request) {
	active, err := s.registry.ListActive(r.Context())
	if err != nil {
		slog.Error("list active workers failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(active)
}
