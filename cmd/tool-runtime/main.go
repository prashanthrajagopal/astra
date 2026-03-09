package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"astra/internal/tools"
	"astra/pkg/config"
	"astra/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.SetDefault(logger.New(cfg.LogLevel))

	runtime := newToolRuntime()

	port := 8083
	if p := os.Getenv("TOOL_RUNTIME_PORT"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil {
			port = parsed
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /execute", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name           string `json:"name"`
			Input          string `json:"input"` // base64
			TimeoutSeconds int    `json:"timeout_seconds"`
			MemoryLimit    int64  `json:"memory_limit"`
			CPULimit       float64 `json:"cpu_limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		var input []byte
		if req.Input != "" {
			var err error
			input, err = base64.StdEncoding.DecodeString(req.Input)
			if err != nil {
				http.Error(w, "invalid base64 input", http.StatusBadRequest)
				return
			}
		}
		timeout := 30 * time.Second
		if req.TimeoutSeconds > 0 {
			timeout = time.Duration(req.TimeoutSeconds) * time.Second
		}
		memLimit := int64(268435456)
		if req.MemoryLimit > 0 {
			memLimit = req.MemoryLimit
		}
		cpuLimit := 1.0
		if req.CPULimit > 0 {
			cpuLimit = req.CPULimit
		}

		toolReq := tools.ToolRequest{
			Name:        req.Name,
			Input:       input,
			Timeout:     timeout,
			MemoryLimit: memLimit,
			CPULimit:    cpuLimit,
		}

		toolResult, err := runtime.Execute(r.Context(), toolReq)
		if err != nil {
			slog.Error("execute failed", "name", req.Name, "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		outputB64 := base64.StdEncoding.EncodeToString(toolResult.Output)
		resp := map[string]interface{}{
			"output":     outputB64,
			"exit_code":  toolResult.ExitCode,
			"duration_ms": toolResult.Duration.Milliseconds(),
			"artifacts":  toolResult.Artifacts,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	srv := &http.Server{Addr: ":" + strconv.Itoa(port), Handler: mux}
	go func() {
		slog.Info("tool runtime listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "err", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down tool runtime")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}

func newToolRuntime() tools.Runtime {
	rt := strings.ToLower(strings.TrimSpace(os.Getenv("TOOL_RUNTIME")))
	if rt == "docker" {
		return tools.NewDockerRuntime()
	}
	return tools.NewNoopRuntime()
}
