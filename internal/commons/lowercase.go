package commons

import "sort"

// lowerRune returns r's simple lower case under Unicode 15.0, the version
// clients' text is matched by, whatever version the toolchain's unicode
// tables follow. A code point without one, or outside the code space, is
// returned as it is.
func lowerRune(r rune) rune {
	if r < 0x80 {
		if 'A' <= r && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}
	i := sort.Search(len(lowerRuns), func(i int) bool { return lowerRuns[i].last >= r })
	if i == len(lowerRuns) {
		return r
	}
	run := lowerRuns[i]
	if r < run.first || (r-run.first)%rune(run.step) != 0 {
		return r
	}
	return r + run.delta
}
