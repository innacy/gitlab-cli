package audit

import "time"

// CommandRow is one command's run and failure counts.
type CommandRow struct {
	Command string
	Count   int
	Failed  int
	AvgMs   int64
}

// ProjectRow is one project's command count.
type ProjectRow struct {
	Project string
	Count   int
}

// EventRow is a raw event, for export.
type EventRow struct {
	Timestamp  string `json:"timestamp"`
	Command    string `json:"command"`
	Category   string `json:"category"`
	Project    string `json:"project"`
	DurationMs int64  `json:"duration_ms"`
	Success    bool   `json:"success"`
}

// CommandBreakdown counts commands (not side effects) per command name,
// busiest first.
func (r *Recorder) CommandBreakdown(from, to string) ([]CommandRow, error) {
	rows, err := r.db.Query(`SELECT command, COUNT(*), COUNT(CASE WHEN success = 0 THEN 1 END),
		COALESCE(AVG(duration_ms), 0)
		FROM events WHERE date >= ? AND date <= ? AND `+isCommand+`
		GROUP BY command ORDER BY COUNT(*) DESC, command`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommandRow
	for rows.Next() {
		var c CommandRow
		var avg float64
		if err := rows.Scan(&c.Command, &c.Count, &c.Failed, &avg); err != nil {
			return nil, err
		}
		c.AvgMs = int64(avg)
		out = append(out, c)
	}
	return out, rows.Err()
}

// ProjectBreakdown returns the most active projects.
func (r *Recorder) ProjectBreakdown(from, to string, limit int) ([]ProjectRow, error) {
	rows, err := r.db.Query(`SELECT project, COUNT(*) FROM events
		WHERE date >= ? AND date <= ? AND `+isCommand+` AND COALESCE(project, '') <> ''
		GROUP BY project ORDER BY COUNT(*) DESC, project LIMIT ?`, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProjectRow
	for rows.Next() {
		var p ProjectRow
		if err := rows.Scan(&p.Project, &p.Count); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Events returns every event in the range, oldest first.
func (r *Recorder) Events(from, to string) ([]EventRow, error) {
	rows, err := r.db.Query(`SELECT timestamp, command, category, COALESCE(project, ''),
		COALESCE(duration_ms, 0), success FROM events
		WHERE date >= ? AND date <= ? ORDER BY id`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EventRow
	for rows.Next() {
		var e EventRow
		if err := rows.Scan(&e.Timestamp, &e.Command, &e.Category, &e.Project, &e.DurationMs, &e.Success); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DailyCounts returns one command count per day from..to inclusive,
// including zero days.
func (r *Recorder) DailyCounts(from, to string) ([]int, error) {
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return nil, err
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(`SELECT date, COUNT(*) FROM events
		WHERE date >= ? AND date <= ? AND `+isCommand+` GROUP BY date`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDate := map[string]int{}
	for rows.Next() {
		var d string
		var n int
		if err := rows.Scan(&d, &n); err != nil {
			return nil, err
		}
		byDate[d] = n
	}
	var out []int
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		out = append(out, byDate[d.Format("2006-01-02")])
	}
	return out, rows.Err()
}
