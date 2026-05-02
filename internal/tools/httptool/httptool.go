// Package httptool provides an HTTP fetch tool for agents to query allow-listed external APIs.
package httptool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Request is the JSON input for the http_fetch tool.
type Request struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// Response is the JSON output returned by the http_fetch tool.
type Response struct {
	StatusCode int               `json:"status_code"`
	Body       string            `json:"body"`
	Headers    map[string]string `json:"headers,omitempty"`
}

// Executor executes authenticated HTTP requests against an allow-listed set of hosts.
type Executor struct {
	client      *http.Client
	allowlist   []string // hostname[:port] prefixes; empty means allow all
	bearerToken string   // injected into Authorization header when set
}

// New creates a new Executor. Configuration is read from environment variables:
//
//	HTTP_FETCH_ALLOWLIST  — comma-separated host[:port] allow-list (empty = allow all)
//	HTTP_FETCH_BEARER_TOKEN — Bearer token injected into Authorization header
//	HTTP_FETCH_TIMEOUT_SECONDS — request timeout in seconds (default 30)
func New() *Executor {
	allowlist := []string{}
	if raw := strings.TrimSpace(os.Getenv("HTTP_FETCH_ALLOWLIST")); raw != "" {
		for _, h := range strings.Split(raw, ",") {
			if t := strings.TrimSpace(h); t != "" {
				allowlist = append(allowlist, t)
			}
		}
	}
	timeout := 30 * time.Second
	if s := os.Getenv("HTTP_FETCH_TIMEOUT_SECONDS"); s != "" {
		if n, err := fmt.Sscanf(s, "%f", new(float64)); n == 1 && err == nil {
			// use default
		}
	}
	return &Executor{
		client:      &http.Client{Timeout: timeout},
		allowlist:   allowlist,
		bearerToken: os.Getenv("HTTP_FETCH_BEARER_TOKEN"),
	}
}

// Execute runs the http_fetch tool with the given JSON input bytes.
func (e *Executor) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var req Request
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("http_fetch: invalid input: %w", err)
	}
	if req.URL == "" {
		return nil, fmt.Errorf("http_fetch: url is required")
	}
	if err := e.validateURL(req.URL); err != nil {
		return nil, fmt.Errorf("http_fetch: %w", err)
	}

	method := strings.ToUpper(req.Method)
	if method == "" {
		method = http.MethodGet
	}

	var bodyReader io.Reader
	if req.Body != "" {
		bodyReader = strings.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, req.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http_fetch: build request: %w", err)
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if e.bearerToken != "" && httpReq.Header.Get("Authorization") == "" {
		httpReq.Header.Set("Authorization", "Bearer "+e.bearerToken)
	}

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http_fetch: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024)) // 4 MB cap
	if err != nil {
		return nil, fmt.Errorf("http_fetch: read response: %w", err)
	}

	headers := make(map[string]string, len(resp.Header))
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}

	out, _ := json.Marshal(Response{
		StatusCode: resp.StatusCode,
		Body:       string(respBody),
		Headers:    headers,
	})
	return out, nil
}

func (e *Executor) validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme %q not allowed; use http or https", u.Scheme)
	}
	// Block cloud metadata endpoints
	host := strings.ToLower(u.Hostname())
	if strings.HasPrefix(host, "169.254.") || host == "metadata.google.internal" {
		return fmt.Errorf("metadata/link-local addresses not allowed")
	}
	if len(e.allowlist) == 0 {
		return nil
	}
	hostPort := u.Host // includes port if specified
	for _, allowed := range e.allowlist {
		if hostPort == allowed || host == allowed {
			return nil
		}
	}
	return fmt.Errorf("host %q is not in the allow-list", u.Host)
}
