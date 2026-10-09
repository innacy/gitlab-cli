package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTeamConfigParsing_MultiTeam(t *testing.T) {
	yaml := `
teams:
  - name: "vault"
    parent_folder: "/home/user/vault-repos"
  - name: "cnips"
    parent_folder: "/home/user/cnips-repos"
`
	cfg := loadFromYAML(t, yaml)

	if len(cfg.Teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(cfg.Teams))
	}
	if cfg.Teams[0].Name != "vault" {
		t.Errorf("expected team[0].Name = vault, got %q", cfg.Teams[0].Name)
	}
	if cfg.Teams[0].ParentFolder != "/home/user/vault-repos" {
		t.Errorf("expected team[0].ParentFolder = /home/user/vault-repos, got %q", cfg.Teams[0].ParentFolder)
	}
	if cfg.Teams[1].Name != "cnips" {
		t.Errorf("expected team[1].Name = cnips, got %q", cfg.Teams[1].Name)
	}
	if cfg.Teams[1].ParentFolder != "/home/user/cnips-repos" {
		t.Errorf("expected team[1].ParentFolder = /home/user/cnips-repos, got %q", cfg.Teams[1].ParentFolder)
	}
}

func TestTeamConfigParsing_FlatStringBackwardCompat(t *testing.T) {
	yaml := `
teams:
  - "vault"
  - "cnips"
`
	cfg := loadFromYAML(t, yaml)

	if len(cfg.Teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(cfg.Teams))
	}
	if cfg.Teams[0].Name != "vault" {
		t.Errorf("expected team[0].Name = vault, got %q", cfg.Teams[0].Name)
	}
	if cfg.Teams[1].Name != "cnips" {
		t.Errorf("expected team[1].Name = cnips, got %q", cfg.Teams[1].Name)
	}
}

func TestTeamNames(t *testing.T) {
	cfg := &AppConfig{
		Teams: []TeamConfig{
			{Name: "vault", ParentFolder: "/a"},
			{Name: "cnips", ParentFolder: "/b"},
		},
	}
	names := cfg.TeamNames()
	if len(names) != 2 || names[0] != "vault" || names[1] != "cnips" {
		t.Errorf("expected [vault cnips], got %v", names)
	}
}

func TestFindTeam(t *testing.T) {
	cfg := &AppConfig{
		Teams: []TeamConfig{
			{Name: "vault", ParentFolder: "/a"},
			{Name: "cnips", ParentFolder: "/b"},
		},
	}

	tc := cfg.FindTeam("cnips")
	if tc == nil {
		t.Fatal("expected to find team cnips")
	}
	if tc.ParentFolder != "/b" {
		t.Errorf("expected parent_folder /b, got %q", tc.ParentFolder)
	}

	if cfg.FindTeam("nonexistent") != nil {
		t.Error("expected nil for nonexistent team")
	}
}

func TestActiveParentFolder_TeamLevel(t *testing.T) {
	cfg := &AppConfig{
		GitLab: GitLabConfig{ParentFolder: "/global"},
		Teams: []TeamConfig{
			{Name: "vault", ParentFolder: "/team-specific"},
		},
	}
	folder := cfg.ActiveParentFolder("vault")
	if folder != "/team-specific" {
		t.Errorf("expected /team-specific, got %q", folder)
	}
}

func TestActiveParentFolder_FallbackToGlobal(t *testing.T) {
	cfg := &AppConfig{
		GitLab: GitLabConfig{ParentFolder: "/global"},
		Teams: []TeamConfig{
			{Name: "vault"},
		},
	}
	folder := cfg.ActiveParentFolder("vault")
	if folder != "/global" {
		t.Errorf("expected /global fallback, got %q", folder)
	}
}

func TestActiveParentFolder_UnknownTeam(t *testing.T) {
	cfg := &AppConfig{
		GitLab: GitLabConfig{ParentFolder: "/global"},
	}
	folder := cfg.ActiveParentFolder("unknown")
	if folder != "/global" {
		t.Errorf("expected /global fallback, got %q", folder)
	}
}

