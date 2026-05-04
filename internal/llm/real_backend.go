package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type EndpointBackend struct {
	store      *ConfigStore
	httpClient *http.Client
}

func NewEndpointBackendFromDB(store *ConfigStore) *EndpointBackend {
	return &EndpointBackend{
		store:      store,
		httpClient: &http.Client{Timeout: 300 * time.Second},
	}
}

func (b *EndpointBackend) Complete(ctx context.Context, model string, prompt string) (string, int, int, error) {
	provider, modelName := splitModel(model)

	if cfg, ok := b.store.Get(provider); ok && cfg.Enabled {
		if modelName == "" {
			modelName = cfg.ModelName
		}
		if r, in, out, err := b.completeWith(ctx, cfg, modelName, prompt); err == nil {
			return r, in, out, nil
		}
	}

	def, ok := b.store.GetDefault()
	if !ok {
		return "", 0, 0, fmt.Errorf("no default LLM provider configured")
	}
	r, in, out, err := b.completeWith(ctx, def, def.ModelName, prompt)
	if err != nil {
		return "", 0, 0, fmt.Errorf("llm fallback to %s failed: %w", def.Provider, err)
	}
	return r, in, out, nil
}

func (b *EndpointBackend) completeWith(ctx context.Context, cfg ProviderConfig, modelName string, prompt string) (string, int, int, error) {
	switch cfg.Provider {
	case "openai":
		return b.openAIComplete(ctx, modelName, prompt, cfg.HostURL, cfg.APIKey)
	case "anthropic":
		return b.anthropicComplete(ctx, modelName, prompt, cfg.HostURL, cfg.APIKey)
	case "gemini", "google":
		return b.geminiComplete(ctx, modelName, prompt, cfg.HostURL, cfg.APIKey)
	case "ollama":
		return b.ollamaComplete(ctx, modelName, prompt, cfg.HostURL)
	case "mlx":
		return b.mlxComplete(ctx, modelName, prompt, cfg.HostURL)
	default:
		return "", 0, 0, fmt.Errorf("unknown provider: %s", cfg.Provider)
	}
}

func (b *EndpointBackend) openAIComplete(ctx context.Context, model, prompt, hostURL, apiKey string) (string, int, int, error) {
	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	url := strings.TrimSuffix(hostURL, "/") + "/chat/completions"
	err := b.postJSON(ctx, url, reqBody, map[string]string{
		"Authorization": "Bearer " + apiKey,
	}, &resp)
	if err != nil {
		return "", 0, 0, err
	}
	if len(resp.Choices) == 0 {
		return "", 0, 0, fmt.Errorf("openai returned no choices")
	}
	return resp.Choices[0].Message.Content, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, nil
}

func (b *EndpointBackend) anthropicComplete(ctx context.Context, model, prompt, hostURL, apiKey string) (string, int, int, error) {
	reqBody := map[string]any{
		"model":      model,
		"max_tokens": 512,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	var resp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	url := strings.TrimSuffix(hostURL, "/") + "/messages"
	err := b.postJSON(ctx, url, reqBody, map[string]string{
		"x-api-key":         apiKey,
		"anthropic-version": "2023-06-01",
	}, &resp)
	if err != nil {
		return "", 0, 0, err
	}
	if len(resp.Content) == 0 {
		return "", 0, 0, fmt.Errorf("anthropic returned no content")
	}
	return resp.Content[0].Text, resp.Usage.InputTokens, resp.Usage.OutputTokens, nil
}

func (b *EndpointBackend) geminiComplete(ctx context.Context, model, prompt, hostURL, apiKey string) (string, int, int, error) {
	reqBody := map[string]any{
		"contents": []map[string]any{
			{"parts": []map[string]string{{"text": prompt}}},
		},
	}
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", strings.TrimSuffix(hostURL, "/"), model, apiKey)
	err := b.postJSON(ctx, url, reqBody, nil, &resp)
	if err != nil {
		return "", 0, 0, err
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", 0, 0, fmt.Errorf("gemini returned no content")
	}
	return resp.Candidates[0].Content.Parts[0].Text, resp.UsageMetadata.PromptTokenCount, resp.UsageMetadata.CandidatesTokenCount, nil
}

func (b *EndpointBackend) ollamaComplete(ctx context.Context, model, prompt, hostURL string) (string, int, int, error) {
	reqBody := map[string]any{
		"model":  model,
		"prompt": prompt,
		"stream": false,
	}
	var resp struct {
		Response        string `json:"response"`
		PromptEvalCount int    `json:"prompt_eval_count"`
		EvalCount       int    `json:"eval_count"`
	}
	err := b.postJSON(ctx, strings.TrimSuffix(hostURL, "/")+"/api/generate", reqBody, nil, &resp)
	if err != nil {
		return "", 0, 0, err
	}
	return resp.Response, resp.PromptEvalCount, resp.EvalCount, nil
}

func (b *EndpointBackend) mlxComplete(ctx context.Context, model, prompt, hostURL string) (string, int, int, error) {
	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	base := strings.TrimSuffix(hostURL, "/")
	err := b.postJSON(ctx, base+"/v1/chat/completions", reqBody, nil, &resp)
	if err != nil && isNotFound(err) {
		err = b.postJSON(ctx, base+"/chat/completions", reqBody, nil, &resp)
	}
	if err != nil {
		return "", 0, 0, err
	}
	if len(resp.Choices) == 0 {
		return "", 0, 0, fmt.Errorf("mlx returned no choices")
	}
	return resp.Choices[0].Message.Content, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, nil
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "status 404")
}

func (b *EndpointBackend) postJSON(ctx context.Context, url string, reqBody any, headers map[string]string, out any) error {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func splitModel(model string) (provider, modelName string) {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) < 2 {
		return "ollama", model
	}
	return strings.ToLower(parts[0]), parts[1]
}
