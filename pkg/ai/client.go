package ai

import "context"

type ChatResult struct {
	Text       string
	Model      string
	DurationMs int64
}

type ChatClient interface {
	Chat(ctx context.Context, systemPrompt, userMessage string) (ChatResult, error)
	ProviderName() string
}
