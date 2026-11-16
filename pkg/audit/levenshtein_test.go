package audit

import "testing"

func TestEditRatio(t *testing.T) {
	tests := []struct {
		a, b string
		want float64
	}{
		{"hello", "hello", 0.0},
		{"", "", 0.0},
		{"abc", "", 1.0},
		{"", "abc", 1.0},
		{"kitten", "sitting", 0.4285714285714286},
	}
	for _, tt := range tests {
		got := EditRatio(tt.a, tt.b)
		if got < tt.want-0.001 || got > tt.want+0.001 {
			t.Errorf("EditRatio(%q, %q) = %f, want %f", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestDistance(t *testing.T) {
	for _, c := range []struct {
		a, b string
		d    int
	}{{"", "", 0}, {"abc", "abc", 0}, {"mr-staus", "mr-status", 1}, {"kitten", "sitting", 3}, {"ä", "a", 1}, {"lsit", "list", 1}, {"ab", "ba", 1}} {
		if got := Distance(c.a, c.b); got != c.d {
			t.Errorf("Distance(%q,%q) = %d, want %d", c.a, c.b, got, c.d)
		}
	}
}
