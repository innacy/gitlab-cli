package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"

	"gitlab-ai/internal/models"
)

// AppConfig holds the complete application configuration.
type AppConfig struct {
	Platform        string                `mapstructure:"platform" yaml:"platform"`
	GitLab          GitLabConfig          `mapstructure:"gitlab" yaml:"gitlab"`
	AI              AIConfig              `mapstructure:"ai" yaml:"ai"`
	Review          ReviewConfig          `mapstructure:"review" yaml:"review"`
	Issues          IssuesConfig          `mapstructure:"issues" yaml:"issues"`
	Merge           MergeConfig           `mapstructure:"merge" yaml:"merge"`
	CLI             CLIConfig             `mapstructure:"cli" yaml:"cli"`
	Other           OtherOutputConfig     `mapstructure:"other" yaml:"other"`
	TicketContent   ContentTemplateConfig `mapstructure:"ticket_content" yaml:"ticket_content"`
	EpicContent     ContentTemplateConfig `mapstructure:"epic_content" yaml:"epic_content"`
	Teams           []TeamConfig          `mapstructure:"teams" yaml:"teams"`
	Projects        []string              `mapstructure:"projects" yaml:"projects"`
	IgnoredProjects []string              `mapstructure:"ignored_projects" yaml:"ignored_projects"`

	// Environments are named overlays (any subset of the keys above) merged
	// over the base config; DefaultEnvironment picks one when --env is unset.
	Environments       map[string]interface{} `mapstructure:"environments" yaml:"environments"`
	DefaultEnvironment string                 `mapstructure:"default_environment" yaml:"default_environment"`

	// ConfigFile is the config file that was loaded ("" when none was found).
	ConfigFile string `mapstructure:"-" yaml:"-"`
	// Environment is the active environment ("" when none).
	Environment string `mapstructure:"-" yaml:"-"`
}

// EnvironmentNames returns the configured environment names, sorted.
func (c *AppConfig) EnvironmentNames() []string { return sortedKeys(c.Environments) }

// GitLabConfig holds GitLab connection settings.
type GitLabConfig struct {
	BaseURL        string `mapstructure:"base_url" yaml:"base_url"`
	APIVersion     string `mapstructure:"api_version" yaml:"api_version"`
	Token          string `mapstructure:"token" yaml:"token"`
	TokenEnv       string `mapstructure:"token_env" yaml:"token_env"`
	DefaultProject string `mapstructure:"default_project" yaml:"default_project"`
	ParentFolder   string `mapstructure:"parent_folder" yaml:"parent_folder"`
}

// AIConfig holds AI provider settings.
type AIConfig struct {
	Provider       string          `mapstructure:"provider" yaml:"provider"`
	TimeoutSeconds int             `mapstructure:"timeout_seconds" yaml:"timeout_seconds"`
	Anthropic      AnthropicConfig `mapstructure:"anthropic" yaml:"anthropic"`
	Gemini         GeminiConfig    `mapstructure:"gemini" yaml:"gemini"`
	Nvidia         NvidiaConfig    `mapstructure:"nvidia" yaml:"nvidia"`
}

// NvidiaConfig holds NVIDIA NIM-specific settings.
type NvidiaConfig struct {
	APIKey          string  `mapstructure:"api_key" yaml:"api_key"`
	APIKeyEnv       string  `mapstructure:"api_key_env" yaml:"api_key_env"`
	Model           string  `mapstructure:"model" yaml:"model"`
	FallbackModel   string  `mapstructure:"fallback_model" yaml:"fallback_model"`
	MaxTokens       int     `mapstructure:"max_tokens" yaml:"max_tokens"`
	Temperature     float64 `mapstructure:"temperature" yaml:"temperature"`
	TopP            float64 `mapstructure:"top_p" yaml:"top_p"`
	EnableThinking  bool    `mapstructure:"enable_thinking" yaml:"enable_thinking"`
	ReasoningBudget int     `mapstructure:"reasoning_budget" yaml:"reasoning_budget"`
}

// GeminiConfig holds Google Gemini-specific settings.
type GeminiConfig struct {
	APIKey        string `mapstructure:"api_key" yaml:"api_key"`
	APIKeyEnv     string `mapstructure:"api_key_env" yaml:"api_key_env"`
	Model         string `mapstructure:"model" yaml:"model"`
	FallbackModel string `mapstructure:"fallback_model" yaml:"fallback_model"`
	MaxTokens     int    `mapstructure:"max_tokens" yaml:"max_tokens"`
}

// AnthropicConfig holds Anthropic-specific settings.
type AnthropicConfig struct {
	APIKey        string  `mapstructure:"api_key" yaml:"api_key"`
	APIKeyEnv     string  `mapstructure:"api_key_env" yaml:"api_key_env"`
	Model         string  `mapstructure:"model" yaml:"model"`
	FallbackModel string  `mapstructure:"fallback_model" yaml:"fallback_model"`
	MaxTokens     int     `mapstructure:"max_tokens" yaml:"max_tokens"`
	Temperature   float64 `mapstructure:"temperature" yaml:"temperature"`
}

