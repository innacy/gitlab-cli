package cmd

import (
	"fmt"
	"strings"

	"github.com/chzyer/readline"

	"gitlab-ai/pkg/audit"
)

// command is the single source of truth for a REPL command: dispatch, help,
// tab completion, intent classification and the AI assistant's command list
// are all generated from commandTable.
type command struct {
	name     string
	usage    string // argument synopsis shown after the name
	desc     string
	section  string
	category string                      // audit category; "" means the command is not recorded
	complete func(r *replState) []string // tab-completion candidates for the first argument
	subs     []string                    // fixed sub-commands offered by tab completion
	run      func(r *replState, args []string)
}

func (c *command) synopsis() string {
	if c.usage == "" {
		return c.name
	}
	return c.name + " " + c.usage
}

var sectionOrder = []string{
	"Merge Requests", "Pipelines / CI", "Tickets / Issues", "Repository",
	"Content Generation", "Workflow", "Stats", "Session",
}

// Completion sources for command.complete.
var (
	projects     = (*replState).projectSuggestions
	environments = func(r *replState) []string { return r.cfg.EnvironmentNames() }
)

// noArgs adapts a handler that takes no arguments.
func noArgs(f func(*replState)) func(*replState, []string) {
	return func(r *replState, _ []string) { f(r) }
}

// commandTable and commandsByName are filled in init: handlers such as
// showHelp read the table, so a static initializer would be a cycle.
var (
	commandTable   []*command
	commandsByName map[string]*command
)

func init() {
	commandTable = []*command{
		{name: "mr-status", usage: "<project>", desc: "List open MRs", section: "Merge Requests", category: audit.CategoryRead, complete: projects, run: (*replState).handleMRStatus},
		{name: "mr-review", usage: "<project> [mr]", desc: "AI-powered MR review", section: "Merge Requests", category: audit.CategoryAIGeneration, complete: projects, run: (*replState).handleMRReview},
		{name: "mr-comment", usage: "<project> [mr]", desc: "Post review as MR comment", section: "Merge Requests", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handleMRComment},
		{name: "mr-open", usage: "<project> [branch] [target]", desc: "Create a new MR", section: "Merge Requests", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handleMROpen},
		{name: "mr-merge", usage: "<project> <mr>", desc: "Merge an MR", section: "Merge Requests", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handleMRMerge},
		{name: "mr-approve", usage: "<project> <mr>", desc: "Approve an MR", section: "Merge Requests", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handleMRApprove},
		{name: "mr-unapprove", usage: "<project> <mr>", desc: "Remove your approval", section: "Merge Requests", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handleMRUnapprove},
		{name: "mr-rebase", usage: "<project> <mr>", desc: "Rebase source branch", section: "Merge Requests", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handleMRRebase},
		{name: "mr-update", usage: "<project> <mr>", desc: "Update MR metadata", section: "Merge Requests", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handleMRUpdate},
		{name: "mr-close", usage: "<project> <mr>", desc: "Close an MR", section: "Merge Requests", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handleMRClose},
		{name: "mr-reopen", usage: "<project> <mr>", desc: "Reopen a closed MR", section: "Merge Requests", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handleMRReopen},
		{name: "mr-checks", usage: "<project> <mr>", desc: "Pipeline status for an MR", section: "Merge Requests", category: audit.CategoryRead, complete: projects, run: (*replState).handleMRChecks},

		{name: "pipeline", usage: "<project>", desc: "List recent pipelines", section: "Pipelines / CI", category: audit.CategoryRead, complete: projects, run: (*replState).handlePipeline},
		{name: "pipeline-view", usage: "<project> <id>", desc: "Pipeline details and jobs", section: "Pipelines / CI", category: audit.CategoryRead, complete: projects, run: (*replState).handlePipelineView},
		{name: "pipeline-logs", usage: "<project> <job-id>", desc: "Job log output", section: "Pipelines / CI", category: audit.CategoryRead, complete: projects, run: (*replState).handlePipelineLogs},
		{name: "pipeline-retry", usage: "<project> <id>", desc: "Retry a failed pipeline", section: "Pipelines / CI", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handlePipelineRetry},
		{name: "pipeline-cancel", usage: "<project> <id>", desc: "Cancel a running pipeline", section: "Pipelines / CI", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handlePipelineCancel},
		{name: "pipeline-triage", usage: "<project> [id]", desc: "AI triage of a failed pipeline", section: "Pipelines / CI", category: audit.CategoryAIGeneration, complete: projects, run: (*replState).handlePipelineTriage},

		{name: "ticket-open", usage: "[project]", desc: "Create a new ticket", section: "Tickets / Issues", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handleTicketOpen},
		{name: "ticket-open-empty", desc: "Quick-create an empty ticket assigned to you", section: "Tickets / Issues", category: audit.CategoryGitLabMutation, run: (*replState).handleTicketOpenEmpty},
		{name: "ticket-close", usage: "<project> <number>", desc: "Close a ticket", section: "Tickets / Issues", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handleTicketClose},
		{name: "ticket-reopen", usage: "<project> <number>", desc: "Reopen a ticket", section: "Tickets / Issues", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handleTicketReopen},
		{name: "ticket-update", usage: "<project> <number>", desc: "Update ticket metadata", section: "Tickets / Issues", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handleTicketUpdate},
		{name: "ticket-search", usage: "<project>", desc: "Search tickets", section: "Tickets / Issues", category: audit.CategoryRead, complete: projects, run: (*replState).handleTicketSearch},
		{name: "tickets", desc: "Generate open tickets report", section: "Tickets / Issues", category: audit.CategoryRead, run: (*replState).handleTickets},
		{name: "tickets-black", desc: "Generate malformed tickets report", section: "Tickets / Issues", category: audit.CategoryRead, run: (*replState).handleTicketsBlack},

		{name: "list", desc: "List team projects (alias: projects)", section: "Repository", category: audit.CategoryRead, run: noArgs((*replState).handleList)},
		{name: "diff", usage: "<project>", desc: "Compare tags or branches", section: "Repository", category: audit.CategoryRead, complete: projects, run: (*replState).handleDiff},
		{name: "branch-cleanup", usage: "<project>", desc: "Remove stale merged branches", section: "Repository", category: audit.CategoryGitLabMutation, complete: projects, run: (*replState).handleBranchCleanup},
		{name: "release", desc: "Check release status of all projects", section: "Repository", category: audit.CategoryRead, run: noArgs((*replState).handleRelease)},

		{name: "create-ticket-content", usage: "[project]", desc: "Generate ticket content from branch diff", section: "Content Generation", category: audit.CategoryAIGeneration, complete: projects, run: (*replState).handleCreateTicketContent},
		{name: "create-ticket-desc", usage: "[project]", desc: "Generate ticket description from linked MRs", section: "Content Generation", category: audit.CategoryAIGeneration, complete: projects, run: (*replState).handleTicketDescribe},
		{name: "create-epic-content", usage: "[project]", desc: "Generate epic content from branch diff", section: "Content Generation", category: audit.CategoryAIGeneration, complete: projects, run: (*replState).handleCreateEpicContent},

		{name: "ship", desc: "Full flow: ticket → folders → commit → push → MR → update ticket", section: "Workflow", category: audit.CategoryAIGeneration, run: (*replState).handleShip},

		{name: "stats", usage: "[week|month [YYYY-MM]|dashboard]", desc: "Activity summary or web dashboard", section: "Stats", subs: []string{"week", "month", "dashboard"}, run: (*replState).handleStats},

		{name: "start", desc: "Start session (auto-runs on first command)", section: "Session", run: noArgs((*replState).handleStart)},
		{name: "switch-team", desc: "Switch active team", section: "Session", run: noArgs((*replState).handleSwitchTeam)},
		{name: "switch-env", usage: "[name]", desc: "Switch environment profile (GitLab, AI, teams)", section: "Session", complete: environments, run: (*replState).handleSwitchEnv},
		{name: "config", desc: "Show current configuration", section: "Session", run: noArgs((*replState).handleConfig)},
		{name: "help", desc: "Show this help", section: "Session", run: noArgs((*replState).showHelp)},
		{name: "exit", desc: "End session", section: "Session"}, // handled by dispatch
	}

	commandsByName = make(map[string]*command, len(commandTable)+len(commandAliases))
	for _, c := range commandTable {
		commandsByName[c.name] = c
	}
	for alias, target := range commandAliases {
		commandsByName[alias] = commandsByName[target]
	}
}

