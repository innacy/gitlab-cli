package config

import (
	"fmt"
	"os"
)

// SecretPermissionWarning warns when the config file holds a secret (GitLab
// token or an AI API key) and can be read by group or others.
func (c *AppConfig) SecretPermissionWarning() string {
	if c.ConfigFile == "" || !c.hasFileSecret() {
		return ""
	}
	fi, err := os.Stat(c.ConfigFile)
	if err != nil || fi.Mode().Perm()&0o077 == 0 {
		return ""
	}
	return fmt.Sprintf("%s contains secrets but has permissions %o — run: chmod 600 %s",
		c.ConfigFile, fi.Mode().Perm(), c.ConfigFile)
}

func (c *AppConfig) hasFileSecret() bool {
	return c.GitLab.Token != "" || c.AI.Anthropic.APIKey != "" || c.AI.Gemini.APIKey != "" || c.AI.Nvidia.APIKey != ""
}
