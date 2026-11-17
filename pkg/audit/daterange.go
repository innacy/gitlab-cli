package audit

import (
	"fmt"
	"time"
)

// DateRange turns a range name ("week", "month" with optional YYYY-MM) into
// inclusive local-date bounds. Events are stored with local dates, so "today"
// matches the user's wall clock rather than UTC.
func DateRange(name, arg string, now time.Time) (from, to, label string, err error) {
	day := func(t time.Time) string { return t.Format("2006-01-02") }
	switch name {
	case "today":
		return day(now), day(now), "Today", nil
	case "year":
		y := now.Year()
		if arg != "" {
			t, err := time.Parse("2006", arg)
			if err != nil {
				return "", "", "", fmt.Errorf("invalid year: %s (use YYYY)", arg)
			}
			y = t.Year()
		}
		return fmt.Sprintf("%d-01-01", y), fmt.Sprintf("%d-12-31", y), fmt.Sprint(y), nil
	case "week":
		return day(now.AddDate(0, 0, -6)), day(now), "Last 7 Days", nil
	case "month", "":
		if arg == "" {
			return day(now.AddDate(0, 0, -29)), day(now), "Last 30 Days", nil
		}
		m, err := time.ParseInLocation("2006-01", arg, now.Location())
		if err != nil {
			return "", "", "", fmt.Errorf("invalid date format: %s (use YYYY-MM)", arg)
		}
		return day(m), day(m.AddDate(0, 1, -1)), m.Format("January 2006"), nil
	default:
		return "", "", "", fmt.Errorf("unknown range %q (use today, week, month [YYYY-MM] or year [YYYY])", name)
	}
}
