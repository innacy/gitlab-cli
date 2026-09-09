package ai

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

const nvidiaAPI = "https://integrate.api.nvidia.com/v1/chat/completions"

// NvidiaClient calls the NVIDIA NIM API (OpenAI-compatible endpoint).
type NvidiaClient struct {
	apiKey          string
	model           string
	maxTokens       int
	temperature     float64
	topP            float64
	enableThinking  bool
	reasoningBudget int
	httpClient      *http.Client
	stream          bool
}

// NvidiaOpts holds optional NVIDIA-specific parameters.
type NvidiaOpts struct {
	Temperature     float64
	TopP            float64
	EnableThinking  bool
	ReasoningBudget int
	Stream          bool
}

// NewNvidiaClient creates a new NVIDIA NIM API client.
func NewNvidiaClient(apiKey, model string, maxTokens int, timeout time.Duration, opts NvidiaOpts) *NvidiaClient {
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	return &NvidiaClient{
		apiKey:          apiKey,
		model:           model,
		maxTokens:       maxTokens,
		temperature:     opts.Temperature,
		topP:            opts.TopP,
		stream:          opts.Stream,
		reasoningBudget: opts.ReasoningBudget,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// ProviderName returns "NVIDIA".
func (c *NvidiaClient) ProviderName() string { return "NVIDIA" }

// ─── Request / Response types (OpenAI-compatible) ────────────────────────────

type openAIChatRequest struct {
	Model              string              `json:"model"`
	Messages           []openAIMessage     `json:"messages"`
	MaxTokens          int                 `json:"max_tokens"`
	Temperature        float64             `json:"temperature"`
	TopP               float64             `json:"top_p,omitempty"`
	ChatTemplateKwargs *chatTemplateKwargs `json:"chat_template_kwargs,omitempty"`
	ReasoningBudget    int                 `json:"reasoning_budget,omitempty"`
}

type chatTemplateKwargs struct {
	EnableThinking bool `json:"enable_thinking"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []openAIChoice  `json:"choices"`
	Error   *openAIAPIError `json:"error,omitempty"`
}

type openAIChoice struct {
	Message openAIMessage `json:"message"`
}

type openAIAPIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// ─── Public API ──────────────────────────────────────────────────────────────

const (
	nvidiaMaxRetries    = 4
	nvidiaBaseBackoff   = 2 * time.Second
	nvidiaMaxBackoff    = 30 * time.Second
	nvidiaBackoffFactor = 2.0
)

func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests ||
		code == http.StatusServiceUnavailable ||
		code == http.StatusBadGateway ||
		code == http.StatusGatewayTimeout
}

// Chat sends a message with an optional system prompt and returns the text response.
// Retries with exponential backoff on transient errors (429, 502, 503, 504).
func (c *NvidiaClient) Chat(ctx context.Context, systemPrompt, userMessage string) (ChatResult, error) {
	start := time.Now()
	messages := make([]openAIMessage, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, openAIMessage{Role: "user", Content: userMessage})

	reqBody := openAIChatRequest{
		Model:       c.model,
		Messages:    messages,
		MaxTokens:   c.maxTokens,
		Temperature: c.temperature,
		TopP:        c.topP,
	}

	if c.enableThinking {
		reqBody.ChatTemplateKwargs = &chatTemplateKwargs{EnableThinking: true}
	}
	if c.reasoningBudget > 0 {
		reqBody.ReasoningBudget = c.reasoningBudget
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return ChatResult{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	backoff := nvidiaBaseBackoff

	for attempt := 0; attempt <= nvidiaMaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ChatResult{}, fmt.Errorf("request cancelled during retry backoff: %w", ctx.Err())
			case <-time.After(backoff):
			}
			backoff = time.Duration(float64(backoff) * nvidiaBackoffFactor)
			if backoff > nvidiaMaxBackoff {
				backoff = nvidiaMaxBackoff
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", nvidiaAPI, bytes.NewReader(body))
		if err != nil {
			return ChatResult{}, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("API request failed: %w", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		if isRetryableStatus(resp.StatusCode) {
			var errResp openAIChatResponse
			if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
				lastErr = fmt.Errorf("NVIDIA API error (%d): %s", resp.StatusCode, errResp.Error.Message)
			} else {
				lastErr = fmt.Errorf("NVIDIA API error (%d): %s", resp.StatusCode, string(respBody))
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			var errResp openAIChatResponse
			if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != nil {
				return ChatResult{}, fmt.Errorf("NVIDIA API error (%d): %s", resp.StatusCode, errResp.Error.Message)
			}
			return ChatResult{}, fmt.Errorf("NVIDIA API error (%d): %s", resp.StatusCode, string(respBody))
		}

		var chatResp openAIChatResponse
		if err := json.Unmarshal(respBody, &chatResp); err != nil {
			return ChatResult{}, fmt.Errorf("failed to parse response: %w", err)
		}

		if chatResp.Error != nil {
			return ChatResult{}, fmt.Errorf("NVIDIA API error: %s", chatResp.Error.Message)
		}

		var result strings.Builder
		for _, choice := range chatResp.Choices {
			result.WriteString(choice.Message.Content)
		}

		text := result.String()
		if text == "" {
			return ChatResult{}, fmt.Errorf("empty response from NVIDIA API")
		}

		return ChatResult{Text: text, Model: c.model, DurationMs: time.Since(start).Milliseconds()}, nil
	}

	return ChatResult{}, fmt.Errorf("NVIDIA API exhausted %d retries: %w", nvidiaMaxRetries, lastErr)
}
