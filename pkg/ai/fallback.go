package ai

import (
	"context"
	"fmt"
)

// FallbackClient wraps a primary and fallback ChatClient of the same provider.
// If the primary model fails, the request is retried on the fallback.
type FallbackClient struct {
	primary  ChatClient
	fallback ChatClient
}

func NewFallbackClient(primary, fallback ChatClient) *FallbackClient {
	return &FallbackClient{primary: primary, fallback: fallback}
}

func (c *FallbackClient) ProviderName() string {
	return c.primary.ProviderName()
}

func (c *FallbackClient) Chat(ctx context.Context, systemPrompt, userMessage string) (ChatResult, error) {
	result, err := c.primary.Chat(ctx, systemPrompt, userMessage)
	if err == nil {
		return result, nil
	}

	primaryErr := err
	result, err = c.fallback.Chat(ctx, systemPrompt, userMessage)
	if err != nil {
		return ChatResult{}, fmt.Errorf("primary (%v); fallback (%v)", primaryErr, err)
	}
	return result, nil
}
