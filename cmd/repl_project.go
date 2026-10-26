package cmd

import (
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitlab-ai/internal/models"
	"gitlab-ai/pkg/output"
	"gitlab-ai/pkg/platform"
)

// ─── List ────────────────────────────────────────────────────────────────────

func (r *replState) handleList() {
	if !r.ensureSession() {
		return
	}

	projects := r.ensureFullCache()

	if len(projects) == 0 {
		output.PrintWarning("No projects found.")
		return
	}

	output.PrintProjectsTable(projects)
	fmt.Println()
}

// ─── Branch Cleanup ──────────────────────────────────────────────────────────

func (r *replState) handleBranchCleanup(args []string) {
	if !r.ensureSession() {
		return
	}

	project := ""
	if len(args) > 0 {
		project = strings.Join(args, " ")
	}
	if project == "" {
		project = r.promptForProject("Select project for branch cleanup")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}
	project = r.resolveProject(project)

	s := output.NewLoader(fmt.Sprintf(" Scanning branches in '%s'...", project))
	s.Start()

	merged, err := r.provider.Repos().ListMergedBranches(project)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to list branches: %v", err))
		return
	}

	if len(merged) == 0 {
		output.PrintSuccess("No stale or merged branches found. All clean!")
		fmt.Println()
		return
	}

	output.PrintBranchesTable(merged, fmt.Sprintf("Stale/Merged Branches — %s", project))
	fmt.Println()

	if !r.promptForYesNoSafe(fmt.Sprintf("Delete all %d branches listed above?", len(merged))) {
		output.PrintWarning("Branch cleanup cancelled.")
		fmt.Println()
		return
	}

	deleted := 0
	for _, b := range merged {
		err := r.provider.Repos().DeleteBranch(project, b.Name)
		if err != nil {
			output.PrintError(fmt.Sprintf("  Failed to delete '%s': %v", b.Name, err))
		} else {
			output.PrintSuccess(fmt.Sprintf("  Deleted '%s'", b.Name))
			deleted++
		}
	}

	fmt.Println()
	output.PrintSuccess(fmt.Sprintf("Cleanup complete: %d/%d branches deleted", deleted, len(merged)))
	fmt.Println()
}

// ─── Tickets ─────────────────────────────────────────────────────────────────

func (r *replState) handleTickets(args []string) {
	if !r.ensureSession() {
		return
	}
	_ = args

	projects := r.allTeamProjects()
	if len(projects) == 0 {
		output.PrintWarning("No projects found for the selected team.")
		return
	}

	s := output.NewProgress(len(projects), "Fetching open tickets...")
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

	go func() {
		wg.Wait()
		close(ch)
	}()

	projectCounts := make(map[string]int, len(projects))
	assigneeCounts := map[string]int{"Unassigned": 0}
	rows := make([]ticketReportRow, 0, 500)
	errorCount := 0

	for res := range ch {
		if res.err != nil {
			errorCount++
			continue
		}

		projectCounts[res.project] = len(res.issues)
		for _, issue := range res.issues {
			rows = append(rows, ticketReportRow{Project: res.project, Issue: issue})

			assignee := strings.TrimSpace(issue.Assignee)
			if assignee == "" {
				assignee = "Unassigned"
			}
			assigneeCounts[assignee]++
		}
	}

	s.Stop()

	now := time.Now()
	filename := filepath.Join(r.cfg.Issues.Output.Directory, fmt.Sprintf("tickets_report_%s.md", now.Format("2006-01-02_15-04")))
	content := buildTicketsAggregateMarkdown(projectCounts, assigneeCounts, rows, now)

	if err := writeFile(filename, content); err != nil {
		output.PrintError(fmt.Sprintf("Failed to save tickets report: %v", err))
		return
	}

	if errorCount > 0 {
		output.PrintWarning(fmt.Sprintf("Completed with %d project fetch errors. See markdown report for available data.", errorCount))
	}
	output.PrintSuccess("Tickets report saved to:")
	output.PrintFilePath(filename)

	r.stats.issuesViewed += len(rows)
	r.stats.filesCreated++
}

