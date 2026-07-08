package cmd

import (
	"fmt"
	"strings"
	"time"

	"gitlab-ai/pkg/audit"
	"gitlab-ai/pkg/output"
)

func (r *replState) handleStats(args []string) {
	if r.auditor == nil {
		output.PrintWarning("Audit trail not available. Run a command first to initialize it.")
		return
	}

	subCmd := "month"
	dateArg := ""
	if len(args) > 0 {
		subCmd = strings.ToLower(args[0])
	}
	if len(args) > 1 {
		dateArg = args[1]
	}

	switch subCmd {
	case "dashboard":
		r.handleStatsDashboard()
		return
	}

	var from, to string
	now := time.Now().UTC()
	label := ""

	switch subCmd {
	case "week":
		from = now.AddDate(0, 0, -7).Format("2006-01-02")
		to = now.Format("2006-01-02")
		label = "Last 7 Days"
	case "month":
		if dateArg != "" {
			parsed, err := time.Parse("2006-01", dateArg)
			if err != nil {
				output.PrintError(fmt.Sprintf("Invalid date format: %s (use YYYY-MM)", dateArg))
				return
			}
			from = parsed.Format("2006-01-02")
			to = parsed.AddDate(0, 1, -1).Format("2006-01-02")
			label = parsed.Format("January 2006")
		} else {
			from = now.AddDate(0, 0, -30).Format("2006-01-02")
			to = now.Format("2006-01-02")
			label = "Last 30 Days"
		}
	default:
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		to = now.Format("2006-01-02")
		label = now.Format("January 2006")
	}

	summary, err := r.auditor.Summary(from, to)
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to load stats: %v", err))
		return
	}

	models, _ := r.auditor.ModelUsage(from, to)
	accuracy, _ := r.auditor.AccuracyStats(from, to)
	busiest, _ := r.auditor.BusiestDays(from, to, 3)

	lines := []string{
		fmt.Sprintf("Activity — %s", label),
		"",
		fmt.Sprintf("Commands run:     %d", summary.TotalEvents),
		fmt.Sprintf("AI generations:   %d", summary.AIGenerations),
		fmt.Sprintf("MRs created:      %d", summary.MRsCreated),
		fmt.Sprintf("MRs reviewed:     %d", summary.MRsReviewed),
		fmt.Sprintf("Tickets created:  %d", summary.TicketsCreated),
		fmt.Sprintf("Files written:    %d", summary.FilesWritten),
	}

	if len(models) > 0 {
		lines = append(lines, "", "AI Model Usage:")
		total := 0
		for _, m := range models {
			total += m.Count
		}
		for _, m := range models {
			pct := 0
			if total > 0 {
				pct = m.Count * 100 / total
			}
			lines = append(lines, fmt.Sprintf("  %-24s %d (%d%%)", m.Model+":", m.Count, pct))
		}
	}

	if accuracy != nil {
		totalAI := accuracy.Accepted + accuracy.Edited + accuracy.Discarded
		if totalAI > 0 {
			lines = append(lines, "", "AI Output Quality:")
			lines = append(lines, fmt.Sprintf("  Accepted as-is:  %d (%d%%)",
				accuracy.Accepted, accuracy.Accepted*100/totalAI))
			editLine := fmt.Sprintf("  Edited:          %d (%d%%)",
				accuracy.Edited, accuracy.Edited*100/totalAI)
			if accuracy.AvgEditRatio > 0 {
				editLine += fmt.Sprintf("  avg %.0f%%", accuracy.AvgEditRatio*100)
			}
			lines = append(lines, editLine)
			lines = append(lines, fmt.Sprintf("  Discarded:       %d (%d%%)",
				accuracy.Discarded, accuracy.Discarded*100/totalAI))
		}
	}

	if len(busiest) > 0 {
		lines = append(lines, "", "Busiest Days:")
		for _, d := range busiest {
			t, _ := time.Parse("2006-01-02", d.Date)
			lines = append(lines, fmt.Sprintf("  %s:  %d actions", t.Format("Mon Jan 02"), d.Count))
		}
	}

	fmt.Println()
	output.ThemeHeader("Stats")
	output.ThemeBox(lines)
	fmt.Println()
}

func (r *replState) handleStatsDashboard() {
	if r.auditor == nil {
		output.PrintWarning("Audit trail not available.")
		return
	}

	srv := audit.NewServer(r.auditor)

	addr, shutdown, err := srv.Start(func(url string) {
		output.PrintURLOpen(url)
	})
	if err != nil {
		output.PrintError(fmt.Sprintf("Dashboard server error: %v", err))
		return
	}

	fmt.Println()
	output.PrintSuccess(fmt.Sprintf("Dashboard running at %s", addr))
	fmt.Println()
	output.GetTheme().Muted.Println("  Press Enter to stop the dashboard and return to the REPL.")
	fmt.Println()

	r.rl.Readline()
	r.resetIdle()
	shutdown()
	output.PrintSuccess("Dashboard stopped.")
	fmt.Println()
}
