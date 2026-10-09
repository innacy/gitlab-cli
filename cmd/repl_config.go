package cmd

import (
	"fmt"
	"strings"
	"time"

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

	theme.Primary.Print("  GitLab URL:     ")
	fmt.Println(r.cfg.GitLab.BaseURL)

	theme.Primary.Print("  AI Provider:    ")
	switch r.cfg.AI.Provider {
	case "anthropic", "claude", "":
		if r.cfg.AI.Anthropic.APIKey != "" {
			fmt.Printf("Anthropic (Claude) — %s\n", r.cfg.AI.Anthropic.Model)
		} else {
			theme.Warning.Println("anthropic (ANTHROPIC_API_KEY not set)")
		}
	case "gemini", "google":
		if r.cfg.AI.Gemini.APIKey != "" {
			fmt.Printf("Google Gemini — %s\n", r.cfg.AI.Gemini.Model)
		} else {
			theme.Warning.Println("gemini (GEMINI_API_KEY not set)")
		}
	default:
		theme.Warning.Printf("unknown provider %q\n", r.cfg.AI.Provider)
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
	theme.Muted.Println("  Config: config.yaml (+ env var overrides)")

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

	printSection := func(title string, cmds []helpCmd) {
		theme.Primary.Printf("  %s\n", title)
		for _, c := range cmds {
			fmt.Printf("    %-36s %s\n", c.name, c.desc)
		}
		fmt.Println()
	}

	printSection("Merge Requests", []helpCmd{
		{"mr-status <project>", "List open MRs"},
		{"mr-review <project> [mr]", "AI-powered MR review"},
		{"mr-comment <project> [mr]", "Post review as MR comment"},
		{"mr-open <project> [branch] [target]", "Create a new MR"},
		{"mr-merge <project> <mr>", "Merge an MR"},
		{"mr-approve <project> <mr>", "Approve an MR"},
		{"mr-unapprove <project> <mr>", "Remove approval"},
		{"mr-rebase <project> <mr>", "Rebase source branch"},
		{"mr-update <project> <mr>", "Update MR metadata"},
		{"mr-close <project> <mr>", "Close an MR"},
		{"mr-reopen <project> <mr>", "Reopen a closed MR"},
		{"mr-checks <project> <mr>", "Pipeline status for MR"},
	})

	printSection("Pipelines / CI", []helpCmd{
		{"pipeline <project>", "List recent pipelines"},
		{"pipeline-view <project> <id>", "Pipeline details and jobs"},
		{"pipeline-logs <project> <job-id>", "Job log output"},
		{"pipeline-retry <project> <id>", "Retry failed pipeline"},
		{"pipeline-cancel <project> <id>", "Cancel running pipeline"},
	})

	printSection("Tickets / Issues", []helpCmd{
		{"ticket-open [project]", "Create a new ticket"},
		{"ticket-open-empty", "Quick-create empty ticket (assigned to you)"},
		{"ticket-close <project> <number>", "Close a ticket"},
		{"ticket-reopen <project> <number>", "Reopen a ticket"},
		{"ticket-update <project> <number>", "Update ticket metadata"},
		{"ticket-search <project>", "Search tickets"},
		{"tickets", "Generate tickets report"},
	})

	printSection("Repository", []helpCmd{
		{"list / projects", "List team projects"},
		{"diff <project>", "Compare tags or branches"},
		{"branch-cleanup <project>", "Remove stale branches"},
		{"release", "Check release status"},
	})

	printSection("Content Generation", []helpCmd{
		{"create-ticket-content [project]", "Generate ticket content from diff"},
		{"create-ticket-desc [project]", "Generate description from linked MRs"},
		{"create-epic-content [project]", "Generate epic content from diff"},
	})

	printSection("Workflow", []helpCmd{
		{"ship", "Full flow: ticket → multi-select folders → commit → push → MR → update ticket"},
	})

	printSection("Session", []helpCmd{
		{"start", "Start session"},
		{"switch-team", "Switch active team"},
		{"config", "Show current configuration"},
		{"help", "Show this help"},
		{"exit", "End session"},
	})

	theme.Muted.Println("  Any unrecognized input is sent to AI as a question.")
	theme.Muted.Println("  Tab = auto-complete | Up/Down = history | Ctrl+R = search history")
	fmt.Println()
}

type helpCmd struct {
	name string
	desc string
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
	r.activeTeam = name

	r.cacheMu.Lock()
	r.projectCache = nil
	r.lastRefreshTime = time.Time{}
	r.cacheMu.Unlock()

	r.cacheReady = make(chan struct{})
	go r.fetchProjectCache()

	s := newSpinner(fmt.Sprintf(" Switching to team '%s'...", name))
	s.Start()
	projects := r.waitForCache()
	s.Stop()

	output.PrintSuccess(fmt.Sprintf("Switched team: %s → %s", prev, name))

	pf := r.cfg.ActiveParentFolder(name)
	if pf != "" {
		output.PrintSuccess(fmt.Sprintf("Parent folder: %s", pf))
	}

	if len(projects) > 0 {
		output.PrintSuccess(fmt.Sprintf("Loaded %d projects for team '%s'", len(projects), name))
	} else {
		output.PrintWarning(fmt.Sprintf("No projects found for team '%s'. Loading full list in background...", name))
	}

	go r.ensureFullCache()
	fmt.Println()
}