func (r *replState) handleTicketOpen(args []string) {
	if !r.ensureSession() {
		return
	}

	project := ""
	if len(args) > 0 {
		project = strings.Join(args, " ")
	} else {
		project = r.selectProjectWithConfig("Select project for new ticket")
	}
	if project == "" {
		output.PrintError("No project selected.")
		return
	}
	project = r.resolveProject(project)

	fmt.Println()
	contextMode := r.promptForChoice("How would you like to provide ticket context?", []string{
		"Type a summary manually",
		"Auto-generate from branch commits & diff",
	})

	var title, description string
	switch contextMode {
	case 1:
		title, description = r.ticketFromBranchContext(project)
		if title == "" {
			return
		}
	default:
		output.PrintSuccess("Summarize ticket context in one clear sentence/paragraph.")
		ticketCtx := r.promptForText("ticket-context")
		if ticketCtx == "" {
			output.PrintError("Ticket context cannot be empty.")
			return
		}

		if err := r.ensureAI(); err == nil {
			s := output.NewLoader(" Enhancing ticket description with AI...")
			s.Start()
			aiTitle, aiDesc, aiErr := r.enhanceTicketDescription(ticketCtx)
			s.Stop()
			if aiErr != nil {
				output.PrintWarning(fmt.Sprintf("AI enhancement failed, using basic template: %v", aiErr))
				title, description = buildTicketContent(ticketCtx)
			} else {
				title, description = aiTitle, aiDesc
				output.PrintSuccess("AI-enhanced ticket description ready")
			}
		} else {
			output.PrintWarning("AI not available, using basic template")
			title, description = buildTicketContent(ticketCtx)
		}
	}

	labels, err := r.provider.Issues().ListProjectLabels(project)
	if err != nil {
		output.PrintWarning(fmt.Sprintf("Could not load labels for '%s': %v", project, err))
		labels = nil
	}

	const maxLabels = 5
	var createLabels []string
	if len(labels) > 0 {
		displayLabels := labels
		if len(displayLabels) > maxLabels {
			displayLabels = displayLabels[:maxLabels]
		}

		options := make([]string, 0, len(displayLabels)+1)
		options = append(options, displayLabels...)
		options = append(options, "— no label —")

		choice := r.promptForChoice("Select label (optional)", options)
		if choice >= 0 && choice < len(displayLabels) {
			createLabels = append(createLabels, displayLabels[choice])
		}
	}

	s := output.NewLoader(fmt.Sprintf(" Creating ticket in '%s'...", project))
	s.Start()
	issue, err := r.provider.Issues().CreateIssue(project, title, description, createLabels)
	s.Stop()
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to create ticket: %v", err))
		return
	}

	output.PrintSuccess(fmt.Sprintf("Ticket #%d created: %s", issue.IID, issue.Title))
	output.PrintSuccess("Status: todo")
	output.PrintSuccess("Assignee: none")
	if len(createLabels) == 0 {
		output.PrintSuccess("Label: none")
	} else {
		output.PrintSuccess(fmt.Sprintf("Label: %s", strings.Join(createLabels, ", ")))
	}
	output.PrintURLOpen(issue.WebURL)
	fmt.Println()

	r.refreshCacheAsync()
}

// ─── Ticket Close ────────────────────────────────────────────────────────────

func (r *replState) handleTicketClose(args []string) {
	if !r.ensureSession() {
		return
	}
	project, issueNumber := r.parseTicketArgs(args)
	if project == "" || issueNumber <= 0 {
		return
	}

	s := output.NewLoader(fmt.Sprintf(" Closing ticket #%d...", issueNumber))
	s.Start()
	err := r.provider.Issues().CloseIssue(project, issueNumber)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to close ticket: %v", err))
		return
	}
	output.PrintSuccess(fmt.Sprintf("Ticket #%d closed", issueNumber))
	fmt.Println()
}

// ─── Ticket Reopen ───────────────────────────────────────────────────────────

func (r *replState) handleTicketReopen(args []string) {
	if !r.ensureSession() {
		return
	}
	project, issueNumber := r.parseTicketArgs(args)
	if project == "" || issueNumber <= 0 {
		return
	}

	s := output.NewLoader(fmt.Sprintf(" Reopening ticket #%d...", issueNumber))
	s.Start()
	err := r.provider.Issues().ReopenIssue(project, issueNumber)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to reopen ticket: %v", err))
		return
	}
	output.PrintSuccess(fmt.Sprintf("Ticket #%d reopened", issueNumber))
	fmt.Println()
}

