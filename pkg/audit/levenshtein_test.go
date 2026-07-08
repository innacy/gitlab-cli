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
