package output

import (
	"testing"
	"time"
)

func TestFormatElapsed(t *testing.T) {
	cases := []struct {
		d       time.Duration
		precise bool
		want    string
	}{
		{3700 * time.Millisecond, false, "3s"},
		{4250 * time.Millisecond, true, "4.2s"},
		{65 * time.Second, true, "1m 5s"},
	}
	for _, c := range cases {
		if got := FormatElapsed(c.d, c.precise); got != c.want {
			t.Errorf("FormatElapsed(%v,%v) = %q, want %q", c.d, c.precise, got, c.want)
		}
	}
}

func TestProgressRender(t *testing.T) {
	p := NewProgress(4, "x")
	p.Inc()
	if got := p.render(); got != " [█████░░░░░░░░░░░░░░░] 1/4 x" {
		t.Errorf("render = %q", got)
	}
}
