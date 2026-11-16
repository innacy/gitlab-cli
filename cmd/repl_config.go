package cmd

import (
	"fmt"
	"strings"
	"time"

	"gitlab-ai/pkg/config"
	"gitlab-ai/pkg/output"
	"gitlab-ai/pkg/platform"
)

func (r *replState) handleConfig() {
	theme := output.GetTheme()

	output.ThemeHeader("Current Configuration")
	fmt.Println()

	theme.Primary.Print("  Platform:       ")
	p := r.cfg.Platform
	if p == "" {
		p = "gitlab"
	}
	fmt.Println(p)

	if len(r.cfg.Environments) > 0 {
		theme.Primary.Print("  Environment:    ")
		fmt.Printf("%s (available: %s)\n", orNone(r.cfg.Environment), strings.Join(r.cfg.EnvironmentNames(), ", "))
	}

	theme.Primary.Print("  GitLab URL:     ")
	fmt.Println(r.cfg.GitLab.BaseURL)

	theme.Primary.Print("  AI Provider:    ")
	ai := r.cfg.AI.Resolve()
	switch {
	case ai.Provider == "":
		theme.Warning.Printf("unknown provider %q\n", r.cfg.AI.Provider)
	case ai.APIKey == "":
		theme.Warning.Printf("%s — %s (no API key: set ai.%s.api_key or %s)\n", ai.Label, ai.Model, ai.Provider, ai.KeyEnv)
	default:
		fmt.Printf("%s — %s (key from %s)\n", ai.Label, ai.Model, ai.KeySource)
	}

	theme.Primary.Print("  Color Output:   ")
	fmt.Println(boolStr(r.cfg.CLI.ColorOutput))

	theme.Primary.Print("  Output Format:  ")
	f := r.cfg.CLI.OutputFormat
	if f == "" {
		f = "text"
	}
	fmt.Println(f)

	theme.Primary.Print("  Theme:          ")
	t := r.cfg.CLI.Theme
	if t == "" {
		t = "default"
	}
	fmt.Println(t)

	theme.Primary.Print("  Confirm Posts:  ")
	fmt.Println(boolStr(r.cfg.CLI.ConfirmBeforePost))

	theme.Primary.Print("  Idle Timeout:   ")
	fmt.Printf("%d minutes\n", r.cfg.CLI.IdleTimeoutMinutes)

	if len(r.cfg.Teams) > 0 {
		theme.Primary.Print("  Teams:          ")
		fmt.Println(strings.Join(r.cfg.TeamNames(), ", "))
	}

	if r.activeTeam != "" {
		theme.Primary.Print("  Active Team:    ")
		fmt.Println(r.activeTeam)
		pf := r.cfg.ActiveParentFolder(r.activeTeam)
		if pf != "" {
			theme.Primary.Print("  Parent Folder:  ")
			fmt.Println(pf)
		}
	}

	fmt.Println()

	supported := platform.SupportedPlatforms()
	theme.Muted.Printf("  Supported platforms: %s\n", strings.Join(supported, ", "))
	cfgFile := r.cfg.ConfigFile
	if cfgFile == "" {
		cfgFile = "(none found — defaults only)"
	}
	theme.Muted.Printf("  Config file:         %s (+ GITLAB_AI_* env overrides)\n", cfgFile)

	if r.provider != nil {
		theme.Muted.Printf("  Active session:      %s (@%s)\n", r.provider.Name(), r.provider.Username())
	}
	fmt.Println()
}

func (r *replState) showHelp() {
	theme := output.GetTheme()

	fmt.Println()
	theme.Header.Println("  git-agent — Command Reference")
	fmt.Println()

	for _, section := range sectionOrder {
		theme.Primary.Printf("  %s\n", section)
		for _, c := range commandTable {
			if c.section == section {
				fmt.Printf("    %-40s %s\n", c.synopsis(), c.desc)
			}
		}
		fmt.Println()
	}

	theme.Muted.Println("  Any unrecognized input is sent to AI as a question.")
	theme.Muted.Println("  Tab = auto-complete | Up/Down = history | Ctrl+R = search history")
	fmt.Println()
}

func boolStr(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

// ─── Switch Team ─────────────────────────────────────────────────────────────

func (r *replState) handleSwitchTeam() {
	if !r.ensureSession() {
		return
	}

	names := r.cfg.TeamNames()
	if len(names) == 0 {
		output.PrintWarning("No teams configured. Add teams to config.yaml.")
		return
	}
	if len(names) == 1 {
		output.PrintWarning(fmt.Sprintf("Only one team configured: %s", names[0]))
		return
	}

	options := make([]string, 0, len(names))
	for _, n := range names {
		label := n
		if n == r.activeTeam {
			label = n + "  ← current"
		}
		options = append(options, label)
	}

	choice := r.promptForChoice("Switch to team", options)
	if choice < 0 || choice >= len(names) {
		return
	}

	target := names[choice]
	if target == r.activeTeam {
		output.PrintWarning(fmt.Sprintf("Already on team '%s'", target))
		return
	}

	r.switchTeam(target)
}

func (r *replState) switchTeam(name string) {
	prev := r.activeTeam
	r.resetCache(name)
	r.startCacheLoad()

	r.rl.SetPrompt(r.prompt())
	output.PrintSuccess(fmt.Sprintf("Switched team: %s → %s", prev, name))

	pf := r.cfg.ActiveParentFolder(name)
	if pf != "" {
		output.PrintSuccess(fmt.Sprintf("Parent folder: %s", pf))
	}
	fmt.Println()
}

// ─── Switch Environment ──────────────────────────────────────────────────────

func (r *replState) handleSwitchEnv(args []string) {
	names := r.cfg.EnvironmentNames()
	if len(names) == 0 {
		output.PrintWarning("No environments configured. Add an 'environments:' section to config.yaml.")
		return
	}

	target := ""
	if len(args) > 0 {
		target = strings.ToLower(args[0])
	} else {
		options := make([]string, len(names))
		for i, n := range names {
			options[i] = n
			if n == r.cfg.Environment {
				options[i] += "  ← current"
			}
		}
		choice := r.promptForChoice("Switch to environment", options)
		if choice < 0 {
			return
		}
		target = names[choice]
	}
	if target == r.cfg.Environment {
		output.PrintWarning(fmt.Sprintf("Already on environment '%s'", target))
		return
	}

	cfg, err := config.LoadWith(r.cfg.ConfigFile, target)
	if err != nil {
		output.PrintError(err.Error())
		return
	}
	if res := config.Validate(cfg); len(res.Errors) > 0 {
		output.PrintError(fmt.Sprintf("Environment '%s' is invalid: %s", target, strings.Join(res.Errors, "; ")))
		return
	}

	prev := r.cfg.Environment
	r.cfg = cfg
	// Everything derived from the old environment is stale. The provider is
	// replaced by connect, never set to nil, because a background cache load
	// from the old session may still be running.
	r.aiClient = nil
	r.resetCache("")

	output.PrintSuccess(fmt.Sprintf("Switched environment: %s → %s", orNone(prev), target))
	r.connect()
	r.rl.SetPrompt(r.prompt())
}

// resetCache sets the active team, empties the project cache and invalidates
// in-flight loads. activeTeam is written under cacheMu because background
// loads read it (filterByTeam) while holding that lock.
func (r *replState) resetCache(team string) {
	r.cacheMu.Lock()
	r.activeTeam = team
	r.projectCache = nil
	r.lastRefreshTime = time.Time{}
	r.cacheGen++
	r.cacheMu.Unlock()
}

func orNone(s string) string {
	if s == "" {
		return "(base)"
	}
	return s
}
