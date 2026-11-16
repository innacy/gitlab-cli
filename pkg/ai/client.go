package ai

import (
	"context"
	"strings"
)

type ChatResult struct {
	Text         string
	Model        string
	DurationMs   int64
	InputTokens  int // as reported by the provider; 0 if unknown
	OutputTokens int
}

// endpoint joins a configured base URL (scheme://host[/prefix]) with an API
// path, falling back to the provider's public host.
func endpoint(baseURL, defaultHost, path string) string {
	if baseURL == "" {
		baseURL = defaultHost
	}
	return strings.TrimSuffix(baseURL, "/") + path
}

type ChatClient interface {
	Chat(ctx context.Context, systemPrompt, userMessage string) (ChatResult, error)
	ProviderName() string
}
