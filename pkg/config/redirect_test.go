package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAIBaseURL(t *testing.T) {
	cases := map[string]bool{ // url -> valid
		"https://proxy.example.com":     true,
		"http://127.0.0.1:8080":         true,
		"http://localhost:9000/":        true,
		"http://[::1]:1":                true,
		"http://proxy.example.com":      false,
		"https://user:pw@proxy.example": false,
		"https://proxy.example/?x=1":    false,
		"ftp://proxy.example":           false,
		"not a url":                     false,
	}
	for raw, valid := range cases {
		r := &ValidationResult{}
		validateAI(&AIConfig{Anthropic: AnthropicConfig{BaseURL: raw}}, r)
		if (len(r.Errors) == 0) != valid {
			t.Errorf("base_url %q: errors=%v, want valid=%v", raw, r.Errors, valid)
		}
	}
}

func TestUntrustedAIBaseURLIsIgnored(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	yaml := "ai:\n  anthropic:\n    base_url: https://evil.example/\n"

	// Found by search in the cwd: dropped, with a warning.
	t.Chdir(t.TempDir())
	os.WriteFile("config.yaml", []byte(yaml), 0o600)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI.Anthropic.BaseURL != "" {
		t.Errorf("cwd base_url used: %q", cfg.AI.Anthropic.BaseURL)
	}
	if w := cfg.AIRedirectWarnings(); len(w) != 1 || !strings.Contains(w[0], "ignoring ai.anthropic.base_url") {
		t.Errorf("warnings = %v", w)
	}

	// A symlink into the cwd pretending to be the user dir does not count.
	os.MkdirAll(filepath.Join(xdg, "git-agent"), 0o700)
	if inUserDir("config.yaml") {
		t.Error("cwd file reported as inside the user dir")
	}

	// Env var override is the user's own choice: kept.
	t.Setenv("GITLAB_AI_AI_ANTHROPIC_BASE_URL", "https://proxy.example/")
	cfg, _ = Load()
	if cfg.AI.Anthropic.BaseURL != "https://proxy.example/" || len(cfg.AIRedirectWarnings()) != 0 {
		t.Errorf("env base_url = %q, warnings %v", cfg.AI.Anthropic.BaseURL, cfg.AIRedirectWarnings())
	}
	t.Setenv("GITLAB_AI_AI_ANTHROPIC_BASE_URL", "")
	os.Unsetenv("GITLAB_AI_AI_ANTHROPIC_BASE_URL")

	// Explicit --config: kept.
	cfg, _ = LoadFrom("config.yaml")
	if cfg.AI.Anthropic.BaseURL != "https://evil.example/" {
		t.Errorf("explicit --config base_url dropped")
	}

	// User config dir: kept.
	t.Chdir(t.TempDir())
	os.WriteFile(filepath.Join(xdg, "git-agent", "config.yaml"), []byte(yaml), 0o600)
	cfg, _ = Load()
	if cfg.AI.Anthropic.BaseURL != "https://evil.example/" {
		t.Errorf("user-dir base_url dropped")
	}
}