var commandAliases = map[string]string{"projects": "list"}

var renamedCommands = map[string]string{
	"mr":       "Command renamed → use 'mr-review' instead.",
	"add":      "Command renamed → use 'mr-comment' instead.",
	"index":    "Command removed. Context is managed automatically during reviews.",
	"raise-mr": "Command renamed → use 'mr-open' instead.",
}

func lookupCommand(name string) *command { return commandsByName[name] }

// commandListText renders "- name usage — desc" lines, used to tell the AI
// which commands exist.
func commandListText() string {
	var sb strings.Builder
	for _, section := range sectionOrder {
		fmt.Fprintf(&sb, "\n%s:\n", section)
		for _, c := range commandTable {
			if c.section == section {
				fmt.Fprintf(&sb, "- %s — %s\n", c.synopsis(), c.desc)
			}
		}
	}
	return sb.String()
}

func (r *replState) buildCompleter() *readline.PrefixCompleter {
	items := make([]readline.PrefixCompleterInterface, 0, len(commandTable)+len(commandAliases))
	add := func(name string, c *command) {
		var children []readline.PrefixCompleterInterface
		if c.complete != nil {
			complete := c.complete
			children = append(children, readline.PcItemDynamic(func(string) []string { return complete(r) }))
		}
		for _, s := range c.subs {
			children = append(children, readline.PcItem(s))
		}
		items = append(items, readline.PcItem(name, children...))
	}
	for _, c := range commandTable {
		add(c.name, c)
	}
	for alias, target := range commandAliases {
		add(alias, commandsByName[target])
	}
	return readline.NewPrefixCompleter(items...)
}
