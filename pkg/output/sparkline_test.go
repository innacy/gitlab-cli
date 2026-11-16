package output

import "testing"

func TestSparkline(t *testing.T) {
	if got := Sparkline([]int{0, 1, 4, 8}, 0); got != " ▂▅█" {
		t.Errorf("Sparkline = %q", got)
	}
	if got := Sparkline([]int{1, 1, 1, 1, 0, 0}, 3); got != "██ " {
		t.Errorf("bucketed Sparkline = %q", got)
	}
	if got := Sparkline([]int{0, 0}, 0); got != "  " {
		t.Errorf("all-zero Sparkline = %q", got)
	}
}
