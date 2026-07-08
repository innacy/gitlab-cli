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

	today := time.Now().UTC().Format("2006-01-02")
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

	today := time.Now().UTC().Format("2006-01-02")
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

	today := time.Now().UTC().Format("2006-01-02")
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

	today := time.Now().UTC().Format("2006-01-02")
	acc, err := rec.AccuracyStats(today, today)
	if err != nil {
		t.Fatalf("accuracy: %v", err)
	}
	if acc.Accepted != 3 {
		t.Errorf("accepted=%d want 3", acc.Accepted)
	}
}
