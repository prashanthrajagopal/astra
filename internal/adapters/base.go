package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BaseAdapter provides shared HTTP plumbing that concrete adapters can embed.
type BaseAdapter struct {
	endpoint   string
	authToken  string
	httpClient *http.Client
	ecosystem  string
}

// NewBaseAdapter constructs a BaseAdapter with a default 30 s HTTP timeout.
func NewBaseAdapter(ecosystem, endpoint, authToken string) *BaseAdapter {
	return &BaseAdapter{
		ecosystem: ecosystem,
		endpoint:  endpoint,
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name satisfies part of the Adapter interface.
func (b *BaseAdapter) Name() string { return b.ecosystem }

// DoRequest executes an HTTP request against the adapter's endpoint.
// path is appended to the base endpoint URL.
// body is JSON-encoded when non-nil; pass nil for requests without a body.
// The caller is responsible for closing the returned response body.
func (b *BaseAdapter) DoRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("adapters: marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := b.endpoint + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("adapters: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("adapters: %s %s: %w", method, url, err)
	}
	return resp, nil
}

// pollWithBackoff is a convenience helper for implementations that need to
// retry a status poll.  It calls poll up to maxAttempts times with exponential
// back-off starting at initialDelay (capped at 10 s).  It returns the first
// non-nil *JobResult whose Status is not StatusPending/StatusRunning, or the
// last result when maxAttempts is exhausted.
func (b *BaseAdapter) pollWithBackoff(
	ctx context.Context,
	maxAttempts int,
	initialDelay time.Duration,
	poll func(ctx context.Context) (*JobResult, error),
) (*JobResult, error) {
	delay := initialDelay
	var last *JobResult
	for i := 0; i < maxAttempts; i++ {
		result, err := poll(ctx)
		if err != nil {
			return nil, err
		}
		last = result
		if result.Status != StatusPending && result.Status != StatusRunning {
			return result, nil
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
		if delay > 10*time.Second {
			delay = 10 * time.Second
		}
	}
	return last, nil
}
