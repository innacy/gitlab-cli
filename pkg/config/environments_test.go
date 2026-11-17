package config

import (
	"strings"
	"testing"
)

const envYAML = `
gitlab:
  base_url: "https://base.example.com/"
  api_version: v4
ai:
  provider: anthropic
teams:
  - base-team
default_environment: work
environments:
  work:
    gitlab:
      base_url: "https://work.example.com/"
      token_env: WORK_TOKEN
    teams:
      - name: cnips
        parent_folder: /src/cnips
  Personal:
    ai:
      provider: gemini
`

func TestEnvironmentOverlay(t *testing.T) {
	path := writeYAML(t, envYAML)

	cfg, err := LoadWith(path, "") // default_environment applies
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment != "work" {
		t.Errorf("Environment = %q, want work", cfg.Environment)
	}
	if cfg.GitLab.BaseURL != "https://work.example.com/" || cfg.GitLab.TokenEnv != "WORK_TOKEN" {
		t.Errorf("gitlab not overlaid: %+v", cfg.GitLab)
	}
	if cfg.GitLab.APIVersion != "v4" {
		t.Errorf("unrelated base key lost in deep merge: api_version=%q", cfg.GitLab.APIVersion)
	}
	if len(cfg.Teams) != 1 || cfg.Teams[0].Name != "cnips" || cfg.Teams[0].ParentFolder != "/src/cnips" {
		t.Errorf("teams = %+v, want the environment's list", cfg.Teams)
	}
	if got := strings.Join(cfg.EnvironmentNames(), ","); got != "personal,work" {
		t.Errorf("EnvironmentNames = %s", got)
	}

	cfg, err = LoadWith(path, "PERSONAL") // case-insensitive, overrides default
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI.Provider != "gemini" || cfg.GitLab.BaseURL != "https://base.example.com/" {
		t.Errorf("personal: provider=%q base_url=%q", cfg.AI.Provider, cfg.GitLab.BaseURL)
	}
	if len(cfg.Teams) != 1 || cfg.Teams[0].Name != "base-team" {
		t.Errorf("personal should keep base teams, got %+v", cfg.Teams)
	}
}

func TestUnknownEnvironment(t *testing.T) {
	_, err := LoadWith(writeYAML(t, envYAML), "staging")
	if err == nil || !strings.Contains(err.Error(), "personal, work") {
		t.Errorf("err = %v, want unknown environment listing the configured ones", err)
	}
}

func TestEnvVarSelectsDefaultEnvironment(t *testing.T) {
	t.Setenv("GITLAB_AI_DEFAULT_ENVIRONMENT", "personal")
	cfg, err := LoadWith(writeYAML(t, envYAML), "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment != "personal" {
		t.Errorf("Environment = %q, want personal", cfg.Environment)
	}
}

func TestNoEnvironments(t *testing.T) {
	cfg := loadFromYAML(t, "gitlab:\n  base_url: \"https://x/\"\n")
	if cfg.Environment != "" || len(cfg.EnvironmentNames()) != 0 {
		t.Errorf("unexpected environment state: %q %v", cfg.Environment, cfg.EnvironmentNames())
	}
}
