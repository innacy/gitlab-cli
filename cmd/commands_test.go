package cmd

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestCommandTableIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range commandTable {
		if seen[c.name] {
			t.Errorf("duplicate command %q", c.name)
		}
		seen[c.name] = true
		if c.run == nil && c.name != "exit" {
			t.Errorf("%s has no handler", c.name)
		}
		if !slices.Contains(sectionOrder, c.section) {
			t.Errorf("%s is in unknown section %q (missing from help)", c.name, c.section)
		}
		if c.desc == "" {
			t.Errorf("%s has no description", c.name)
		}
	}
	for alias, target := range commandAliases {
		if lookupCommand(alias) == nil || lookupCommand(alias) != lookupCommand(target) {
			t.Errorf("alias %q does not resolve to %q", alias, target)
		}
	}
}

func TestEveryCommandIsCompletedAndListedForAI(t *testing.T) {
	r := &replState{}
	var completed []string
	for _, item := range r.buildCompleter().GetChildren() {
		completed = append(completed, strings.TrimSpace(string(item.GetName())))
	}
	list := commandListText()
	for _, c := range commandTable {
		if !slices.Contains(completed, c.name) {
			t.Errorf("%s missing from tab completion", c.name)
		}
		if !strings.Contains(list, "- "+c.name+" ") && !strings.Contains(list, "- "+c.name+" —") {
			t.Errorf("%s missing from AI command list", c.name)
		}
	}
	if !strings.Contains(intentSystemPrompt(), "- mr-review <project> [mr]") {
		t.Error("intent prompt is not built from the command table")
	}
}

// The README is hand-written; this keeps it from drifting from the table.
func TestReadmeDocumentsEveryCommand(t *testing.T) {
	readme, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range commandTable {
		if !strings.Contains(string(readme), "`"+c.name) {
			t.Errorf("README.md does not document %q", c.name)
		}
	}
}

func TestSuggestCommand(t *testing.T) {
	cases := map[string]string{
		"mr-staus":     "mr-status",
		"pipline":      "pipeline",
		"tickest":      "tickets",
		"lsit":         "list",
		"what":         "",
		"hello":        "",
		"mr-status":    "mr-status",
		"swich-env":    "switch-env",
		"completely-x": "",
	}
	for in, want := range cases {
		if got := suggestCommand(in); got != want {
			t.Errorf("suggestCommand(%q) = %q, want %q", in, got, want)
		}
	}
}
