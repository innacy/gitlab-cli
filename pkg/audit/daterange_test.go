package audit

import (
	"testing"
	"time"
)

func TestDateRange(t *testing.T) {
	ist := time.FixedZone("IST", 5*3600+1800)
	// 02:00 IST on Nov 16 is still Nov 15 in UTC: the range must use local dates.
	now := time.Date(2026, 11, 16, 2, 0, 0, 0, ist)

	cases := []struct {
		name, arg, from, to string
	}{
		{"week", "", "2026-11-10", "2026-11-16"}, // 7 days inclusive
		{"month", "", "2026-10-18", "2026-11-16"},
		{"", "", "2026-10-18", "2026-11-16"},
		{"month", "2026-02", "2026-02-01", "2026-02-28"},
		{"today", "", "2026-11-16", "2026-11-16"},
		{"year", "", "2026-01-01", "2026-12-31"},
		{"year", "2025", "2025-01-01", "2025-12-31"},
	}
	for _, c := range cases {
		from, to, _, err := DateRange(c.name, c.arg, now)
		if err != nil || from != c.from || to != c.to {
			t.Errorf("DateRange(%q,%q) = %s..%s, %v; want %s..%s", c.name, c.arg, from, to, err, c.from, c.to)
		}
	}
	for _, bad := range [][2]string{{"month", "2026-13"}, {"fortnight", ""}, {"year", "20x5"}} {
		if _, _, _, err := DateRange(bad[0], bad[1], now); err == nil {
			t.Errorf("DateRange(%q,%q) should fail", bad[0], bad[1])
		}
	}
}

func TestRecordUsesLocalDate(t *testing.T) {
	rec, _ := OpenInMemory()
	defer rec.Close()
	rec.Record(Event{Command: "list", Category: CategoryRead, Success: true})
	var date string
	rec.db.QueryRow("SELECT date FROM events").Scan(&date)
	if want := time.Now().Format("2006-01-02"); date != want {
		t.Errorf("stored date %s, want local %s", date, want)
	}
}
