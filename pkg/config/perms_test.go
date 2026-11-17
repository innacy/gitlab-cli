package config

import (
	"os"
	"strings"
	"testing"
)

func TestSecretPermissionWarning(t *testing.T) {
	path := writeYAML(t, "gitlab:\n  token: secret\n")
	os.Chmod(path, 0o644)
	cfg, _ := LoadFrom(path)
	if w := cfg.SecretPermissionWarning(); !strings.Contains(w, "chmod 600") {
		t.Errorf("want chmod warning, got %q", w)
	}
	os.Chmod(path, 0o600)
	if w := cfg.SecretPermissionWarning(); w != "" {
		t.Errorf("0600 file warned: %q", w)
	}
	plain := writeYAML(t, "gitlab:\n  base_url: \"https://x/\"\n")
	os.Chmod(plain, 0o644)
	cfg, _ = LoadFrom(plain)
	if w := cfg.SecretPermissionWarning(); w != "" {
		t.Errorf("secret-free file warned: %q", w)
	}
}
