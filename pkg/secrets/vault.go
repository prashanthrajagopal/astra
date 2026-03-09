package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func LoadKV(ctx context.Context, addr, token, path string) (map[string]string, error) {
	if addr == "" || token == "" || path == "" {
		return nil, fmt.Errorf("vault config incomplete")
	}

	url := strings.TrimSuffix(addr, "/") + "/v1/" + strings.TrimPrefix(path, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("vault request: %w", err)
	}
	req.Header.Set("X-Vault-Token", token)

	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("vault returned status %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("vault decode: %w", err)
	}

	out := map[string]string{}
	data, _ := body["data"].(map[string]any)
	if inner, ok := data["data"].(map[string]any); ok {
		for k, v := range inner {
			out[k] = fmt.Sprint(v)
		}
		return out, nil
	}
	for k, v := range data {
		out[k] = fmt.Sprint(v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("vault response missing data")
	}
	return out, nil
}
