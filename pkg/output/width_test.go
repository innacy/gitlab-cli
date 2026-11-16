package output

import (
	"strings"
	"testing"

	"github.com/fatih/color"
)

func TestTruncateRespectsRunes(t *testing.T) {
	s := strings.Repeat("ä", 50) // 2 bytes per rune
	got := Truncate(s, 10)
	if Width(got) != 10 || !strings.HasSuffix(got, "...") || !strings.HasPrefix(got, "äää") {
		t.Errorf("Truncate = %q (width %d)", got, Width(got))
	}
	if Truncate("short", 10) != "short" {
		t.Error("short string changed")
	}
	if got := Cut("Fix — löschen", 6); got != "Fix — " {
		t.Errorf("Cut = %q", got)
	}
}

func TestThemeBoxAlignsMultiByteLines(t *testing.T) {
	color.NoColor = true
	out := captureStdout(t, func() { ThemeBox([]string{"Activity — Today", "plain"}) })
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	want := Width(lines[0])
	for _, l := range lines {
		if Width(l) != want {
			t.Errorf("box line width %d != %d: %q", Width(l), want, l)
		}
	}
}

func TestStatusLabel(t *testing.T) {
	color.NoColor = true
	for status, want := range map[string]string{"success": "✓ success", "failed": "✗ failed", "running": "● running", "skipped": "⊘ skipped", "weird": "· weird"} {
		if got := StatusLabel(status); got != want {
			t.Errorf("StatusLabel(%q) = %q", status, got)
		}
	}
}
