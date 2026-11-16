package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFindsConfigInUserDir(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Chdir(t.TempDir()) // no config.yaml in cwd
	dir := filepath.Join(xdg, "git-agent")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("gitlab:\n  base_url: \"https://home.example.com/\"\n"), 0o600)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitLab.BaseURL != "https://home.example.com/" {
		t.Errorf("base_url = %q, config in %s was not used", cfg.GitLab.BaseURL, dir)
	}
	if cfg.ConfigFile != filepath.Join(dir, "config.yaml") {
		t.Errorf("ConfigFile = %q", cfg.ConfigFile)
	}
	// Relative output dirs follow the config file, not the cwd.
	if want := filepath.Join(dir, ".data"); cfg.Review.Output.Directory != want {
		t.Errorf("review dir = %q, want %q", cfg.Review.Output.Directory, want)
	}
}

func TestLoadPrefersCwdConfig(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	os.MkdirAll(filepath.Join(xdg, "git-agent"), 0o700)
	os.WriteFile(filepath.Join(xdg, "git-agent", "config.yaml"), []byte("gitlab:\n  base_url: \"https://home/\"\n"), 0o600)
	t.Chdir(t.TempDir())
	abs := filepath.Join(t.TempDir(), "reviews")
	os.WriteFile("config.yaml", []byte("gitlab:\n  base_url: \"https://cwd/\"\nreview:\n  output:\n    directory: "+abs+"\n"), 0o600)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitLab.BaseURL != "https://cwd/" {
		t.Errorf("base_url = %q, want the cwd config", cfg.GitLab.BaseURL)
	}
	if cfg.Review.Output.Directory != abs {
		t.Errorf("absolute dir rewritten to %q", cfg.Review.Output.Directory)
	}
}

func TestAuditDirMigratesLegacyDB(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Chdir(t.TempDir())
	os.MkdirAll(".git-agent/audit", 0o755)
	os.WriteFile(".git-agent/audit/audit.db", []byte("db"), 0o600)

	dir, notice := AuditDir()
	want := filepath.Join(xdg, "git-agent", "audit")
	if dir != want || notice == "" {
		t.Fatalf("AuditDir() = %q, %q; want %q with a notice", dir, notice, want)
	}
	if b, _ := os.ReadFile(filepath.Join(want, "audit.db")); string(b) != "db" {
		t.Error("legacy DB was not moved")
	}
	if fi, _ := os.Stat(filepath.Join(want, "audit.db")); fi.Mode().Perm() != 0o600 {
		t.Errorf("migrated DB mode = %o, want 600", fi.Mode().Perm())
	}

	// Second call: nothing left to migrate.
	if dir, notice = AuditDir(); dir != want || notice != "" {
		t.Errorf("second AuditDir() = %q, %q", dir, notice)
	}
}

func TestAuditDirKeepsLegacyWhenTargetExists(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Chdir(t.TempDir())
	os.MkdirAll(".git-agent/audit", 0o755)
	os.WriteFile(".git-agent/audit/audit.db", []byte("old"), 0o600)
	os.MkdirAll(filepath.Join(xdg, "git-agent", "audit"), 0o700)
	os.WriteFile(filepath.Join(xdg, "git-agent", "audit", "audit.db"), []byte("new"), 0o600)

	dir, _ := AuditDir()
	if dir != filepath.Join(xdg, "git-agent", "audit") {
		t.Errorf("dir = %q", dir)
	}
	if b, _ := os.ReadFile(".git-agent/audit/audit.db"); string(b) != "old" {
		t.Error("legacy DB must not be touched when a newer one exists")
	}
}

func TestAuditDirRefusesUntrustedLegacyDB(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Chdir(t.TempDir())
	os.MkdirAll(".git-agent/audit", 0o755)
	target := filepath.Join(t.TempDir(), "elsewhere.db")
	os.WriteFile(target, []byte("x"), 0o600)

	os.Symlink(target, ".git-agent/audit/audit.db")
	if _, notice := AuditDir(); notice != "" {
		t.Error("migrated a symlink")
	}

	os.Remove(".git-agent/audit/audit.db")
	os.WriteFile(".git-agent/audit/audit.db", []byte("x"), 0o600)
	os.Chmod(".git-agent/audit/audit.db", 0o666)
	if _, notice := AuditDir(); notice != "" {
		t.Error("migrated a world-writable file")
	}
}
