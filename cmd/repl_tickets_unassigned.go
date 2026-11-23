package cmd

import (
	"fmt"
	"slices"
	"sync"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/audit"
	"gitlab-ai/pkg/output"
	"gitlab-ai/pkg/platform"
)

// ponytail: hardcoded; move to config if the redirect list grows.
const redirectAuthorUsername = "devesh.kumar"

type unassignedTicket struct {
	project      string
	issue        models.Issue
	targetID     int
	targetHandle string
}

// assignTarget picks who an unassigned ticket goes to: its creator, or the
// current user when the creator is redirectAuthorUsername. ok=false when the
// author is unknown.
func assignTarget(issue models.Issue, meID int, meUsername string) (int, string, bool) {
	if issue.AuthorID == 0 {
		return 0, "", false
	}
	if issue.Author == redirectAuthorUsername {
		return meID, meUsername, true
	}
	return issue.AuthorID, issue.Author, true
}

func (r *replState) handleTicketsUnassigned(args []string) {
	if !r.ensureSession() {
		return
	}
	dryRun := slices.Contains(args, "--dry-run")

	projects := r.allTeamProjects()
	if len(projects) == 0 {
		return
	}

	s := output.NewProgress(len(projects), "Scanning unassigned tickets...")
	s.Start()
	type projectIssues struct {
		project string
		issues  []models.Issue
		err     error
	}
	ch := make(chan projectIssues, len(projects))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for _, p := range projects {
		wg.Add(1)
		go func(project string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			result, err := r.provider.Issues().ListProjectIssues(project, models.IssueFilter{State: "opened"})
			s.Inc()
			if err != nil {
				ch <- projectIssues{project: project, err: err}
				return
			}
			ch <- projectIssues{project: project, issues: result.Issues}
		}(p.Path)
	}
	go func() { wg.Wait(); close(ch) }()

	meID, meUsername := r.provider.UserID(), r.provider.Username()
	var todo []unassignedTicket
	fetchErrors := 0
	for res := range ch {
		if res.err != nil {
			fetchErrors++
			continue
		}
		for _, issue := range res.issues {
			if issue.Assignee != "" {
				continue
			}
			id, handle, ok := assignTarget(issue, meID, meUsername)
			if !ok {
				continue
			}
			todo = append(todo, unassignedTicket{project: res.project, issue: issue, targetID: id, targetHandle: handle})
		}
	}
	s.Stop()

	if fetchErrors > 0 {
		output.PrintWarning(fmt.Sprintf("%d project(s) could not be fetched and were skipped.", fetchErrors))
	}
	if len(todo) == 0 {
		output.PrintSuccess("No unassigned open tickets found.")
		return
	}

	slices.SortFunc(todo, func(a, b unassignedTicket) int {
		if a.project != b.project {
			if a.project < b.project {
				return -1
			}
			return 1
		}
		return a.issue.IID - b.issue.IID
	})
	for _, t := range todo {
		fmt.Printf("  %s#%d  %s\n      creator @%s → assign @%s\n", t.project, t.issue.IID, t.issue.Title, t.issue.Author, t.targetHandle)
	}
	fmt.Println()

	if dryRun {
		output.PrintSuccess(fmt.Sprintf("Dry run: %d ticket(s) would be assigned.", len(todo)))
		return
	}
	if !r.confirm(fmt.Sprintf("Assign %d ticket(s) as listed?", len(todo))) {
		return
	}

	updated, failed := 0, 0
	for _, t := range todo {
		id := t.targetID
		if _, err := r.provider.Issues().UpdateIssue(t.project, t.issue.IID, platform.UpdateIssueOptions{AssigneeID: &id}); err != nil {
			failed++
			output.PrintError(fmt.Sprintf("%s#%d → @%s: %v", t.project, t.issue.IID, t.targetHandle, err))
			continue
		}
		updated++
	}
	output.PrintSuccess(fmt.Sprintf("Assigned %d ticket(s), %d failed.", updated, failed))
	if updated > 0 {
		r.recordEffect(audit.CategoryGitLabMutation)
	}
}
