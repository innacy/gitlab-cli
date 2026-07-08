package audit

func EditRatio(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 && lb == 0 {
		return 0.0
	}

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
			ins := prev[j] + 1
			del := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = min(ins, min(del, sub))
		}
		prev, curr = curr, prev
	}

	maxLen := la
	if lb > maxLen {
		maxLen = lb
	}
	return float64(prev[lb]) / float64(maxLen)
}
