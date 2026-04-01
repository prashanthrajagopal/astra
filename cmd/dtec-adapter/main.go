package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"astra/internal/adapters"
	"astra/pkg/config"
)

// dtecAdapter implements adapters.Adapter for the D.TEC ecosystem.
type dtecAdapter struct {
	*adapters.BaseAdapter
}

// newDtecAdapter constructs a dtecAdapter.
func newDtecAdapter(endpoint, authToken string) *dtecAdapter {
	return &dtecAdapter{
		BaseAdapter: adapters.NewBaseAdapter("dtec", endpoint, authToken),
	}
}

// dtecDispatchRequest is the payload sent to the D.TEC dispatch endpoint.
type dtecDispatchRequest struct {
	Ref      string                `json:"ref"`
	GoalID   string                `json:"goal_id"`
	GoalText string                `json:"goal_text"`
	AgentID  string                `json:"agent_id"`
	Priority int                   `json:"priority"`
	Metadata json.RawMessage       `json:"metadata,omitempty"`
}

// dtecDispatchResponse is returned by the D.TEC dispatch endpoint.
type dtecDispatchResponse struct {
	JobID string `json:"job_id"`
	Error string `json:"error,omitempty"`
}

// dtecStatusResponse is returned by the D.TEC status endpoint.
type dtecStatusResponse struct {
	JobID  string          `json:"job_id"`
	Status string          `json:"status"`
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// dtecHealthResponse is returned by the D.TEC health endpoint.
type dtecHealthResponse struct {
	Healthy bool   `json:"healthy"`
	Message string `json:"message,omitempty"`
}

// DispatchGoal sends a goal to the D.TEC system and returns the resulting job ID.
func (d *dtecAdapter) DispatchGoal(ctx context.Context, ref string, goal adapters.GoalContext) (string, error) {
	payload := dtecDispatchRequest{
		Ref:      ref,
		GoalID:   goal.GoalID,
		GoalText: goal.GoalText,
		AgentID:  goal.AgentID,
		Priority: goal.Priority,
		Metadata: goal.Metadata,
	}
	resp, err := d.BaseAdapter.DoRequest(ctx, http.MethodPost, "/v1/goals/dispatch", payload)
	if err != nil {
		return "", fmt.Errorf("dtec: dispatch goal: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("dtec: dispatch goal: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result dtecDispatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("dtec: dispatch goal: decode response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("dtec: dispatch goal: %s", result.Error)
	}
	if result.JobID == "" {
		return "", fmt.Errorf("dtec: dispatch goal: empty job_id in response")
	}
	return result.JobID, nil
}

// PollStatus queries the D.TEC system for the current status of a job.
func (d *dtecAdapter) PollStatus(ctx context.Context, jobID string) (*adapters.JobResult, error) {
	resp, err := d.BaseAdapter.DoRequest(ctx, http.MethodGet, "/v1/jobs/"+jobID+"/status", nil)
	if err != nil {
		return nil, fmt.Errorf("dtec: poll status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("dtec: poll status: job %q not found", jobID)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dtec: poll status: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result dtecStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("dtec: poll status: decode response: %w", err)
	}

	jobResult := &adapters.JobResult{
		Output: result.Output,
		Error:  result.Error,
	}
	switch result.Status {
	case "pending":
		jobResult.Status = adapters.StatusPending
	case "running":
		jobResult.Status = adapters.StatusRunning
	case "completed":
		jobResult.Status = adapters.StatusCompleted
	case "failed":
		jobResult.Status = adapters.StatusFailed
	default:
		jobResult.Status = adapters.StatusPending
	}
	return jobResult, nil
}

// HandleCallback processes a D.TEC webhook payload.
func (d *dtecAdapter) HandleCallback(ctx context.Context, payload json.RawMessage) error {
	var cb struct {
		JobID  string          `json:"job_id"`
		Event  string          `json:"event"`
		Status string          `json:"status"`
		Data   json.RawMessage `json:"data,omitempty"`
	}
	if err := json.Unmarshal(payload, &cb); err != nil {
		return fmt.Errorf("dtec: handle callback: decode payload: %w", err)
	}
	if cb.JobID == "" {
		return fmt.Errorf("dtec: handle callback: missing job_id")
	}
	slog.Info("dtec callback received",
		"job_id", cb.JobID,
		"event", cb.Event,
		"status", cb.Status,
	)
	return nil
}

// ListCapabilities returns the capabilities offered by the D.TEC adapter.
func (d *dtecAdapter) ListCapabilities(_ context.Context) ([]adapters.Capability, error) {
	return []adapters.Capability{
		{
			Name:        "venue_monitoring",
			Description: "Real-time monitoring of venue environments including occupancy, access, and environmental sensors",
			Version:     "1.0",
		},
		{
			Name:        "crowd_analytics",
			Description: "Crowd density detection, flow analysis, and anomaly alerting for large public spaces",
			Version:     "1.0",
		},
		{
			Name:        "perimeter_security",
			Description: "Perimeter breach detection, access-control enforcement, and threat escalation",
			Version:     "1.0",
		},
	}, nil
}

// HealthCheck verifies connectivity to the D.TEC system.
func (d *dtecAdapter) HealthCheck(ctx context.Context) (bool, error) {
	resp, err := d.BaseAdapter.DoRequest(ctx, http.MethodGet, "/v1/health", nil)
	if err != nil {
		return false, fmt.Errorf("dtec: health check: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}
	var result dtecHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// A 200 with undecodable body still counts as healthy.
		return true, nil
	}
	return result.Healthy, nil
}

// ---- HTTP handlers ----

func handleHealth(registry *adapters.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		statuses := registry.HealthCheckAll(ctx)
		allHealthy := true
		for _, healthy := range statuses {
			if !healthy {
				allHealthy = false
				break
			}
		}

		code := http.StatusOK
		if !allHealthy {
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"healthy":  allHealthy,
			"adapters": statuses,
		})
	}
}

func handleCallback(adapter adapters.Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Error("dtec-adapter: read callback body", "err", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		if err := adapter.HandleCallback(r.Context(), json.RawMessage(body)); err != nil {
			slog.Error("dtec-adapter: handle callback", "err", err)
			http.Error(w, "callback processing failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func handleCapabilities(adapter adapters.Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caps, err := adapter.ListCapabilities(r.Context())
		if err != nil {
			slog.Error("dtec-adapter: list capabilities", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(caps)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	_, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}

	dtecEndpoint := getEnv("DTEC_ENDPOINT", "https://api.dtec.example.com")
	dtecAuthToken := os.Getenv("DTEC_AUTH_TOKEN")
	if dtecAuthToken == "" {
		slog.Warn("DTEC_AUTH_TOKEN not set; requests to D.TEC will be unauthenticated")
	}

	adapter := newDtecAdapter(dtecEndpoint, dtecAuthToken)

	registry := adapters.NewRegistry()
	if err := registry.Register(adapter); err != nil {
		slog.Error("dtec-adapter: register adapter", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth(registry))
	mux.HandleFunc("GET /ready", handleHealth(registry))
	mux.HandleFunc("POST /callbacks/dtec", handleCallback(adapter))
	mux.HandleFunc("GET /adapters/dtec/capabilities", handleCapabilities(adapter))

	port := getEnv("DTEC_ADAPTER_PORT", "8098")
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("dtec-adapter listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("dtec-adapter server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("dtec-adapter shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("dtec-adapter shutdown error", "err", err)
	}
}
