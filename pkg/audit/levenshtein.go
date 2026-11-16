package audit

// EditRatio is the edit distance normalised by the longer string's length.
func EditRatio(a, b string) float64 {
	la, lb := len([]rune(a)), len([]rune(b))
	if la == 0 && lb == 0 {
		return 0.0
	}
	return float64(Distance(a, b)) / float64(max(la, lb))
}

// Distance is the edit distance between a and b, counting insertions,
// deletions, substitutions and swaps of adjacent characters ("lsit" → "list"
// is 1), i.e. optimal string alignment.
func Distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)

	// pp, prev, curr are rows i-2, i-1, i.
	pp := make([]int, lb+1)
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				curr[j] = min(curr[j], pp[j-2]+1)
			}
		}
		pp, prev, curr = prev, curr, pp
	}

	return prev[lb]
}
