package commons

import (
	"sort"
	"unicode"
	"unicode/utf16"
)

// A capital sigma lowers to the final form ς when it ends a word: some
// character before it in its word is cased and none after it is. Words are
// found by a legacy rule-based word-boundary scan driven by the tables in
// wordbreak_tables.go, which differs from Unicode text segmentation
// (UAX #29). The scan is reproduced step by step, quirks included: a
// boundary at offset o is tested by backing up from o-1 to a position the
// backward table deems safe and scanning forward from there, so an offset
// inside a surrogate pair is scanned from its low half, and each sigma's
// test reuses its last break when that lies before o-1.

// endsWord reports whether the capital sigma at units[i] of w's text ends a
// word.
func (w *wordText) endsWord(i int) bool {
	units := w.units
	s := wordScan{w: w, last: scanDone}
	for at := i; at >= 0 && !s.isBoundary(at); {
		r := codePointBefore(units, at)
		if isCased(r) {
			for at = i + 1; at < len(units) && !s.isBoundary(at); {
				r = codePointAt(units, at)
				if isCased(r) {
					return false
				}
				at += unitsOf(r)
			}
			return true
		}
		at -= unitsOf(r)
	}
	return false
}

// isCased reports whether r is cased: Lu, Ll, or Lt in the scan's letter
// category, or one of a fixed list of other lowercase and uppercase
// characters. Case is read as of the Unicode version of the scan's tables:
// the letter category leaves out letters encoded later, and U+0295 counts
// as the lowercase letter it was then.
func isCased(r rune) bool {
	switch {
	case r == 0x0295, 0x02B0 <= r && r <= 0x02B8, r == 0x02C0, r == 0x02C1, 0x02E0 <= r && r <= 0x02E4,
		r == 0x0345, r == 0x037A, 0x1D2C <= r && r <= 0x1D61,
		0x2160 <= r && r <= 0x217F, 0x24B6 <= r && r <= 0x24E9:
		return true
	}
	return wordCategory(r) == wordLetter && unicode.In(r, unicode.Lu, unicode.Ll, unicode.Lt)
}

const (
	wordSkip   = -1 // characters a scan steps over without a transition
	wordLetter = 4  // letters other than ideographs and kana
	// wordDone is what a scan reads past either end of the text. A U+FFFF
	// unit reads the same, so it ends a scan too.
	wordDone = 0xFFFF
	scanDone = -1 // no break: the scan stood at the end of the text
)

// wordCategory returns r's column in the scan's state tables.
func wordCategory(r rune) int {
	key := uint32(r)<<8 | 0xFF
	i := sort.Search(len(wordBreakRanges), func(i int) bool { return wordBreakRanges[i] > key }) - 1
	return int(wordBreakRanges[i]&0xFF) - 1
}

// wordText is a text under boundary scans, as UTF-16 units. Forward and
// backward scan results depend only on the text and the start position,
// so they are kept for every scan of the text to share: each sigma's test
// then costs time in proportion to the span it inspects, not the text.
type wordText struct {
	units []uint16
	next  map[int]int     // forward scan result by start position
	back  map[backKey]int // backward scan result by loop state
}

// backKey is the state of a backward scan between two steps: the position
// and code point it stands on (a surrogate pair or a single unit), its
// table state, and whether the code point it last stepped over was skipped.
type backKey struct {
	pos         int
	pair        bool
	state       int8
	skippedLast bool
}

func newWordText(units []uint16) *wordText {
	return &wordText{units: units, next: map[int]int{}, back: map[backKey]int{}}
}

// wordScan is one sigma's sequence of boundary tests over a text. last is
// the last break a following call returned, or scanDone.
type wordScan struct {
	w    *wordText
	last int
}

func (s *wordScan) isBoundary(offset int) bool {
	if offset == 0 {
		return true
	}
	return s.following(offset-1) == offset
}