// ─── Ticket Update ───────────────────────────────────────────────────────────

func (r *replState) handleTicketUpdate(args []string) {
	if !r.ensureSession() {
		return
	}
	project, issueNumber := r.parseTicketArgs(args)
	if project == "" || issueNumber <= 0 {
		return
	}

	fields := []string{"Title", "Description", "Labels", "Cancel"}
	choice := r.promptForChoice("What to update?", fields)
	if choice < 0 || choice == 3 {
		return
	}

	var opts platform.UpdateIssueOptions
	switch choice {
	case 0:
		title := r.promptForText("new-title")
		if title == "" {
			output.PrintError("Title cannot be empty.")
			return
		}
		opts.Title = &title
	case 1:
		desc := r.promptForText("new-description")
		opts.Description = &desc
	case 2:
		labelsStr := r.promptForText("labels (comma-separated)")
		if labelsStr != "" {
			labels := strings.Split(labelsStr, ",")
			for i := range labels {
				labels[i] = strings.TrimSpace(labels[i])
			}
			opts.Labels = labels
		}
	}

	s := output.NewLoader(fmt.Sprintf(" Updating ticket #%d...", issueNumber))
	s.Start()
	issue, err := r.provider.Issues().UpdateIssue(project, issueNumber, opts)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to update ticket: %v", err))
		return
	}
	output.PrintSuccess(fmt.Sprintf("Ticket #%d updated: %s", issue.IID, issue.Title))
	fmt.Println()
}

// ─── Ticket Search ───────────────────────────────────────────────────────────

func (r *replState) handleTicketSearch(args []string) {
	if !r.ensureSession() {
		return
	}

	project := ""
	if len(args) > 0 {
		project = args[0]
	}
	if project == "" {
		project = r.promptForProject("Select project for ticket search")
		if project == "" {
			output.PrintError("No project selected.")
			return
		}
	}
	project = r.resolveProject(project)

	query := r.promptForText("search-query")
	if query == "" {
		output.PrintError("Search query cannot be empty.")
		return
	}

	s := output.NewLoader(fmt.Sprintf(" Searching tickets in '%s'...", project))
	s.Start()
	issues, err := r.provider.Issues().SearchIssues(project, query)
	s.Stop()

	if err != nil {
		output.PrintError(fmt.Sprintf("Search failed: %v", err))
		return
	}

	if len(issues) == 0 {
		output.PrintWarning("No tickets found matching the query.")
		return
	}

	result := &models.IssueListResult{
		ProjectName: project,
		Issues:      issues,
		TotalCount:  len(issues),
	}
	output.PrintIssuesTable(result)
	fmt.Println()
}

// ─── Shared Ticket Arg Parser ────────────────────────────────────────────────

func (r *replState) parseTicketArgs(args []string) (string, int) {
	project, remaining := parseProjectFlag(args)

	var issueNumberStr string
	if project == "" {
		for _, arg := range remaining {
			if _, err := strconv.Atoi(arg); err == nil && issueNumberStr == "" {
				issueNumberStr = arg
			} else if project == "" {
				project = arg
			}
		}
	} else if len(remaining) > 0 {
		issueNumberStr = remaining[0]
	}

	if project == "" {
		project = r.promptForProject("Select project")
		if project == "" {
			output.PrintError("No project selected.")
			return "", 0
		}
	}
	project = r.resolveProject(project)

	var issueNumber int
	if issueNumberStr != "" {
		n, err := strconv.Atoi(issueNumberStr)
		if err != nil {
			output.PrintError(fmt.Sprintf("Invalid ticket number: %s", issueNumberStr))
			return "", 0
		}
		issueNumber = n
	} else {
		issueNumber = r.promptForNumber("Ticket number")
		if issueNumber <= 0 {
			output.PrintError("Invalid ticket number.")
			return "", 0
		}
	}

	return project, issueNumber
}

