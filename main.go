package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"

	"gitlab-ai/cmd"
	"gitlab-ai/pkg/config"
	"gitlab-ai/pkg/output"
	"gitlab-ai/pkg/utils"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if !cfg.CLI.ColorOutput || os.Getenv("NO_COLOR") != "" {
		color.NoColor = true
	}

	if cfg.CLI.OutputFormat != "" {
		output.SetOutputFormat(cfg.CLI.OutputFormat)
	}

	utils.InitLogger(cfg.CLI.Verbose)

	result := config.Validate(cfg)
	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "⚠ config warning: %s\n", w)
	}
	if len(result.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "\n✗ Configuration errors:\n")
		for _, e := range result.Errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
		fmt.Fprintf(os.Stderr, "\nFix these in config.yaml or set via GITLAB_AI_* environment variables.\n")
		os.Exit(1)
	}

	cmd.RunREPL(cfg)
}
