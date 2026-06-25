package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gitlab-ai/pkg/output"
)

type intentResult struct {
	Command    string            `json:"command"`
	Args       map[string]string `json:"args"`
	Confidence float64           `json:"confidence"`
}

const intentSystemPrompt = `You are a command classifier for git-agent CLI. Given user input, determine which command they want and extract arguments.

Available commands:
- mr-review <project> [mr-number] — AI-powered MR review
- mr-status <project> — List open MRs
- mr-open <project> [branch] [target] — Create a new MR
- mr-merge <project> <mr-number> — Merge an MR
- mr-approve <project> <mr-number> — Approve an MR
- mr-comment <project> [mr-number] — Post review as MR comment
- mr-checks <project> <mr-number> — Pipeline status for MR
- mr-update <project> <mr-number> — Update MR metadata
- mr-close <project> <mr-number> — Close an MR
- mr-reopen <project> <mr-number> — Reopen a closed MR
- pipeline <project> — List recent pipelines
- pipeline-view <project> <id> — View pipeline details
- pipeline-logs <project> <job-id> — Show job log output
- pipeline-retry <project> <id> — Retry a failed pipeline
- ticket-open [project] — Create a new ticket
- ticket-close <project> <number> — Close a ticket
- ticket-search <project> — Search tickets
- tickets — Generate tickets report
- create-ticket-content [project] — Generate ticket content from diff
- create-ticket-desc [project] — Generate description from linked MRs
- create-epic-content [project] — Generate epic content from diff
- diff <project> — Compare tags or branches
- branch-cleanup <project> — Remove stale branches
- release — Check release status
- list — List team projects
- ship — Full flow: ticket → multi-select folders → commit → MR → update ticket
- config — Show configuration
- help — Show commands
- exit — End session

Respond with ONLY valid JSON (no markdown fences):
{"command":"<command-name>","args":{"project":"...","mr":"..."},"confidence":0.0-1.0}

If the input is a general question (not a command), respond:
{"command":"chat","args":{},"confidence":1.0}

Rules:
- confidence: 1.0 = exact match, 0.8+ = strong intent, 0.5-0.8 = ambiguous
- Extract project names, MR numbers, branch names from natural language
- "latest" or "last" MR means omit mr number (interactive picker will handle it)
- If unsure which command, set lower confidence`

func (r *replState) classifyIntent(input string) *intentResult {
	if r.aiClient == nil {
		if err := r.ensureAI(); err != nil {
			return nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	response, err := r.aiClient.Chat(ctx, intentSystemPrompt, input)
	if err != nil {
		return nil
	}

	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var result intentResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil
	}

	return &result
}

var knownCommands = map[string]bool{
	"start": true, "list": true, "projects": true,
	"mr-review": true, "mr-comment": true, "mr-status": true, "mr-checks": true,
	"mr-open": true, "mr-merge": true, "mr-approve": true, "mr-unapprove": true,
	"mr-rebase": true, "mr-update": true, "mr-close": true, "mr-reopen": true,
	"pipeline": true, "pipeline-view": true, "pipeline-logs": true,
	"pipeline-retry": true, "pipeline-cancel": true,
	"diff": true, "branch-cleanup": true, "tickets": true,
	"ticket-open": true, "ticket-open-empty": true, "ship": true,
	"ticket-close": true, "ticket-reopen": true, "ticket-update": true,
	"ticket-search": true, "create-ticket-desc": true, "tickets-black": true,
	"create-ticket-content": true, "create-epic-content": true,
	"release": true, "config": true, "help": true, "exit": true,
}

func (r *replState) handleIntentOrChat(line string) {
	intent := r.classifyIntent(line)
	if intent == nil || intent.Command == "chat" || intent.Command == "" {
		r.handleChat(line)
		return
	}

	if !knownCommands[intent.Command] {
		r.handleChat(line)
		return
	}

	if intent.Confidence >= 0.8 {
		parts := buildCommandParts(intent)
		output.GetTheme().Muted.Printf("  → %s\n", strings.Join(parts, " "))
		fmt.Println()
		r.dispatch(strings.Join(parts, " "))
	} else if intent.Confidence >= 0.5 {
		parts := buildCommandParts(intent)
		suggestion := strings.Join(parts, " ")
		choice := r.promptForChoice(
			fmt.Sprintf("Did you mean: %s?", suggestion),
			[]string{"Yes, run it", "No, ask AI instead"},
		)
		if choice == 0 {
			r.dispatch(suggestion)
		} else {
			r.handleChat(line)
		}
	} else {
		r.handleChat(line)
	}
}

func buildCommandParts(intent *intentResult) []string {
	parts := []string{intent.Command}
	if p, ok := intent.Args["project"]; ok && p != "" {
		parts = append(parts, p)
	}
	if mr, ok := intent.Args["mr"]; ok && mr != "" {
		parts = append(parts, mr)
	}
	if branch, ok := intent.Args["branch"]; ok && branch != "" {
		parts = append(parts, branch)
	}
	if target, ok := intent.Args["target"]; ok && target != "" {
		parts = append(parts, target)
	}
	if id, ok := intent.Args["id"]; ok && id != "" {
		parts = append(parts, id)
	}
	if num, ok := intent.Args["number"]; ok && num != "" {
		parts = append(parts, num)
	}
	return parts
}
