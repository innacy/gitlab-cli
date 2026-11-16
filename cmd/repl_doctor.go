package cmd

import (
	"fmt"

	"gitlab-ai/pkg/config"
	gl "gitlab-ai/pkg/gitlab"
	"gitlab-ai/pkg/output"
	"gitlab-ai/pkg/platform"
)

// handleDoctor shows where each important setting comes from and checks that
// GitLab is reachable with the configured token.
func (r *replState) handleDoctor() {
	output.ThemeHeader("Doctor")
	fmt.Println()
	cfg := r.cfg

	if cfg.ConfigFile == "" {
		output.PrintWarning("Config file: none found (defaults and GITLAB_AI_* env only)")
	} else {
		output.PrintSuccess("Config file: " + cfg.ConfigFile)
	}
	if w := cfg.SecretPermissionWarning(); w != "" {
		output.PrintWarning(w)
	}
	if cfg.Environment != "" {
		output.PrintSuccess("Environment: " + cfg.Environment)
	}
	output.PrintSuccess("Data dir:    " + config.Dir())

	res := config.Validate(cfg)
	for _, e := range res.Errors {
		output.PrintError("Config: " + e)
	}
	for _, w := range res.Warnings {
		output.PrintWarning("Config: " + w)
	}

	output.PrintSuccess("GitLab URL:  " + cfg.GitLab.BaseURL)
	if token, src := gl.TokenSource(&cfg.GitLab); token == "" {
		output.PrintError(fmt.Sprintf("GitLab token: not found (set %s, gitlab.token, or ~/.netrc)", cfg.GitLab.TokenEnv))
	} else {
		output.PrintSuccess(fmt.Sprintf("GitLab token: %s (from %s)", mask(token), src))
		if prov, err := platform.NewProvider(cfg); err != nil {
			output.PrintError(fmt.Sprintf("GitLab: %v", err))
		} else {
			output.PrintSuccess(fmt.Sprintf("GitLab: reachable, authenticated as @%s", prov.Username()))
		}
	}

	ai := cfg.AI.Resolve()
	switch {
	case ai.Provider == "":
		output.PrintError(fmt.Sprintf("AI provider %q is not supported", cfg.AI.Provider))
	case ai.APIKey == "":
		output.PrintError(fmt.Sprintf("AI: %s — no API key (set ai.%s.api_key or %s)", ai.Label, ai.Provider, ai.KeyEnv))
	default:
		output.PrintSuccess(fmt.Sprintf("AI: %s — %s, key %s (from %s)", ai.Label, ai.Model, mask(ai.APIKey), ai.KeySource))
	}
	fmt.Println()
}

// mask hides a secret, keeping only its last 4 characters.
func mask(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return "****" + s[len(s)-4:]
}