func (r *replState) handleTicketsBlack(args []string) {
	if !r.ensureSession() {
		return
	}

	_ = args
	const minDescriptionChars = 40

	projects := r.allTeamProjects()
	if len(projects) == 0 {
		output.PrintWarning("No projects found for the selected team.")
		return
	}

	s := output.NewProgress(len(projects), "Scanning malformed tickets...")
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

	go func() {
		wg.Wait()
		close(ch)
	}()

	malformed := make([]ticketReportRow, 0, 200)
	errorCount := 0
	for res := range ch {
		if res.err != nil {
			errorCount++
			continue
		}
		for _, issue := range res.issues {
			descLen := len(strings.TrimSpace(issue.Description))
			if descLen == 0 || descLen < minDescriptionChars {
				malformed = append(malformed, ticketReportRow{
					Project: res.project,
					Issue:   issue,
				})
			}
		}
	}
	s.Stop()

	now := time.Now()
	filename := filepath.Join(r.cfg.Issues.Output.Directory, fmt.Sprintf("tickets_black_%s.md", now.Format("2006-01-02_15-04")))
	content := buildTicketsBlackMarkdown(malformed, minDescriptionChars, now)
	if err := writeFile(filename, content); err != nil {
		output.PrintError(fmt.Sprintf("Failed to save malformed tickets report: %v", err))
		return
	}

	if errorCount > 0 {
		output.PrintWarning(fmt.Sprintf("Completed with %d project fetch errors. See markdown report for available data.", errorCount))
	}
	output.PrintSuccess("Malformed tickets report saved to:")
	output.PrintFilePath(filename)
	fmt.Println()

	r.stats.filesCreated++
}

func (r *replState) allTeamProjects() []models.ProjectInfo {
	projects := r.ensureFullCache()
	if len(projects) == 0 {
		output.PrintWarning("No projects found.")
		return nil
	}

	return r.filterIgnoredProjects(projects)
}

