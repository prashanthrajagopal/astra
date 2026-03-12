package dashboard

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"astra/pkg/config"
	"astra/pkg/httpx"
)

type ServiceStatus struct {
	Name      string `json:"name"`
	Port      int    `json:"port"`
	Type      string `json:"type"`
	Healthy   bool   `json:"healthy"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

type Snapshot struct {
	GeneratedAt string              `json:"generated_at"`
	Services    []ServiceStatus     `json:"services"`
	Workers     []map[string]any    `json:"workers"`
	Approvals   []map[string]any    `json:"approvals"`
	Jobs        map[string]any      `json:"jobs"`
	Cost        map[string]any      `json:"cost"`
	Logs        map[string][]string `json:"logs"`
	PIDs        map[string]int      `json:"pids"`
	Agents      []map[string]any    `json:"agents,omitempty"`
	AgentCount  int                 `json:"agent_count"`
	Errors      map[string]string   `json:"errors,omitempty"`
}

type Collector struct {
	cfg    *config.Config
	client *http.Client
}

func NewCollector(cfg *config.Config) (*Collector, error) {
	client, err := httpx.NewClient(cfg, 120*time.Millisecond)
	if err != nil {
		return nil, err
	}
	return &Collector{cfg: cfg, client: client}, nil
}

func (c *Collector) Collect(ctx context.Context) Snapshot {
	snap := Snapshot{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Services:     probeServices(ctx),
		Workers:      []map[string]any{},
		Approvals:    []map[string]any{},
		Jobs:         map[string]any{},
		Cost:         map[string]any{"rows": []any{}},
		Logs:         map[string][]string{},
		PIDs:         map[string]int{},
		Agents:       []map[string]any{},
		Errors:       map[string]string{},
	}

	c.collectJobs(ctx, &snap)
	c.collectWorkers(ctx, &snap)
	c.collectApprovals(ctx, &snap)
	c.collectCost(ctx, &snap)

	services := serviceNames()
	for _, svc := range services {
		logFile := filepath.Join(c.cfg.LogsDir, svc+".log")
		lines, err := tailLines(logFile, 20)
		if err != nil {
			snap.Logs[svc] = []string{}
		} else {
			snap.Logs[svc] = lines
		}

		pidFile := filepath.Join(c.cfg.LogsDir, svc+".pid")
		pidBytes, err := os.ReadFile(pidFile)
		if err == nil {
			if pid, parseErr := strconv.Atoi(strings.TrimSpace(string(pidBytes))); parseErr == nil {
				snap.PIDs[svc] = pid
			}
		}
	}

	if len(snap.Errors) == 0 {
		snap.Errors = nil
	}
	return snap
}

func (c *Collector) goalServiceAddr() string {
	port := c.cfg.GoalServicePort
	if port == 0 {
		port = 8088
	}
	return fmt.Sprintf("http://localhost:%d", port)
}

func (c *Collector) collectJobs(ctx context.Context, snap *Snapshot) {
	if jobs, err := c.fetchObject(ctx, c.goalServiceAddr()+"/stats"); err == nil {
		snap.Jobs = jobs
	} else {
		snap.Errors["jobs"] = err.Error()
	}
}

func (c *Collector) collectWorkers(ctx context.Context, snap *Snapshot) {
	if workers, err := c.fetchArray(ctx, strings.TrimSuffix(c.cfg.WorkerManagerAddr, "/")+"/workers"); err == nil {
		snap.Workers = normalizeWorkers(workers)
	} else {
		snap.Errors["workers"] = err.Error()
	}
}

func (c *Collector) collectApprovals(ctx context.Context, snap *Snapshot) {
	if approvals, err := c.fetchArray(ctx, strings.TrimSuffix(c.cfg.AccessControlAddr, "/")+"/approvals/pending"); err == nil {
		snap.Approvals = approvals
	} else {
		snap.Errors["approvals"] = err.Error()
	}
}

func (c *Collector) collectCost(ctx context.Context, snap *Snapshot) {
	if cost, err := c.fetchObject(ctx, strings.TrimSuffix(c.cfg.CostTrackerAddr, "/")+"/cost/daily?days=7"); err == nil {
		snap.Cost = cost
	} else {
		snap.Errors["cost"] = err.Error()
	}
}

func (c *Collector) fetchArray(ctx context.Context, url string) ([]map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Collector) fetchObject(ctx context.Context, url string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func probeServices(ctx context.Context) []ServiceStatus {
	defs := []struct {
		name     string
		port     int
		typeName string
	}{
		{"api-gateway", 8080, "http"},
		{"task-service", 9090, "grpc"},
		{"agent-service", 9091, "grpc"},
		{"worker-manager", 8082, "http"},
		{"tool-runtime", 8083, "http"},
		{"prompt-manager", 8084, "http"},
		{"identity", 8085, "http"},
		{"access-control", 8086, "http"},
		{"planner-service", 8087, "http"},
		{"goal-service", 8088, "http"},
		{"evaluation-service", 8089, "http"},
		{"cost-tracker", 8090, "http"},
		{"memory-service", 9092, "grpc"},
		{"llm-router", 9093, "grpc"},
	}

	out := make([]ServiceStatus, len(defs))
	var wg sync.WaitGroup
	for i, d := range defs {
		wg.Add(1)
		go func(i int, d struct {
			name     string
			port     int
			typeName string
		}) {
			defer wg.Done()
			status := ServiceStatus{Name: d.name, Port: d.port, Type: d.typeName}
			start := time.Now()
			if d.typeName == "http" {
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://localhost:%d/health", d.port), nil)
				client := &http.Client{Timeout: 100 * time.Millisecond}
				resp, err := client.Do(req)
				if err == nil {
					resp.Body.Close()
					status.Healthy = resp.StatusCode < 300
					if !status.Healthy {
						status.Error = fmt.Sprintf("status %d", resp.StatusCode)
					}
				} else {
					status.Error = err.Error()
				}
			} else {
				conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", d.port), 100*time.Millisecond)
				if err == nil {
					status.Healthy = true
					conn.Close()
				} else {
					status.Error = err.Error()
				}
			}
			status.LatencyMs = time.Since(start).Milliseconds()
			out[i] = status
		}(i, d)
	}
	wg.Wait()
	return out
}

func tailLines(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		lines = append(lines, s.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func serviceNames() []string {
	return []string{
		"task-service", "agent-service", "scheduler-service", "execution-worker",
		"worker-manager", "tool-runtime", "browser-worker", "memory-service",
		"llm-router", "prompt-manager", "identity", "access-control",
		"planner-service", "goal-service", "evaluation-service", "cost-tracker", "api-gateway",
	}
}

func normalizeWorkers(in []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, w := range in {
		out = append(out, map[string]any{
			"id":             firstAny(w, "id", "ID"),
			"hostname":       firstAny(w, "hostname", "Hostname"),
			"status":         firstAny(w, "status", "Status"),
			"capabilities":   firstAny(w, "capabilities", "Capabilities"),
			"last_heartbeat": firstAny(w, "last_heartbeat", "LastHeartbeat", "lastHeartbeat"),
		})
	}
	return out
}

func firstAny(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}
