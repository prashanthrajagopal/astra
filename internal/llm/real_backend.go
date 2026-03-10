package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type EndpointBackend struct {
	openAIKey    string
	anthropicKey string
	geminiKey    string
	ollamaHost   string
	fallback     string
	httpClient   *http.Client
}

func NewEndpointBackendFromEnv() *EndpointBackend {
	host := strings.TrimSuffix(strings.TrimSpace(os.Getenv("OLLAMA_HOST")), "/")
	if host == "" {
		host = "http://localhost:11434"
	}
	fallback := strings.TrimSpace(os.Getenv("OLLAMA_MODEL"))
	if fallback == "" {
		fallback = "llama3:8b"
	}
	return &EndpointBackend{
		openAIKey:    strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		anthropicKey: strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")),
		geminiKey:    strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		ollamaHost:   host,
		fallback:     fallback,
		httpClient:   &http.Client{Timeout: 300 * time.Second},
	}
}

func (b *EndpointBackend) Complete(ctx context.Context, model string, prompt string) (string, int, int, error) {
	provider, modelName := splitModel(model)
	switch provider {
	case "openai":
		if b.openAIKey != "" {
			if r, in, out, err := b.openAIComplete(ctx, modelName, prompt); err == nil {
				return r, in, out, nil
			}
		}
	case "anthropic":
		if b.anthropicKey != "" {
			if r, in, out, err := b.anthropicComplete(ctx, modelName, prompt); err == nil {
				return r, in, out, nil
			}
		}
	case "gemini", "google":
		if b.geminiKey != "" {
			if r, in, out, err := b.geminiComplete(ctx, modelName, prompt); err == nil {
				return r, in, out, nil
			}
		}
	case "ollama":
		if r, in, out, err := b.ollamaComplete(ctx, modelName, prompt); err == nil {
			return r, in, out, nil
		}
	}

	// Fallback path: always try local Ollama llama3:8b when cloud provider is unavailable.
	resp, in, out, err := b.ollamaComplete(ctx, b.fallback, prompt)
	if err != nil {
		return "", 0, 0, fmt.Errorf("llm fallback to ollama failed: %w", err)
	}
	return resp, in, out, nil
}

func (b *EndpointBackend) openAIComplete(ctx context.Context, model string, prompt string) (string, int, int, error) {
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
	err := b.postJSON(ctx, "https://api.openai.com/v1/chat/completions", reqBody, map[string]string{
		"Authorization": "Bearer " + b.openAIKey,
	}, &resp)
	if err != nil {
		return "", 0, 0, err
	}
	if len(resp.Choices) == 0 {
		return "", 0, 0, fmt.Errorf("openai returned no choices")
	}
	return resp.Choices[0].Message.Content, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, nil
}

func (b *EndpointBackend) anthropicComplete(ctx context.Context, model string, prompt string) (string, int, int, error) {
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
	err := b.postJSON(ctx, "https://api.anthropic.com/v1/messages", reqBody, map[string]string{
		"x-api-key":         b.anthropicKey,
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

func (b *EndpointBackend) geminiComplete(ctx context.Context, model string, prompt string) (string, int, int, error) {
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
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, b.geminiKey)
	err := b.postJSON(ctx, url, reqBody, nil, &resp)
	if err != nil {
		return "", 0, 0, err
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", 0, 0, fmt.Errorf("gemini returned no content")
	}
	return resp.Candidates[0].Content.Parts[0].Text, resp.UsageMetadata.PromptTokenCount, resp.UsageMetadata.CandidatesTokenCount, nil
}

func (b *EndpointBackend) ollamaComplete(ctx context.Context, model string, prompt string) (string, int, int, error) {
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
	err := b.postJSON(ctx, b.ollamaHost+"/api/generate", reqBody, nil, &resp)
	if err != nil {
		return "", 0, 0, err
	}
	return resp.Response, resp.PromptEvalCount, resp.EvalCount, nil
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
