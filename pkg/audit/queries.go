package audit

import "database/sql"

type SummaryResult struct {
	TotalEvents    int
	AIGenerations  int
	MRsCreated     int
	MRsReviewed    int
	TicketsCreated int
	FilesWritten   int
}

type HeatmapDay struct {
	Date  string
	Count int
}

type ModelUsageRow struct {
	Model string
	Count int
}

type AccuracyResult struct {
	Accepted     int
	Edited       int
	Discarded    int
	AvgEditRatio float64
}

func (r *Recorder) Summary(from, to string) (*SummaryResult, error) {
	s := &SummaryResult{}
	err := r.db.QueryRow(
		"SELECT COUNT(*) FROM events WHERE date >= ? AND date <= ?", from, to,
	).Scan(&s.TotalEvents)
	if err != nil {
		return nil, err
	}

	_ = r.db.QueryRow(
		"SELECT COUNT(*) FROM events WHERE date >= ? AND date <= ? AND category = ?",
		from, to, CategoryAIGeneration,
	).Scan(&s.AIGenerations)

	_ = r.db.QueryRow(
		"SELECT COUNT(*) FROM events WHERE date >= ? AND date <= ? AND command = 'mr-open'",
		from, to,
	).Scan(&s.MRsCreated)

	_ = r.db.QueryRow(
		"SELECT COUNT(*) FROM events WHERE date >= ? AND date <= ? AND command = 'mr-review'",
		from, to,
	).Scan(&s.MRsReviewed)

	_ = r.db.QueryRow(
		"SELECT COUNT(*) FROM events WHERE date >= ? AND date <= ? AND command = 'ticket-open'",
		from, to,
	).Scan(&s.TicketsCreated)

	_ = r.db.QueryRow(
		"SELECT COUNT(*) FROM events WHERE date >= ? AND date <= ? AND category = ?",
		from, to, CategoryFileWrite,
	).Scan(&s.FilesWritten)

	return s, nil
}

func (r *Recorder) Heatmap(from, to string) ([]HeatmapDay, error) {
	rows, err := r.db.Query(
		"SELECT date, COUNT(*) FROM events WHERE date >= ? AND date <= ? GROUP BY date ORDER BY date",
		from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var days []HeatmapDay
	for rows.Next() {
		var d HeatmapDay
		if err := rows.Scan(&d.Date, &d.Count); err != nil {
			return nil, err
		}
		days = append(days, d)
	}
	return days, rows.Err()
}

func (r *Recorder) ModelUsage(from, to string) ([]ModelUsageRow, error) {
	rows, err := r.db.Query(
		`SELECT a.model, COUNT(*)
		 FROM ai_outputs a
		 JOIN events e ON a.event_id = e.id
		 WHERE e.date >= ? AND e.date <= ?
		 GROUP BY a.model
		 ORDER BY COUNT(*) DESC`,
		from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []ModelUsageRow
	for rows.Next() {
		var m ModelUsageRow
		if err := rows.Scan(&m.Model, &m.Count); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

func (r *Recorder) AccuracyStats(from, to string) (*AccuracyResult, error) {
	a := &AccuracyResult{}

	_ = r.db.QueryRow(
		`SELECT COUNT(*) FROM ai_outputs a JOIN events e ON a.event_id = e.id
		 WHERE e.date >= ? AND e.date <= ? AND a.outcome = ?`,
		from, to, OutcomeAccepted,
	).Scan(&a.Accepted)

	_ = r.db.QueryRow(
		`SELECT COUNT(*) FROM ai_outputs a JOIN events e ON a.event_id = e.id
		 WHERE e.date >= ? AND e.date <= ? AND a.outcome = ?`,
		from, to, OutcomeEdited,
	).Scan(&a.Edited)

	_ = r.db.QueryRow(
		`SELECT COUNT(*) FROM ai_outputs a JOIN events e ON a.event_id = e.id
		 WHERE e.date >= ? AND e.date <= ? AND a.outcome = ?`,
		from, to, OutcomeDiscarded,
	).Scan(&a.Discarded)

	var avgRatio sql.NullFloat64
	_ = r.db.QueryRow(
		`SELECT AVG(a.edit_ratio) FROM ai_outputs a JOIN events e ON a.event_id = e.id
		 WHERE e.date >= ? AND e.date <= ? AND a.edit_ratio IS NOT NULL`,
		from, to,
	).Scan(&avgRatio)
	if avgRatio.Valid {
		a.AvgEditRatio = avgRatio.Float64
	}

	return a, nil
}

func (r *Recorder) BusiestDays(from, to string, limit int) ([]HeatmapDay, error) {
	rows, err := r.db.Query(
		"SELECT date, COUNT(*) as cnt FROM events WHERE date >= ? AND date <= ? GROUP BY date ORDER BY cnt DESC LIMIT ?",
		from, to, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var days []HeatmapDay
	for rows.Next() {
		var d HeatmapDay
		if err := rows.Scan(&d.Date, &d.Count); err != nil {
			return nil, err
		}
		days = append(days, d)
	}
	return days, rows.Err()
}
