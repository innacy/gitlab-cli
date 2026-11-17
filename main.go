package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/fatih/color"

	"gitlab-ai/cmd"
	"gitlab-ai/pkg/config"
	"gitlab-ai/pkg/output"
	"gitlab-ai/pkg/utils"
)

func main() {
	configPath := flag.String("config", "", "path to config.yaml (default: ./config.yaml, ./configs/, then "+config.Dir()+")")
	env := flag.String("env", "", "environment profile from the 'environments:' config section")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage:\n  git-agent [flags]                     interactive REPL\n  git-agent [flags] <command> [args]    run one command and exit (status 1 on error)\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	cfg, err := config.LoadWith(*configPath, *env)
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

	if w := cfg.SecretPermissionWarning(); w != "" {
		fmt.Fprintf(os.Stderr, "⚠ %s\n", w)
	}
	for _, w := range cfg.AIRedirectWarnings() {
		fmt.Fprintf(os.Stderr, "⚠ %s\n", w)
	}

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

	if flag.NArg() > 0 {
		os.Exit(cmd.RunOnce(cfg, flag.Args()))
	}
	cmd.RunREPL(cfg)
}
