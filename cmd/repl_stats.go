package cmd

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
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
	case "export":
		r.handleStatsExport(args[1:])
		return
	}

	from, to, label, err := audit.DateRange(subCmd, dateArg, time.Now())
	if err != nil {
		output.PrintError(err.Error())
		return
	}

	summary, err := r.auditor.Summary(from, to)
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to load stats: %v", err))
		return
	}

	models, _ := r.auditor.ModelUsage(from, to)
	accuracy, _ := r.auditor.AccuracyStats(from, to)
	busiest, _ := r.auditor.BusiestDays(from, to, 3)
	modelByDate, _ := r.auditor.ModelUsageByDate(from, to)
	modelPerf, _ := r.auditor.ModelPerformance(from, to)

	lines := []string{
		fmt.Sprintf("Activity — %s", label),
		"",
		fmt.Sprintf("Commands run:     %d%s", summary.TotalEvents, successRate(summary)),
		fmt.Sprintf("AI generations:   %d", summary.AIGenerations),
		fmt.Sprintf("MRs created:      %d", summary.MRsCreated),
		fmt.Sprintf("MRs reviewed:     %d", summary.MRsReviewed),
		fmt.Sprintf("Tickets created:  %d", summary.TicketsCreated),
		fmt.Sprintf("Files written:    %d", summary.FilesWritten),
	}
	if summary.MRsReviewed > 0 {
		lines = append(lines, fmt.Sprintf("Reviews posted:   %d of %d (%d%%)",
			summary.ReviewsPosted, summary.MRsReviewed, summary.ReviewsPosted*100/summary.MRsReviewed))
	}
	if days, err := r.auditor.DailyCounts(from, to); err == nil && len(days) > 1 {
		lines = append(lines, "", "Trend:  "+output.Sparkline(days, 52))
	}
	if cmds, _ := r.auditor.CommandBreakdown(from, to); len(cmds) > 0 {
		lines = append(lines, "", "Top Commands:")
		for _, c := range cmds[:min(len(cmds), 8)] {
			line := fmt.Sprintf("  %-22s %4d", c.Command, c.Count)
			if c.Failed > 0 {
				line += fmt.Sprintf("  (%d failed)", c.Failed)
			}
			lines = append(lines, line)
		}
	}
	if projects, _ := r.auditor.ProjectBreakdown(from, to, 5); len(projects) > 0 {
		lines = append(lines, "", "Top Projects:")
		for _, p := range projects {
			lines = append(lines, fmt.Sprintf("  %-30s %4d", p.Project, p.Count))
		}
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

	if len(modelPerf) > 0 {
		lines = append(lines, "", "Model Performance:")
		lines = append(lines, fmt.Sprintf("  %-28s %6s  %6s  %6s  %6s  %9s  %9s", "Model", "Calls", "Avg", "Min", "Max", "Tokens in", "Tokens out"))
		for _, m := range modelPerf {
			lines = append(lines, fmt.Sprintf("  %-28s %6d  %5.1fs  %5.1fs  %5.1fs  %9d  %9d",
				m.Model, m.Count,
				float64(m.AvgMs)/1000, float64(m.MinMs)/1000, float64(m.MaxMs)/1000,
				m.InputTokens, m.OutputTokens))
		}
		lines = append(lines, fmt.Sprintf("  Fastest: %s (avg %.1fs)", modelPerf[0].Model, float64(modelPerf[0].AvgMs)/1000))
	}

	if len(modelByDate) > 0 {
		lines = append(lines, "", "Model Usage by Date:")
		lastDate := ""
		for _, m := range modelByDate {
			if m.Date != lastDate {
				t, _ := time.Parse("2006-01-02", m.Date)
				lines = append(lines, fmt.Sprintf("  %s:", t.Format("Mon Jan 02")))
				lastDate = m.Date
			}
			lines = append(lines, fmt.Sprintf("    %-24s %d call(s)", m.Model, m.Count))
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

	r.rl.SetPrompt("")
	r.rl.Readline()
	r.rl.SetPrompt(r.prompt())
	r.resetIdle()
	shutdown()
	output.PrintSuccess("Dashboard stopped.")
	fmt.Println()
}

func successRate(s *audit.SummaryResult) string {
	if s.TotalEvents == 0 {
		return ""
	}
	return fmt.Sprintf("  (%d failed, %d%% success)", s.Failed, (s.TotalEvents-s.Failed)*100/s.TotalEvents)
}

// handleStatsExport writes the raw events of a range to a CSV or JSON file:
// stats export csv|json [today|week|month [YYYY-MM]|year [YYYY]]
func (r *replState) handleStatsExport(args []string) {
	if len(args) == 0 || (args[0] != "csv" && args[0] != "json") {
		output.PrintError("Usage: stats export csv|json [today|week|month [YYYY-MM]|year [YYYY]]")
		return
	}
	format := args[0]
	rangeName, rangeArg := "month", ""
	if len(args) > 1 {
		rangeName = strings.ToLower(args[1])
	}
	if len(args) > 2 {
		rangeArg = args[2]
	}
	from, to, _, err := audit.DateRange(rangeName, rangeArg, time.Now())
	if err != nil {
		output.PrintError(err.Error())
		return
	}
	events, err := r.auditor.Events(from, to)
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to load events: %v", err))
		return
	}

	var buf bytes.Buffer
	if format == "json" {
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		if events == nil {
			events = []audit.EventRow{}
		}
		err = enc.Encode(events)
	} else {
		w := csv.NewWriter(&buf)
		w.Write([]string{"timestamp", "command", "category", "project", "duration_ms", "success"})
		for _, e := range events {
			w.Write([]string{e.Timestamp, e.Command, e.Category, e.Project,
				strconv.FormatInt(e.DurationMs, 10), strconv.FormatBool(e.Success)})
		}
		w.Flush()
		err = w.Error()
	}
	if err != nil {
		output.PrintError(fmt.Sprintf("Failed to encode events: %v", err))
		return
	}

	filename := filepath.Join(r.cfg.Other.Directory, fmt.Sprintf("stats_%s_%s.%s", from, to, format))
	if err := writeFile(filename, buf.String()); err != nil {
		output.PrintError(fmt.Sprintf("Failed to write export: %v", err))
		return
	}
	output.PrintSuccess(fmt.Sprintf("Exported %d events (%s → %s)", len(events), from, to))
	output.PrintFilePath(filename)
}
