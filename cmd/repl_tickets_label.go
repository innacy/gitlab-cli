package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/audit"
	"gitlab-ai/pkg/output"
	"gitlab-ai/pkg/platform"
)

const (
	maxSuggestedLabels   = 3
	labelDescCharLimit   = 4000
	defaultNewLabelColor = "#428BCA"
)

var hexColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

type labelSuggestion struct {
	Labels   []string `json:"labels"`
	NewLabel string   `json:"new_label"`
	Color    string   `json:"color"`
	Reason   string   `json:"reason"`
}

type ownTicket struct {
	project string
	issue   models.Issue
}

// parseLabelSuggestion decodes the AI reply and keeps only labels that exist
// (case-insensitive, returned with their canonical spelling), capped at
// maxSuggestedLabels. A new label is only kept when no existing label matched.
func parseLabelSuggestion(reply string, available []string) (labelSuggestion, error) {
	reply = strings.TrimSpace(reply)
	if i, j := strings.Index(reply, "{"), strings.LastIndex(reply, "}"); i >= 0 && j > i {
		reply = reply[i : j+1]
	}
	var raw labelSuggestion
	if err := json.Unmarshal([]byte(reply), &raw); err != nil {
		return labelSuggestion{}, fmt.Errorf("invalid AI response: %w", err)
	}

	out := labelSuggestion{Reason: strings.TrimSpace(raw.Reason)}
	for _, want := range raw.Labels {
		for _, have := range available {
			if strings.EqualFold(strings.TrimSpace(want), have) && !slices.Contains(out.Labels, have) {
				out.Labels = append(out.Labels, have)
				break
			}
		}
		if len(out.Labels) == maxSuggestedLabels {
			break
		}
	}
	if len(out.Labels) == 0 {
		out.NewLabel = strings.TrimSpace(raw.NewLabel)
		out.Color = defaultNewLabelColor
		if hexColorRe.MatchString(strings.TrimSpace(raw.Color)) {
			out.Color = strings.TrimSpace(raw.Color)
		}
	}
	return out, nil
}

func labelSystemPrompt() string {
	return `You assign GitLab labels to issues. Reply with ONLY a JSON object:
{"labels": ["..."], "new_label": "", "color": "", "reason": "..."}
Rules:
- "labels": 0 to 3 names copied EXACTLY from the available list; fewer is fine; only pick clearly relevant ones.
- If none of the available labels fit, leave "labels" empty and propose ONE short "new_label" plus a "color" as #RRGGBB hex.
- "reason": one short sentence.`
}

