package output

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/briandowns/spinner"
)

// workNanos accumulates time spent inside loaders since the last TakeWorkTime.
var workNanos atomic.Int64

// NewLoader returns the shared spinner used while waiting on API calls. Like
// Claude's spinner it appends the live elapsed time: "⠋ Fetching... (3s)".
// It is a no-op when stdout is not a terminal.
func NewLoader(suffix string) *spinner.Spinner {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = suffix

	var base, rendered string
	var started, lastTick time.Time
	// PreUpdate runs under the spinner lock on every tick. Callers may change
	// Suffix between ticks (or reuse the spinner across Start/Stop), so a
	// changed Suffix becomes the new base and a long gap starts a new run.
	s.PreUpdate = func(s *spinner.Spinner) {
		now := time.Now()
		if lastTick.IsZero() || now.Sub(lastTick) > 500*time.Millisecond {
			started = now
		} else {
			workNanos.Add(int64(now.Sub(lastTick)))
		}
		lastTick = now
		if s.Suffix != rendered {
			base = s.Suffix
		}
		rendered = base + GetTheme().Muted.Sprintf(" (%s)", FormatElapsed(now.Sub(started), false))
		s.Suffix = rendered
	}
	return s
}

// TakeWorkTime returns loader time accumulated since the previous call and resets it.
func TakeWorkTime() time.Duration {
	return time.Duration(workNanos.Swap(0))
}

// PrintWorked prints the Claude-style per-command footer: "✻ Worked for 4.2s".
func PrintWorked(d time.Duration) {
	GetTheme().Muted.Printf("  ✻ Worked for %s\n", FormatElapsed(d, true))
}

// FormatElapsed renders 3s / 4.2s (precise) / 1m 5s.
func FormatElapsed(d time.Duration, precise bool) string {
	if d >= time.Minute {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if precise {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

// Progress is a loader that renders a "[████░░░░] 3/8 msg" bar for a known
// number of steps. Inc is safe to call from multiple goroutines.
type Progress struct {
	s     *spinner.Spinner
	msg   string
	total int
	done  int
}

const progressWidth = 20

func NewProgress(total int, msg string) *Progress {
	p := &Progress{s: NewLoader(""), msg: msg, total: total}
	p.s.Suffix = p.render()
	return p
}

func (p *Progress) Start() { p.s.Start() }
func (p *Progress) Stop()  { p.s.Stop() }

func (p *Progress) Inc() {
	p.s.Lock()
	p.done++
	p.s.Suffix = p.render()
	p.s.Unlock()
}

func (p *Progress) render() string {
	filled := 0
	if p.total > 0 {
		filled = min(p.done*progressWidth/p.total, progressWidth)
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", progressWidth-filled)
	return fmt.Sprintf(" [%s] %d/%d %s", bar, p.done, p.total, p.msg)
}