// ReviewConfig holds review-related settings.
type ReviewConfig struct {
	Template ReviewTemplateConfig `mapstructure:"template" yaml:"template"`
	Output   ReviewOutputConfig   `mapstructure:"output" yaml:"output"`
	Filters  ReviewFiltersConfig  `mapstructure:"filters" yaml:"filters"`
}

// ReviewTemplateConfig holds the review template sections.
type ReviewTemplateConfig struct {
	Sections []models.ReviewTemplateSection `mapstructure:"sections" yaml:"sections"`
}

// ReviewOutputConfig holds review output settings.
type ReviewOutputConfig struct {
	Directory       string `mapstructure:"directory" yaml:"directory"`
	FilenamePattern string `mapstructure:"filename_pattern" yaml:"filename_pattern"`
	IncludeMetadata bool   `mapstructure:"include_metadata" yaml:"include_metadata"`
	IncludeDiff     bool   `mapstructure:"include_diff" yaml:"include_diff"`
}

// ReviewFiltersConfig holds review filter settings.
type ReviewFiltersConfig struct {
	ExcludeFiles []string `mapstructure:"exclude_files" yaml:"exclude_files"`
}

// IssuesConfig holds issue-related settings.
type IssuesConfig struct {
	Output IssuesOutputConfig `mapstructure:"output" yaml:"output"`
	Fields []string           `mapstructure:"fields" yaml:"fields"`
}

// IssuesOutputConfig holds issue output settings.
type IssuesOutputConfig struct {
	Directory       string `mapstructure:"directory" yaml:"directory"`
	FilenamePattern string `mapstructure:"filename_pattern" yaml:"filename_pattern"`
}

// OtherOutputConfig holds output settings for non-review, non-issue markdown files.
type OtherOutputConfig struct {
	Directory string `mapstructure:"directory" yaml:"directory"`
}

// ContentTemplateConfig holds a markdown template for AI-generated content.
type ContentTemplateConfig struct {
	Template string `mapstructure:"template" yaml:"template"`
}

// MergeConfig holds default merge behavior settings.
type MergeConfig struct {
	Squash             bool `mapstructure:"squash" yaml:"squash"`
	RemoveSourceBranch bool `mapstructure:"remove_source_branch" yaml:"remove_source_branch"`
}

// CLIConfig holds CLI behavior settings.
type CLIConfig struct {
	ColorOutput        bool   `mapstructure:"color_output" yaml:"color_output"`
	MarkdownRendering  bool   `mapstructure:"markdown_rendering" yaml:"markdown_rendering"`
	Verbose            bool   `mapstructure:"verbose" yaml:"verbose"`
	ConfirmBeforePost  bool   `mapstructure:"confirm_before_post" yaml:"confirm_before_post"`
	NonInteractive     bool   `mapstructure:"non_interactive" yaml:"non_interactive"`
	AutoConfirm        bool   `mapstructure:"auto_confirm" yaml:"auto_confirm"`
	IdleTimeoutMinutes int    `mapstructure:"idle_timeout_minutes" yaml:"idle_timeout_minutes"`
	OutputFormat       string `mapstructure:"output_format" yaml:"output_format"`
	Theme              string `mapstructure:"theme" yaml:"theme"`
	OpenInBrowser      bool   `mapstructure:"open_in_browser" yaml:"open_in_browser"`
}

// TeamConfig holds per-team settings.
type TeamConfig struct {
	Name            string   `mapstructure:"name" yaml:"name"`
	ParentFolder    string   `mapstructure:"parent_folder" yaml:"parent_folder"`
	Projects        []string `mapstructure:"projects" yaml:"projects"`
	IgnoredProjects []string `mapstructure:"ignored_projects" yaml:"ignored_projects"`
}

// TeamNames returns the list of team name strings.
func (c *AppConfig) TeamNames() []string {
	names := make([]string, len(c.Teams))
	for i, t := range c.Teams {
		names[i] = t.Name
	}
	return names
}

// FindTeam returns the TeamConfig for the given name, or nil if not found.
func (c *AppConfig) FindTeam(name string) *TeamConfig {
	for i := range c.Teams {
		if c.Teams[i].Name == name {
			return &c.Teams[i]
		}
	}
	return nil
}

// ActiveParentFolder returns the parent folder for the given team.
// Falls back to the global gitlab.parent_folder if the team has none set.
func (c *AppConfig) ActiveParentFolder(teamName string) string {
	if tc := c.FindTeam(teamName); tc != nil && tc.ParentFolder != "" {
		return tc.ParentFolder
	}
	return c.GitLab.ParentFolder
}

// ActiveProjects returns the issue/ticket projects for the given team.
// Falls back to the global projects list if the team has none set.
func (c *AppConfig) ActiveProjects(teamName string) []string {
	if tc := c.FindTeam(teamName); tc != nil && len(tc.Projects) > 0 {
		return tc.Projects
	}
	return c.Projects
}

