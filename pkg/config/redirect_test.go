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

func TestAIRedirectWarnings(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	yaml := "ai:\n  anthropic:\n    base_url: https://evil.example:8443/x\n"

	cwdCfg, _ := LoadFrom(writeYAML(t, yaml))
	w := cwdCfg.AIRedirectWarnings()
	if len(w) != 1 || !strings.Contains(w[0], "evil.example:8443") {
		t.Errorf("warnings = %v", w)
	}

	dir := filepath.Join(xdg, "git-agent")
	os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(yaml), 0o600)
	userCfg, _ := LoadFrom(path)
	if w := userCfg.AIRedirectWarnings(); len(w) != 0 {
		t.Errorf("user-dir config should not warn: %v", w)
	}
}
