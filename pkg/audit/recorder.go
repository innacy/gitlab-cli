package audit

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const (
	CategoryAIGeneration   = "ai_generation"
	CategoryGitLabMutation = "gitlab_mutation"
	CategoryRead           = "read"

	// Side effects, recorded under the command that caused them. They are
	// not commands and are excluded from command counts.
	CategoryFileWrite     = "file_write"
	CategoryMRCreated     = "mr_created"
	CategoryTicketCreated = "ticket_created"

	OutcomeAccepted  = "accepted"
	OutcomeEdited    = "edited"
	OutcomeDiscarded = "discarded"
)

type Event struct {
	Command    string
	Category   string
	Project    string
	DurationMs int64
	Success    bool
	Metadata   map[string]interface{}
}

type AIOutput struct {
	EventID      int64
	Provider     string
	Model        string
	CharCount    int
	DurationMs   int64
	InputTokens  int
	OutputTokens int
}

type Recorder struct {
	db *sql.DB
}

func Open(dir string) (*Recorder, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create audit dir: %w", err)
	}
	dbPath := filepath.Join(dir, "audit.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open audit db: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate audit db: %w", err)
	}
	return &Recorder{db: db}, nil
}

func OpenInMemory() (*Recorder, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Recorder{db: db}, nil
}

func (r *Recorder) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}

func (r *Recorder) Record(evt Event) (int64, error) {
	now := time.Now()
	var metaJSON []byte
	if evt.Metadata != nil {
		metaJSON, _ = json.Marshal(evt.Metadata)
	}
	successInt := 0
	if evt.Success {
		successInt = 1
	}
	result, err := r.db.Exec(
		`INSERT INTO events (timestamp, date, command, category, project, duration_ms, success, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		now.UTC().Format(time.RFC3339), // timestamp: UTC, compared by SummarySince
		now.Format("2006-01-02"),       // date: local, so "today" matches the wall clock
		evt.Command,
		evt.Category,
		evt.Project,
		evt.DurationMs,
		successInt,
		string(metaJSON),
	)
	if err != nil {
		return 0, fmt.Errorf("insert event: %w", err)
	}
	return result.LastInsertId()
}

// Finish stores a command's final duration and outcome on the event that was
// recorded when it started.
func (r *Recorder) Finish(eventID, durationMs int64, success bool) error {
	_, err := r.db.Exec(`UPDATE events SET duration_ms = ?, success = ? WHERE id = ?`,
		durationMs, success, eventID)
	return err
}

func (r *Recorder) RecordAIOutput(out AIOutput) error {
	_, err := r.db.Exec(
		`INSERT INTO ai_outputs (event_id, provider, model, char_count, duration_ms, input_tokens, output_tokens)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		out.EventID, out.Provider, out.Model, out.CharCount, out.DurationMs, out.InputTokens, out.OutputTokens,
	)
	return err
}

func (r *Recorder) RecordOutcome(eventID int64, outcome string, editRatio *float64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(
		`UPDATE ai_outputs SET outcome = ?, edit_ratio = ?, decided_at = ? WHERE event_id = ?`,
		outcome, editRatio, now, eventID,
	)
	return err
}