// ActiveIgnoredProjects returns the ignored projects for the given team.
// Falls back to the global ignored_projects list if the team has none set.
func (c *AppConfig) ActiveIgnoredProjects(teamName string) []string {
	if tc := c.FindTeam(teamName); tc != nil && len(tc.IgnoredProjects) > 0 {
		return tc.IgnoredProjects
	}
	return c.IgnoredProjects
}

// Load reads configuration from YAML file and environment variables.
func Load() (*AppConfig, error) { return LoadWith("", "") }

// LoadFrom reads configuration from a specific YAML file path.
func LoadFrom(path string) (*AppConfig, error) { return LoadWith(path, "") }

// LoadWith loads the config at path (searched for when "") with the named
// environment applied ("" means default_environment, if set).
func LoadWith(path, env string) (*AppConfig, error) {
	v := viper.New()
	if path != "" {
		v.SetConfigFile(path)
	}
	return loadWithViper(v, path, env)
}

func loadWithViper(v *viper.Viper, path, env string) (*AppConfig, error) {
	v.SetConfigType("yaml")

	if path == "" {
		v.SetConfigName("config")
		v.AddConfigPath(".")
		v.AddConfigPath("./configs")
		v.AddConfigPath(Dir())
	}

	setDefaultsOn(v)

	v.SetEnvPrefix("GITLAB_AI")
	// Nested keys map to env vars with "_": gitlab.base_url → GITLAB_AI_GITLAB_BASE_URL.
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	if env == "" {
		env = v.GetString("default_environment")
	}
	if env != "" {
		env = strings.ToLower(env) // viper lowercases map keys
		overlay, ok := v.Get("environments." + env).(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unknown environment %q (configured: %s)", env,
				strings.Join(sortedKeys(v.GetStringMap("environments")), ", "))
		}
		if err := v.MergeConfigMap(overlay); err != nil {
			return nil, fmt.Errorf("apply environment %q: %w", env, err)
		}
	}

	cfg := &AppConfig{}
	if err := v.Unmarshal(cfg, viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			stringToTeamConfigHookFunc(),
		),
	)); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	cfg.normalizeTeams(v)
	cfg.ConfigFile = v.ConfigFileUsed()
	cfg.Environment = env
	cfg.resolveOutputDirs()

	if err := cfg.EnsureOutputDirs(); err != nil {
		return nil, fmt.Errorf("failed to create output directories: %w", err)
	}

	return cfg, nil
}

// stringToTeamConfigHookFunc allows Viper to unmarshal plain strings like
// teams: ["vault"] into []TeamConfig by treating the string as the Name field.
func stringToTeamConfigHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		if t != reflect.TypeOf(TeamConfig{}) {
			return data, nil
		}
		return map[string]interface{}{"name": data.(string)}, nil
	}
}

// normalizeTeams handles backward compat: if the YAML had plain strings
// (teams: ["vault"]) Viper will fail to unmarshal them into []TeamConfig.
// We detect that and fall back to reading the raw slice.
func (c *AppConfig) normalizeTeams(v *viper.Viper) {
	if len(c.Teams) > 0 && c.Teams[0].Name != "" {
		return
	}

	raw := v.Get("teams")
	if raw == nil {
		return
	}
	items, ok := raw.([]interface{})
	if !ok {
		return
	}

	teams := make([]TeamConfig, 0, len(items))
	for _, item := range items {
		switch val := item.(type) {
		case string:
			teams = append(teams, TeamConfig{Name: val})
		case map[string]interface{}:
			tc := TeamConfig{}
			if n, ok := val["name"].(string); ok {
				tc.Name = n
			}
			if pf, ok := val["parent_folder"].(string); ok {
				tc.ParentFolder = pf
			}
			if projs, ok := val["projects"].([]interface{}); ok {
				for _, p := range projs {
					if s, ok := p.(string); ok {
						tc.Projects = append(tc.Projects, s)
					}
				}
			}
			if ign, ok := val["ignored_projects"].([]interface{}); ok {
				for _, p := range ign {
					if s, ok := p.(string); ok {
						tc.IgnoredProjects = append(tc.IgnoredProjects, s)
					}
				}
			}
			if tc.Name != "" {
				teams = append(teams, tc)
			}
		}
	}
	c.Teams = teams
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// resolveOutputDirs makes relative output directories relative to the config
// file, so outputs land in the same place whatever directory git-agent is run
// from. Without a config file they stay relative to the working directory.
func (c *AppConfig) resolveOutputDirs() {
	if c.ConfigFile == "" {
		return
	}
	base := filepath.Dir(c.ConfigFile)
	for _, d := range []*string{&c.Review.Output.Directory, &c.Issues.Output.Directory, &c.Other.Directory} {
		if *d != "" && !filepath.IsAbs(*d) {
			*d = filepath.Join(base, *d)
		}
	}
}

// EnsureOutputDirs creates all configured output directories if they don't exist.
func (c *AppConfig) EnsureOutputDirs() error {
	dirs := []string{
		c.Review.Output.Directory,
		c.Issues.Output.Directory,
		c.Other.Directory,
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("cannot create directory %q: %w", dir, err)
		}
	}
	return nil
}