func (r *replState) filterIgnoredProjects(projects []models.ProjectInfo) []models.ProjectInfo {
	ignoredList := r.cfg.ActiveIgnoredProjects(r.activeTeam)
	if len(ignoredList) == 0 {
		return projects
	}

	ignored := make(map[string]bool, len(ignoredList))
	for _, p := range ignoredList {
		ignored[strings.ToLower(strings.TrimSpace(p))] = true
	}

	filtered := make([]models.ProjectInfo, 0, len(projects))
	for _, p := range projects {
		if !ignored[strings.ToLower(p.Path)] {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// ─── Release ─────────────────────────────────────────────────────────────────

func (r *replState) handleRelease() {
	if !r.ensureSession() {
		return
	}

	projects := r.ensureFullCache()

	if len(projects) == 0 {
		output.PrintWarning("No projects found.")
		return
	}

	s := output.NewProgress(len(projects), "Checking release status...")
	s.Start()

	type indexedResult struct {
		idx  int
		info models.ProjectReleaseInfo
	}

	results := make([]models.ProjectReleaseInfo, len(projects))
	ch := make(chan indexedResult, len(projects))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for i, p := range projects {
		wg.Add(1)
		go func(idx int, proj models.ProjectInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			info := r.provider.Repos().CheckProjectRelease(proj.Path)
			s.Inc()
			ch <- indexedResult{idx: idx, info: info}
		}(i, p)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for res := range ch {
		results[res.idx] = res.info
	}

	s.Stop()

	now := time.Now()
	cutoff := now.AddDate(0, 0, -90)
	currentYear, currentMonth, _ := now.Date()

	report := &models.ReleaseReport{GeneratedAt: now}
	for _, info := range results {
		switch info.Status {
		case models.ReleasePending:
			if !info.LastDevCommitDate.IsZero() && info.LastDevCommitDate.After(cutoff) {
				y, m, _ := info.LastDevCommitDate.Date()
				if y == currentYear && m == currentMonth {
					report.NewPending = append(report.NewPending, info)
				} else {
					report.OldPending = append(report.OldPending, info)
				}
			}
		case models.ReleaseUpToDate:
			report.Released = append(report.Released, info)
		case models.ReleaseInvalid:
			report.Invalid = append(report.Invalid, info)
		}
	}

	sort.Slice(report.NewPending, func(i, j int) bool {
		return report.NewPending[i].LastDevCommitDate.After(report.NewPending[j].LastDevCommitDate)
	})
	sort.Slice(report.OldPending, func(i, j int) bool {
		return report.OldPending[i].LastDevCommitDate.After(report.OldPending[j].LastDevCommitDate)
	})

	output.PrintReleaseReport(report)

	filename := filepath.Join(r.cfg.Other.Directory, fmt.Sprintf("Release_items-%s.md", time.Now().Format("2006-01-02")))
	content := buildReleaseMarkdown(report)

	if err := writeFile(filename, content); err != nil {
		output.PrintError(fmt.Sprintf("Failed to save release report: %v", err))
		return
	}

	output.PrintSuccess("Release report saved to:")
	output.PrintFilePath(filename)
	fmt.Println()

	r.stats.filesCreated++

	if len(report.NewPending) > 0 {
		r.releaseCreateMRs(report.NewPending)
	}
}

func (r *replState) releaseCreateMRs(newPending []models.ProjectReleaseInfo) {
	items := make([]string, len(newPending))
	for i, p := range newPending {
		items[i] = fmt.Sprintf("%s  (%d commits, last: %s)",
			p.Name, p.CommitsAhead, output.TimeAgo(p.LastDevCommitDate))
	}

	indices := r.promptForMultiSelect(
		fmt.Sprintf("Raise release MRs — New repos (%d)", len(newPending)),
		items, 10)
	if len(indices) == 0 {
		output.PrintWarning("No repos selected.")
		return
	}

	currentMonth := time.Now().Format("January")
	mrTitle := fmt.Sprintf("Release - %s", currentMonth)
	mrDesc := fmt.Sprintf("This is a release MR for %s release.", currentMonth)
	if ref := r.promptForReleaseTicket(); ref != "" {
		// "Related to" links the MR to the ticket without closing it on merge.
		mrDesc += fmt.Sprintf("\n\nRelated to %s", ref)
	}

	fmt.Println()
	output.PrintSuccess(fmt.Sprintf("Creating release MRs for %d repo(s)...", len(indices)))
	fmt.Println()

	for _, idx := range indices {
		p := newPending[idx]
		sourceBranch := p.DevBranch
		targetBranch := p.MasterBranch

		s := output.NewLoader(fmt.Sprintf(" Creating MR: %s → %s in '%s'...", sourceBranch, targetBranch, p.Name))
		s.Start()
		mr, err := r.provider.MRs().CreateMergeRequest(p.Path, sourceBranch, targetBranch, mrTitle, mrDesc)
		s.Stop()

		if err != nil {
			output.PrintError(fmt.Sprintf("%s: Failed to create MR: %v", p.Name, err))
			continue
		}

		output.PrintSuccess(fmt.Sprintf("%s: MR !%d created", p.Name, mr.IID))
		output.PrintURLOpen(mr.WebURL)
		r.stats.mrsCreated++
	}
	fmt.Println()
}

// promptForReleaseTicket asks for an optional ticket and returns a full
// cross-project reference ("group/project#123"), since release MRs span many
// projects and a bare "#123" would resolve against each MR's own project.
func (r *replState) promptForReleaseTicket() string {
	for {
		input := r.promptForText("Release ticket (issue URL or group/project#123, blank to skip)")
		if input == "" {
			return ""
		}
		project, iid, ok := parseIssueRef(input)
		if !ok {
			output.PrintWarning("Use an issue URL or group/project#123.")
			continue
		}
		s := output.NewLoader(fmt.Sprintf(" Checking %s#%d...", project, iid))
		s.Start()
		issue, err := r.provider.Issues().GetIssue(project, iid)
		s.Stop()
		if err != nil {
			output.PrintWarning(fmt.Sprintf("Ticket not found: %v", err))
			continue
		}
		output.PrintSuccess(fmt.Sprintf("Linking ticket: %s", issue.Title))
		return fmt.Sprintf("%s#%d", project, iid)
	}
}

// parseIssueRef accepts ".../group/project/-/issues/123" or "group/project#123".
func parseIssueRef(s string) (string, int, bool) {
	var project, num string
	if i := strings.Index(s, "/-/issues/"); i >= 0 {
		base := s[:i]
		if u, err := url.Parse(base); err == nil && u.Host != "" {
			base = u.Path
		}
		project = strings.Trim(base, "/")
		num = strings.SplitN(s[i+len("/-/issues/"):], "#", 2)[0]
		num = strings.TrimRight(strings.SplitN(num, "?", 2)[0], "/")
	} else if i := strings.LastIndex(s, "#"); i > 0 {
		project, num = s[:i], s[i+1:]
	}
	iid, err := strconv.Atoi(num)
	if err != nil || iid <= 0 || !strings.Contains(project, "/") {
		return "", 0, false
	}
	return project, iid, true
}
