package config

import "testing"

func TestEnvOverridesNestedKeys(t *testing.T) {
	t.Setenv("GITLAB_AI_GITLAB_BASE_URL", "https://env.example.com/")
	t.Setenv("GITLAB_AI_AI_PROVIDER", "gemini")
	t.Setenv("GITLAB_AI_AI_GEMINI_API_KEY", "from-env")
	t.Setenv("GITLAB_AI_CLI_IDLE_TIMEOUT_MINUTES", "5")

	cfg := loadFromYAML(t, "gitlab:\n  base_url: \"https://file.example.com/\"\n")

	if cfg.GitLab.BaseURL != "https://env.example.com/" {
		t.Errorf("base_url = %q, want env value", cfg.GitLab.BaseURL)
	}
	if cfg.AI.Provider != "gemini" {
		t.Errorf("provider = %q, want gemini", cfg.AI.Provider)
	}
	if cfg.AI.Gemini.APIKey != "from-env" {
		t.Errorf("gemini api_key = %q, want from-env", cfg.AI.Gemini.APIKey)
	}
	if cfg.CLI.IdleTimeoutMinutes != 5 {
		t.Errorf("idle_timeout_minutes = %d, want 5", cfg.CLI.IdleTimeoutMinutes)
	}
}