// following returns the first break after offset.
func (s *wordScan) following(offset int) int {
	if offset == 0 {
		s.last = s.w.scanForward(0)
		return s.last
	}
	result := s.last
	if result >= offset || result <= scanDone {
		result = s.w.scanBack(offset)
	}
	for result != scanDone && result <= offset {
		result = s.w.scanForward(result)
	}
	s.last = result
	return result
}

// scanForward returns the next break after start, or scanDone at the end.
func (w *wordText) scanForward(start int) int {
	if start == len(w.units) {
		return scanDone
	}
	if r, ok := w.next[start]; ok {
		return r
	}
	pos := start
	result := w.nextIndex(pos)
	state := 1
	for c := w.current(pos); c != wordDone && state != 0; {
		if cat := wordCategory(c); cat != wordSkip {
			state = int(wordBreakForward[state*wordBreakColumns+cat])
		}
		if wordBreakAccepting[state] {
			result = w.nextIndex(pos)
		}
		pos, c = w.advance(pos)
	}
	w.next[start] = result
	return result
}

// scanBack returns the position a backward scan from start settles on: a
// break whatever precedes it.
func (w *wordText) scanBack(start int) int {
	pos, c := start, w.current(start)
	state, cat, lastCat := 1, 0, 0
	var path []backKey
	result := -1
	for c != wordDone && state != 0 {
		lastCat, cat = cat, wordCategory(c)
		if cat != wordSkip {
			state = int(wordBreakBackward[state*wordBreakColumns+cat])
		}
		pos, c = w.retreat(pos)
		if c == wordDone || state == 0 {
			break
		}
		key := backKey{pos: pos, pair: c >= 0x10000, state: int8(state), skippedLast: cat == wordSkip}
		if r, ok := w.back[key]; ok {
			result = r
			break
		}
		path = append(path, key)
	}
	if result < 0 {
		result = pos
		if c != wordDone {
			result, _ = w.advance(result)
			if lastCat != wordSkip {
				result, _ = w.advance(result)
			}
		}
	}
	for _, key := range path {
		w.back[key] = result
	}
	return result
}

// current returns the code point at pos: a surrogate pair as one, an
// unpaired surrogate as its unit, wordDone at the end of text.
func (w *wordText) current(pos int) rune {
	if pos >= len(w.units) {
		return wordDone
	}
	return codePointAt(w.units, pos)
}

func (w *wordText) nextIndex(pos int) int {
	return min(pos+unitsOf(w.current(pos)), len(w.units))
}

// advance steps past the code point at pos and returns the new position
// and the code point there; with no code point after it, pos stays and the
// code point is wordDone.
func (w *wordText) advance(pos int) (int, rune) {
	if pos == len(w.units) {
		return pos, wordDone
	}
	i := pos + unitsOf(w.current(pos))
	if i >= len(w.units) {
		return pos, wordDone
	}
	return i, w.current(i)
}

// retreat steps back over the code point before pos, a surrogate pair as
// one, and returns the new position and that code point; at the start, pos
// stays and the code point is wordDone.
func (w *wordText) retreat(pos int) (int, rune) {
	if pos == 0 {
		return pos, wordDone
	}
	r := codePointBefore(w.units, pos)
	return pos - unitsOf(r), r
}

// codePointAt returns the code point starting at units[i], a surrogate
// pair as one.
func codePointAt(units []uint16, i int) rune {
	c := rune(units[i])
	if utf16.IsSurrogate(c) && c < 0xDC00 && i+1 < len(units) {
		if low := rune(units[i+1]); utf16.IsSurrogate(low) && low >= 0xDC00 {
			return utf16.DecodeRune(c, low)
		}
	}
	return c
}

// codePointBefore returns the code point ending at units[i-1], a surrogate
// pair as one.
func codePointBefore(units []uint16, i int) rune {
	c := rune(units[i-1])
	if utf16.IsSurrogate(c) && c >= 0xDC00 && i >= 2 {
		if high := rune(units[i-2]); utf16.IsSurrogate(high) && high < 0xDC00 {
			return utf16.DecodeRune(high, c)
		}
	}
	return c
}
