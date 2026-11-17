package output

import "strings"

const sparkLevels = "▁▂▃▄▅▆▇█"

// Sparkline renders counts as a one-line bar chart. More than width values are
// summed into width buckets so a year still fits on one line.
func Sparkline(counts []int, width int) string {
	if width > 0 && len(counts) > width {
		size := (len(counts) + width - 1) / width
		var buckets []int
		for i := 0; i < len(counts); i += size {
			sum := 0
			for _, c := range counts[i:min(i+size, len(counts))] {
				sum += c
			}
			buckets = append(buckets, sum)
		}
		counts = buckets
	}
	peak := 0
	for _, c := range counts {
		peak = max(peak, c)
	}
	levels := []rune(sparkLevels)
	var sb strings.Builder
	for _, c := range counts {
		if peak == 0 || c == 0 {
			sb.WriteRune(' ')
			continue
		}
		sb.WriteRune(levels[(c*(len(levels)-1)+peak-1)/peak])
	}
	return sb.String()
}