// loadFromYAML writes a temp config file and loads it.
func loadFromYAML(t *testing.T, yamlContent string) *AppConfig {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}
	return cfg
}

// ─── Team-scoped Projects & IgnoredProjects ──────────────────────────────────

func TestTeamConfigParsing_TeamProjects(t *testing.T) {
	yaml := `
teams:
  - name: "cnips"
    parent_folder: "/home/user/cnips"
    projects:
      - "cnips/issues"
    ignored_projects:
      - "cnips/testcase"
  - name: "vault"
    parent_folder: "/home/user/vault"
    projects:
      - "vault/tracker"
`
	cfg := loadFromYAML(t, yaml)

	cnips := cfg.FindTeam("cnips")
	if cnips == nil {
		t.Fatal("expected cnips team")
	}
	if len(cnips.Projects) != 1 || cnips.Projects[0] != "cnips/issues" {
		t.Errorf("cnips.Projects = %v, want [cnips/issues]", cnips.Projects)
	}
	if len(cnips.IgnoredProjects) != 1 || cnips.IgnoredProjects[0] != "cnips/testcase" {
		t.Errorf("cnips.IgnoredProjects = %v, want [cnips/testcase]", cnips.IgnoredProjects)
	}

	vault := cfg.FindTeam("vault")
	if vault == nil {
		t.Fatal("expected vault team")
	}
	if len(vault.Projects) != 1 || vault.Projects[0] != "vault/tracker" {
		t.Errorf("vault.Projects = %v, want [vault/tracker]", vault.Projects)
	}
	if len(vault.IgnoredProjects) != 0 {
		t.Errorf("vault.IgnoredProjects = %v, want empty", vault.IgnoredProjects)
	}
}

func TestActiveProjects_TeamLevel(t *testing.T) {
	cfg := &AppConfig{
		Projects: []string{"global/proj"},
		Teams: []TeamConfig{
			{Name: "vault", Projects: []string{"vault/tracker"}},
		},
	}
	got := cfg.ActiveProjects("vault")
	if len(got) != 1 || got[0] != "vault/tracker" {
		t.Errorf("expected [vault/tracker], got %v", got)
	}
}

func TestActiveProjects_FallbackToGlobal(t *testing.T) {
	cfg := &AppConfig{
		Projects: []string{"global/proj"},
		Teams: []TeamConfig{
			{Name: "vault"},
		},
	}
	got := cfg.ActiveProjects("vault")
	if len(got) != 1 || got[0] != "global/proj" {
		t.Errorf("expected [global/proj], got %v", got)
	}
}

func TestActiveProjects_Empty(t *testing.T) {
	cfg := &AppConfig{
		Teams: []TeamConfig{{Name: "vault"}},
	}
	got := cfg.ActiveProjects("vault")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestActiveIgnoredProjects_TeamLevel(t *testing.T) {
	cfg := &AppConfig{
		IgnoredProjects: []string{"global/ignored"},
		Teams: []TeamConfig{
			{Name: "cnips", IgnoredProjects: []string{"cnips/testcase"}},
		},
	}
	got := cfg.ActiveIgnoredProjects("cnips")
	if len(got) != 1 || got[0] != "cnips/testcase" {
		t.Errorf("expected [cnips/testcase], got %v", got)
	}
}

func TestActiveIgnoredProjects_FallbackToGlobal(t *testing.T) {
	cfg := &AppConfig{
		IgnoredProjects: []string{"global/ignored"},
		Teams: []TeamConfig{
			{Name: "vault"},
		},
	}
	got := cfg.ActiveIgnoredProjects("vault")
	if len(got) != 1 || got[0] != "global/ignored" {
		t.Errorf("expected [global/ignored], got %v", got)
	}
}
