package config

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// AIRedirectWarnings flags AI base_url overrides that come from a config file
// outside the user's config dir. A config.yaml in the working directory may
// belong to a repository someone else controls, and base_url decides where
// the API key is sent.
func (c *AppConfig) AIRedirectWarnings() []string {
	if c.ConfigFile == "" || strings.HasPrefix(c.ConfigFile, Dir()+string(filepath.Separator)) {
		return nil
	}
	var out []string
	for name, raw := range map[string]string{
		"anthropic": c.AI.Anthropic.BaseURL, "gemini": c.AI.Gemini.BaseURL, "nvidia": c.AI.Nvidia.BaseURL,
	} {
		if raw == "" {
			continue
		}
		host := raw
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			host = u.Host
		}
		out = append(out, fmt.Sprintf("ai.%s.base_url in %s sends your %s API key to %s", name, c.ConfigFile, name, host))
	}
	return out
}
