package ai

import "context"

type ChatResult struct {
	Text  string
	Model string
}

type ChatClient interface {
	Chat(ctx context.Context, systemPrompt, userMessage string) (ChatResult, error)
	ProviderName() string
}