func (r *replState) handleTicketsLabel(args []string) {
	if !r.ensureSession() {
		return
	}
	if err := r.ensureAI(); err != nil {
		output.PrintError(err.Error())
		return
	}
	dryRun := slices.Contains(args, "--dry-run")
	all := slices.Contains(args, "--all")

	projects := r.allTeamProjects()
	if len(projects) == 0 {
		return
	}

	meID := r.provider.UserID()
	s := output.NewProgress(len(projects), "Fetching your open tickets...")
	s.Start()
	var (
		mu          sync.Mutex
		tickets     []ownTicket
		fetchErrors int
		wg          sync.WaitGroup
	)
	sem := make(chan struct{}, 5)
	for _, p := range projects {
		wg.Add(1)
		go func(project string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			result, err := r.provider.Issues().ListProjectIssues(project, models.IssueFilter{State: "opened", AuthorID: meID})
			s.Inc()
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fetchErrors++
				return
			}
			for _, issue := range result.Issues {
				if all || len(issue.Labels) == 0 {
					tickets = append(tickets, ownTicket{project: project, issue: issue})
				}
			}
		}(p.Path)
	}
	wg.Wait()
	s.Stop()

	if fetchErrors > 0 {
		output.PrintWarning(fmt.Sprintf("%d project(s) could not be fetched and were skipped.", fetchErrors))
	}
	if len(tickets) == 0 {
		output.PrintSuccess("No matching open tickets created by you.")
		return
	}
	slices.SortFunc(tickets, func(a, b ownTicket) int { return b.issue.CreatedAt.Compare(a.issue.CreatedAt) })

	items := make([]string, len(tickets))
	for i, t := range tickets {
		cur := "none"
		if len(t.issue.Labels) > 0 {
			cur = strings.Join(t.issue.Labels, ", ")
		}
		items[i] = fmt.Sprintf("%s#%d  %s  (labels: %s)", t.project, t.issue.IID, t.issue.Title, cur)
	}
	selected := r.promptForMultiSelect("Select tickets to label", items, 10)
	if len(selected) == 0 {
		output.PrintWarning("No tickets selected.")
		return
	}

	labelCache := map[string][]string{}
	labelled, created, skipped := 0, 0, 0
	for _, idx := range selected {
		t := tickets[idx]
		ref := fmt.Sprintf("%s#%d", t.project, t.issue.IID)
		fmt.Printf("\n── %s  %s\n", ref, t.issue.Title)

		available, ok := labelCache[t.project]
		if !ok {
			var err error
			if available, err = r.provider.Issues().ListProjectLabels(t.project); err != nil {
				output.PrintError(fmt.Sprintf("%s: %v", ref, err))
				skipped++
				continue
			}
			labelCache[t.project] = available
		}

		sug, err := r.suggestLabels(t.issue, available)
		if err != nil {
			output.PrintError(fmt.Sprintf("%s: %v", ref, err))
			skipped++
			continue
		}
		if sug.Reason != "" {
			fmt.Printf("   reason: %s\n", sug.Reason)
		}

		var apply []string
		switch {
		case len(sug.Labels) > 0:
			fmt.Printf("   suggested: %s\n", strings.Join(sug.Labels, ", "))
			if dryRun {
				continue
			}
			if !r.promptForYesNo("Apply these labels?") {
				skipped++
				continue
			}
			apply = sug.Labels
		case sug.NewLabel != "":
			fmt.Printf("   no existing label fits; proposed new label: %q (%s)\n", sug.NewLabel, sug.Color)
			if dryRun {
				continue
			}
			if !r.promptForYesNo(fmt.Sprintf("Create project label %q in %s and apply?", sug.NewLabel, t.project)) {
				skipped++
				continue
			}
			if err := r.provider.Issues().CreateProjectLabel(t.project, sug.NewLabel, sug.Color); err != nil {
				output.PrintError(fmt.Sprintf("%s: %v", ref, err))
				skipped++
				continue
			}
			created++
			labelCache[t.project] = append(available, sug.NewLabel)
			apply = []string{sug.NewLabel}
		default:
			output.PrintWarning("   AI returned no suggestion; skipping.")
			skipped++
			continue
		}

		if _, err := r.provider.Issues().UpdateIssue(t.project, t.issue.IID, platform.UpdateIssueOptions{AddLabels: apply}); err != nil {
			output.PrintError(fmt.Sprintf("%s: %v", ref, err))
			skipped++
			continue
		}
		output.PrintSuccess(fmt.Sprintf("%s labelled: %s", ref, strings.Join(apply, ", ")))
		labelled++
	}

	fmt.Println()
	if dryRun {
		output.PrintSuccess("Dry run: no changes made.")
		return
	}
	output.PrintSuccess(fmt.Sprintf("Labelled %d ticket(s), created %d label(s), skipped %d.", labelled, created, skipped))
	if labelled > 0 || created > 0 {
		r.recordEffect(audit.CategoryGitLabMutation)
	}
}

func (r *replState) suggestLabels(issue models.Issue, available []string) (labelSuggestion, error) {
	desc := issue.Description
	if len(desc) > labelDescCharLimit {
		desc = desc[:labelDescCharLimit]
	}
	prompt := fmt.Sprintf("Available labels:\n%s\n\nIssue title: %s\n\nIssue description:\n%s",
		strings.Join(available, "\n"), issue.Title, desc)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := r.aiClient.Chat(ctx, labelSystemPrompt(), prompt)
	if err != nil {
		return labelSuggestion{}, err
	}
	return parseLabelSuggestion(res.Text, available)
}
