package config

import "testing"

func TestResolveAI(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "nv-env")
	t.Setenv("ANTHROPIC_API_KEY", "")

	cases := []struct {
		name                       string
		ai                         AIConfig
		provider, key, source, mdl string
	}{
		{"nvidia key from env", AIConfig{Provider: "nvidia", Nvidia: NvidiaConfig{APIKeyEnv: "NVIDIA_API_KEY", Model: "m"}},
			"nvidia", "nv-env", "env NVIDIA_API_KEY", "m"},
		{"config key wins over env", AIConfig{Provider: "nvidia", Nvidia: NvidiaConfig{APIKey: "file", APIKeyEnv: "NVIDIA_API_KEY"}},
			"nvidia", "file", "config", ""},
		{"empty provider means anthropic", AIConfig{Anthropic: AnthropicConfig{APIKeyEnv: "ANTHROPIC_API_KEY"}},
			"anthropic", "", "", ""},
		{"google alias", AIConfig{Provider: "Google", Gemini: GeminiConfig{APIKey: "g"}},
			"gemini", "g", "config", ""},
		{"unknown", AIConfig{Provider: "bogus"}, "", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.ai.Resolve()
			if got.Provider != c.provider || got.APIKey != c.key || got.KeySource != c.source || got.Model != c.mdl {
				t.Errorf("Resolve() = %+v", got)
			}
		})
	}
}
