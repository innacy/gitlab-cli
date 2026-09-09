package audit

import "database/sql"

const schemaSQL = `
CREATE TABLE IF NOT EXISTS events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp   TEXT    NOT NULL,
    date        TEXT    NOT NULL,
    command     TEXT    NOT NULL,
    category    TEXT    NOT NULL,
    project     TEXT,
    duration_ms INTEGER,
    success     INTEGER NOT NULL DEFAULT 1,
    metadata    TEXT
);

CREATE TABLE IF NOT EXISTS ai_outputs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id    INTEGER NOT NULL REFERENCES events(id),
    provider    TEXT    NOT NULL,
    model       TEXT    NOT NULL,
    char_count  INTEGER NOT NULL,
    duration_ms INTEGER,
    outcome     TEXT,
    edit_ratio  REAL,
    decided_at  TEXT
);

CREATE INDEX IF NOT EXISTS idx_events_date ON events(date);
CREATE INDEX IF NOT EXISTS idx_events_command ON events(command);
CREATE INDEX IF NOT EXISTS idx_ai_outputs_outcome ON ai_outputs(outcome);
`

func migrate(db *sql.DB) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return err
	}
	db.Exec("ALTER TABLE ai_outputs ADD COLUMN duration_ms INTEGER")
	return nil
}
