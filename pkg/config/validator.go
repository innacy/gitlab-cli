package config

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidationError holds a collection of validation errors.
type ValidationError struct {
	Errors []string
}

// Error implements the error interface.
func (v *ValidationError) Error() string {
	return fmt.Sprintf("configuration validation failed:\n  - %s", strings.Join(v.Errors, "\n  - "))
}

// HasErrors returns true if there are validation errors.
func (v *ValidationError) HasErrors() bool {
	return len(v.Errors) > 0
}

// ValidationResult holds both hard errors and soft warnings.
type ValidationResult struct {
	Errors   []string
	Warnings []string
}

// Validate checks the configuration for hard errors (fatal) and warnings (informational).
func Validate(cfg *AppConfig) *ValidationResult {
	result := &ValidationResult{}

	validateGitLab(&cfg.GitLab, result)
	validateAI(&cfg.AI, result)
	validateReview(&cfg.Review, result)
	validateIssues(&cfg.Issues, result)

	if len(cfg.Teams) == 0 {
		result.Warnings = append(result.Warnings, "no teams configured — project filtering will be disabled")
	}

	return result
}

func validateGitLab(cfg *GitLabConfig, r *ValidationResult) {
	if cfg.BaseURL == "" || cfg.BaseURL == "https://gitlab.example.com/" {
		r.Errors = append(r.Errors, "gitlab.base_url is required — set it to your GitLab instance URL")
	} else {
		if _, err := url.ParseRequestURI(cfg.BaseURL); err != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("gitlab.base_url is not a valid URL: %s", cfg.BaseURL))
		}
	}

	if cfg.APIVersion != "" && cfg.APIVersion != "v4" {
		r.Errors = append(r.Errors, fmt.Sprintf("gitlab.api_version '%s' is not supported (use 'v4')", cfg.APIVersion))
	}
}

func validateAI(cfg *AIConfig, r *ValidationResult) {
	supported := map[string]bool{"anthropic": true, "claude": true, "gemini": true, "google": true, "nvidia": true, "": true}
	if !supported[strings.ToLower(cfg.Provider)] {
		r.Errors = append(r.Errors, fmt.Sprintf("ai.provider '%s' is not supported (use: anthropic, gemini, nvidia)", cfg.Provider))
	}
}

func validateReview(cfg *ReviewConfig, r *ValidationResult) {
	if len(cfg.Template.Sections) == 0 {
		r.Warnings = append(r.Warnings, "review.template.sections is empty — mr-review will not work until configured")
	}
	for i, section := range cfg.Template.Sections {
		if section.Name == "" {
			r.Warnings = append(r.Warnings, fmt.Sprintf("review.template.sections[%d].name is empty", i))
		}
		if section.Prompt == "" {
			r.Warnings = append(r.Warnings, fmt.Sprintf("review.template.sections[%d].prompt is empty", i))
		}
	}

	if cfg.Output.Directory == "" {
		r.Errors = append(r.Errors, "review.output.directory is required")
	}
	if cfg.Output.FilenamePattern == "" {
		r.Errors = append(r.Errors, "review.output.filename_pattern is required")
	}
}

func validateIssues(cfg *IssuesConfig, r *ValidationResult) {
	if cfg.Output.Directory == "" {
		r.Errors = append(r.Errors, "issues.output.directory is required")
	}
	if cfg.Output.FilenamePattern == "" {
		r.Errors = append(r.Errors, "issues.output.filename_pattern is required")
	}
}
