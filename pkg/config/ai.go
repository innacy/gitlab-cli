package config

import (
	"os"
	"strings"
)

// ResolvedAI is the active AI provider after resolving aliases and the
// config-file / environment key lookup.
type ResolvedAI struct {
	Provider  string // "anthropic", "gemini", "nvidia"; "" if unknown
	Label     string // display name
	Model     string
	APIKey    string
	KeyEnv    string // env var consulted when api_key is empty
	KeySource string // "config", "env <NAME>", or "" when no key is set
}

// Resolve returns the active provider's settings. A key in the config file
// wins over the environment variable, matching how the clients are built.
func (c *AIConfig) Resolve() ResolvedAI {
	var r ResolvedAI
	var key string
	switch strings.ToLower(c.Provider) {
	case "anthropic", "claude", "":
		r = ResolvedAI{Provider: "anthropic", Label: "Anthropic (Claude)", Model: c.Anthropic.Model, KeyEnv: c.Anthropic.APIKeyEnv}
		key = c.Anthropic.APIKey
	case "gemini", "google":
		r = ResolvedAI{Provider: "gemini", Label: "Google Gemini", Model: c.Gemini.Model, KeyEnv: c.Gemini.APIKeyEnv}
		key = c.Gemini.APIKey
	case "nvidia":
		r = ResolvedAI{Provider: "nvidia", Label: "NVIDIA NIM", Model: c.Nvidia.Model, KeyEnv: c.Nvidia.APIKeyEnv}
		key = c.Nvidia.APIKey
	default:
		return ResolvedAI{Label: c.Provider}
	}
	switch {
	case key != "":
		r.APIKey, r.KeySource = key, "config"
	case r.KeyEnv != "" && os.Getenv(r.KeyEnv) != "":
		r.APIKey, r.KeySource = os.Getenv(r.KeyEnv), "env "+r.KeyEnv
	}
	return r
}
