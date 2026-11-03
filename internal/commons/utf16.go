package commons

import "unicode/utf16"

// Client text is UTF-16 on the wire, and the client counts its length
// limits in UTF-16 code units: a character outside the Basic Multilingual
// Plane counts two. The helpers below count, cut, and convert strings unit
// by unit. Any byte that is not valid UTF-8 counts and converts as U+FFFD,
// as the wire writer sends it, and so does an unpaired surrogate unit.

// UTF16Len reports how many UTF-16 code units s encodes to: the length the
// client counts in.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// UTF16Units returns s as UTF-16 code units.
func UTF16Units(s string) []uint16 {
	units := make([]uint16, 0, UTF16Len(s))
	for _, r := range s {
		units = utf16.AppendRune(units, r)
	}
	return units
}

// UTF16String converts UTF-16 code units back to a string: a valid
// surrogate pair as its supplementary character, an unpaired surrogate as
// U+FFFD.
func UTF16String(units []uint16) string {
	return string(utf16.Decode(units))
}

// UTF16Cut returns the first n UTF-16 code units of s, or s when it is no
// longer. A supplementary character straddling the cut leaves its leading
// unit, which reads as U+FFFD, so the result is still n units long.
func UTF16Cut(s string, n int) string {
	used := 0
	for i, r := range s {
		w := utf16.RuneLen(r)
		if used+w > n {
			if used < n {
				return s[:i] + "�"
			}
			return s[:i]
		}
		used += w
	}
	return s
}
