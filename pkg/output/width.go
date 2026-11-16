package output

import "github.com/mattn/go-runewidth"

// Width is the number of terminal columns s occupies (wide CJK = 2, "—" = 1).
func Width(s string) int { return runewidth.StringWidth(s) }

// Truncate shortens s to at most width columns, ending in "..." when cut.
// Unlike slicing bytes it never splits a multi-byte character.
func Truncate(s string, width int) string { return runewidth.Truncate(s, width, "...") }

// Cut shortens s to at most width columns without an ellipsis.
func Cut(s string, width int) string { return runewidth.Truncate(s, width, "") }

// StatusLabel renders a pipeline/job status as a coloured text symbol. Emoji
// were used before; they render two columns wide in some terminals, which
// broke table alignment, and ignored NO_COLOR.
func StatusLabel(status string) string {
	t := GetTheme()
	switch status {
	case "success":
		return t.Success.Sprint("✓ " + status)
	case "failed":
		return t.Error.Sprint("✗ " + status)
	case "running":
		return t.Warning.Sprint("● " + status)
	case "pending", "created", "waiting_for_resource", "preparing", "scheduled":
		return t.Warning.Sprint("○ " + status)
	case "canceled", "skipped", "manual":
		return t.Muted.Sprint("⊘ " + status)
	default:
		return "· " + status
	}
}
