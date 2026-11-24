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

func intentSystemPrompt() string {
	return `You are a command classifier for git-agent CLI. Given user input, determine which command they want and extract arguments.

Available commands:
` + commandListText() + `
Respond with ONLY valid JSON (no markdown fences):
{"command":"<command-name>","args":{"project":"...","mr":"..."},"confidence":0.0-1.0}

If the input is a general question (not a command), respond:
{"command":"chat","args":{},"confidence":1.0}

Rules:
- confidence: 1.0 = exact match, 0.8+ = strong intent, 0.5-0.8 = ambiguous
- Extract project names, MR numbers, branch names from natural language
- "latest" or "last" MR means omit mr number (interactive picker will handle it)
- If unsure which command, set lower confidence`
}

func (r *replState) classifyIntent(input string) *intentResult {
	if r.aiClient == nil {
		if err := r.ensureAI(); err != nil {
			return nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	chatResult, err := r.aiClient.Chat(ctx, intentSystemPrompt(), input)
	if err != nil {
		return nil
	}
	response := chatResult.Text

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

func (r *replState) handleIntentOrChat(line string) {
	intent := r.classifyIntent(line)
	if intent == nil || intent.Command == "chat" || intent.Command == "" {
		r.handleChat(line)
		return
	}

	if lookupCommand(intent.Command) == nil {
		r.handleChat(line)
		return
	}

	if intent.Confidence >= 0.8 {
		parts := buildCommandParts(intent)
		output.GetTheme().Muted.Printf("  → %s\n", strings.Join(parts, " "))
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
