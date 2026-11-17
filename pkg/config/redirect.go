package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// dropUntrustedAIBaseURLs fails closed on AI base_url overrides. base_url
// decides where the API key is sent, and the config search starts in the
// working directory, which may be a repository someone else controls. An
// override is honoured only when it comes from the environment, from a file
// passed explicitly with --config, or from a config inside the user's config
// dir; otherwise it is cleared and a warning recorded.
func (c *AppConfig) dropUntrustedAIBaseURLs(explicit bool) {
	if c.ConfigFile == "" || explicit || inUserDir(c.ConfigFile) {
		return
	}
	for name, u := range map[string]*string{
		"anthropic": &c.AI.Anthropic.BaseURL, "gemini": &c.AI.Gemini.BaseURL, "nvidia": &c.AI.Nvidia.BaseURL,
	} {
		envName := "GITLAB_AI_AI_" + strings.ToUpper(name) + "_BASE_URL"
		if *u == "" || os.Getenv(envName) != "" {
			continue
		}
		c.ignored = append(c.ignored, fmt.Sprintf(
			"ignoring ai.%s.base_url from %s: it would receive your API key. Put it in %s, pass --config, or set %s",
			name, c.ConfigFile, filepath.Join(Dir(), "config.yaml"), envName))
		*u = ""
	}
}

// inUserDir reports whether path is inside Dir(), after resolving symlinks.
func inUserDir(path string) bool {
	resolve := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		a, _ := filepath.Abs(p)
		return a
	}
	return strings.HasPrefix(resolve(path), resolve(Dir())+string(filepath.Separator))
}

// AIRedirectWarnings lists the base_url overrides that were ignored.
func (c *AppConfig) AIRedirectWarnings() []string { return c.ignored }
