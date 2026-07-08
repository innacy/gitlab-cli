package audit

import (
	"testing"
	"time"
)

func TestOpenInMemory(t *testing.T) {
	rec, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer rec.Close()
}

func TestRecordEvent(t *testing.T) {
	rec, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer rec.Close()

	id, err := rec.Record(Event{
		Command:    "mr-review",
		Category:   CategoryAIGeneration,
		Project:    "my-project",
		DurationMs: 1500,
		Success:    true,
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id <= 0 {
		t.Error("expected positive event ID")
	}
}

func TestRecordAIOutput(t *testing.T) {
	rec, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer rec.Close()

	eventID, _ := rec.Record(Event{
		Command:  "mr-review",
		Category: CategoryAIGeneration,
		Project:  "my-project",
		Success:  true,
	})

	err = rec.RecordAIOutput(AIOutput{
		EventID:   eventID,
		Provider:  "anthropic",
		Model:     "claude-sonnet-4",
		CharCount: 2500,
	})
	if err != nil {
		t.Fatalf("record AI output: %v", err)
	}
}

func TestRecordOutcome(t *testing.T) {
	rec, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer rec.Close()

	eventID, _ := rec.Record(Event{
		Command:  "mr-review",
		Category: CategoryAIGeneration,
		Project:  "my-project",
		Success:  true,
	})

	_ = rec.RecordAIOutput(AIOutput{
		EventID:   eventID,
		Provider:  "anthropic",
		Model:     "claude-sonnet-4",
		CharCount: 2500,
	})

	ratio := 0.15
	err = rec.RecordOutcome(eventID, OutcomeEdited, &ratio)
	if err != nil {
		t.Fatalf("record outcome: %v", err)
	}

	var outcome string
	var editRatio float64
	var decidedAt string
	err = rec.db.QueryRow(
		"SELECT outcome, edit_ratio, decided_at FROM ai_outputs WHERE event_id = ?", eventID,
	).Scan(&outcome, &editRatio, &decidedAt)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if outcome != OutcomeEdited {
		t.Errorf("got outcome %q, want %q", outcome, OutcomeEdited)
	}
	if editRatio != 0.15 {
		t.Errorf("got edit_ratio %f, want 0.15", editRatio)
	}
	parsed, _ := time.Parse(time.RFC3339, decidedAt)
	if time.Since(parsed) > 5*time.Second {
		t.Errorf("decided_at too old: %s", decidedAt)
	}
}
