package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// HTTPClient is the Astra SDK HTTP client for agent-to-agent communication.
// It is distinct from DefaultAgentContext (gRPC-based) and provides a simpler
// REST interface for services that need to post goals or query goal status.
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
	agentID    string // The calling agent's ID
	authToken  string // Service-to-service JWT
}

// NewHTTPClient creates a new Astra SDK HTTP client.
func NewHTTPClient(baseURL, agentID, authToken string) *HTTPClient {
	return &HTTPClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		agentID:    agentID,
		authToken:  authToken,
	}
}

func (c *HTTPClient) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var req *http.Request
	var err error

	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("sdk: marshal request: %w", err)
		}
		req, err = http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("sdk: create request: %w", err)
		}
	} else {
		req, err = http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
		if err != nil {
			return nil, fmt.Errorf("sdk: create request: %w", err)
		}
	}

	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	req.Header.Set("X-Source-Agent-ID", c.agentID)

	return c.httpClient.Do(req)
}
