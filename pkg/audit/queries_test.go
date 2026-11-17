package audit

import (
	"testing"
	"time"
)

func seedEvents(t *testing.T, rec *Recorder) {
	t.Helper()
	cmds := []struct {
		cmd      string
		category string
		project  string
	}{
		{"mr-review", CategoryAIGeneration, "proj-a"},
		{"mr-review", CategoryAIGeneration, "proj-a"},
		{"mr-open", CategoryGitLabMutation, "proj-b"},
		{"ticket-open", CategoryGitLabMutation, "proj-a"},
		{"create-ticket-content", CategoryAIGeneration, "proj-b"},
	}
	for _, c := range cmds {
		id, err := rec.Record(Event{
			Command:    c.cmd,
			Category:   c.category,
			Project:    c.project,
			DurationMs: 1000,
			Success:    true,
		})
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		if c.category == CategoryAIGeneration {
			_ = rec.RecordAIOutput(AIOutput{
				EventID:   id,
				Provider:  "anthropic",
				Model:     "claude-sonnet-4",
				CharCount: 2000,
			})
			_ = rec.RecordOutcome(id, OutcomeAccepted, nil)
		}
	}
}

func TestSummary(t *testing.T) {
	rec, _ := OpenInMemory()
	defer rec.Close()
	seedEvents(t, rec)

	today := time.Now().Format("2006-01-02")
	s, err := rec.Summary(today, today)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if s.TotalEvents != 5 {
		t.Errorf("total=%d want 5", s.TotalEvents)
	}
	if s.AIGenerations != 3 {
		t.Errorf("ai=%d want 3", s.AIGenerations)
	}
}

func TestHeatmap(t *testing.T) {
	rec, _ := OpenInMemory()
	defer rec.Close()
	seedEvents(t, rec)

	today := time.Now().Format("2006-01-02")
	days, err := rec.Heatmap(today, today)
	if err != nil {
		t.Fatalf("heatmap: %v", err)
	}
	if len(days) != 1 {
		t.Fatalf("days=%d want 1", len(days))
	}
	if days[0].Count != 5 {
		t.Errorf("count=%d want 5", days[0].Count)
	}
}

func TestModelUsage(t *testing.T) {
	rec, _ := OpenInMemory()
	defer rec.Close()
	seedEvents(t, rec)

	today := time.Now().Format("2006-01-02")
	models, err := rec.ModelUsage(today, today)
	if err != nil {
		t.Fatalf("model usage: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("models=%d want 1", len(models))
	}
	if models[0].Model != "claude-sonnet-4" || models[0].Count != 3 {
		t.Errorf("got %+v", models[0])
	}
}

func TestAccuracyStats(t *testing.T) {
	rec, _ := OpenInMemory()
	defer rec.Close()
	seedEvents(t, rec)

	today := time.Now().Format("2006-01-02")
	acc, err := rec.AccuracyStats(today, today)
	if err != nil {
		t.Fatalf("accuracy: %v", err)
	}
	if acc.Accepted != 3 {
		t.Errorf("accepted=%d want 3", acc.Accepted)
	}
}

func TestSummarySeparatesCommandsFromEffectsAndCountsFailures(t *testing.T) {
	rec, _ := OpenInMemory()
	defer rec.Close()

	ok, _ := rec.Record(Event{Command: "mr-review", Category: CategoryAIGeneration, Success: true})
	_ = rec.Finish(ok, 1200, true)
	failed, _ := rec.Record(Event{Command: "mr-approve", Category: CategoryGitLabMutation, Success: true})
	_ = rec.Finish(failed, 50, false)
	rec.Record(Event{Command: "mr-comment", Category: CategoryGitLabMutation, Success: true})
	rec.Record(Event{Command: "mr-review", Category: CategoryFileWrite, Success: true})
	rec.Record(Event{Command: "mr-open", Category: CategoryMRCreated, Success: true})
	rec.Record(Event{Command: "ticket-open", Category: CategoryTicketCreated, Success: true})

	today := time.Now().Format("2006-01-02")
	s, err := rec.Summary(today, today)
	if err != nil {
		t.Fatal(err)
	}
	want := SummaryResult{TotalEvents: 3, Failed: 1, AIGenerations: 1, MRsCreated: 1,
		MRsReviewed: 1, ReviewsPosted: 1, TicketsCreated: 1, FilesWritten: 1}
	if *s != want {
		t.Errorf("summary = %+v\nwant      %+v", *s, want)
	}

	var dur int64
	var success bool
	rec.db.QueryRow("SELECT duration_ms, success FROM events WHERE id = ?", failed).Scan(&dur, &success)
	if dur != 50 || success {
		t.Errorf("Finish stored duration=%d success=%v, want 50 false", dur, success)
	}
}

func TestSummarySince(t *testing.T) {
	rec, _ := OpenInMemory()
	defer rec.Close()
	rec.db.Exec(`INSERT INTO events (timestamp, date, command, category, success)
		VALUES ('2000-01-01T00:00:00Z', '2000-01-01', 'mr-open', 'gitlab_mutation', 1)`)
	start := time.Now().Add(-time.Second)
	rec.Record(Event{Command: "mr-open", Category: CategoryGitLabMutation, Success: true})

	s, err := rec.SummarySince(start)
	if err != nil {
		t.Fatal(err)
	}
	if s.TotalEvents != 1 {
		t.Errorf("SummarySince counted %d events, want 1 (older row must be excluded)", s.TotalEvents)
	}
}
