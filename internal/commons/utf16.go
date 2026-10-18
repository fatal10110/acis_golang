package commons

import (
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// Client text is UTF-16 on the wire, and a client may send an unpaired
// surrogate code unit. Such a unit is held in a Go string in its three-byte
// generalized UTF-8 form (ED A0..BF 80..BF, as CESU-8 and WTF-8 write it),
// so it survives until the text is written back to a client. The helpers
// below count, cut, and convert strings unit by unit with that form in mind.

// UTF16Len reports how many UTF-16 code units s encodes to: the length the
// client counts in. An unpaired surrogate is one unit; any other byte that
// is not valid UTF-8 counts one unit, as it is written as U+FFFD.
func UTF16Len(s string) int {
	n := 0
	for i := 0; i < len(s); {
		r, size := decodeUnitRune(s[i:])
		n += unitsOf(r)
		i += size
	}
	return n
}

// UTF16Units returns s as UTF-16 code units, an unpaired surrogate as its
// own unit.
func UTF16Units(s string) []uint16 {
	units := make([]uint16, 0, UTF16Len(s))
	for i := 0; i < len(s); {
		r, size := decodeUnitRune(s[i:])
		if r >= 0x10000 {
			high, low := utf16.EncodeRune(r)
			units = append(units, uint16(high), uint16(low))
		} else {
			units = append(units, uint16(r))
		}
		i += size
	}
	return units
}

// UTF16String converts UTF-16 code units back to a string: a valid
// surrogate pair as its supplementary character, an unpaired surrogate in
// its three-byte form.
func UTF16String(units []uint16) string {
	b := make([]byte, 0, 3*len(units))
	for i := 0; i < len(units); i++ {
		u := rune(units[i])
		if utf16.IsSurrogate(u) && i+1 < len(units) {
			if pair := utf16.DecodeRune(u, rune(units[i+1])); pair != utf8.RuneError {
				b = utf8.AppendRune(b, pair)
				i++
				continue
			}
		}
		b = appendUnitRune(b, u)
	}
	return string(b)
}

// UTF16Runes returns s's code points: a valid surrogate pair as its
// supplementary character, an unpaired surrogate as its own unit's value.
func UTF16Runes(s string) []rune {
	out := make([]rune, 0, len(s))
	for i := 0; i < len(s); {
		r, size := decodeUnitRune(s[i:])
		out = append(out, r)
		i += size
	}
	return out
}

// UTF16Cut returns the first n UTF-16 code units of s, or s when it is no
// longer. A supplementary character straddling the cut keeps its leading
// surrogate as an unpaired unit.
func UTF16Cut(s string, n int) string {
	used := 0
	for i := 0; i < len(s); {
		r, size := decodeUnitRune(s[i:])
		w := unitsOf(r)
		if used+w > n {
			if w == 2 && used < n {
				high, _ := utf16.EncodeRune(r)
				return string(appendUnitRune([]byte(s[:i]), high))
			}
			return s[:i]
		}
		used += w
		i += size
	}
	return s
}

// UTF16LowerKey returns s lower-cased as a key for case-insensitive
// matching of client text: comparing or searching two keys byte-wise
// compares their UTF-16 unit sequences. Code points are lower-cased with a
// surrogate pair taken as its supplementary character and an unpaired
// surrogate left as it is; every unit, each half of a pair included, is
// then written on its own, so a lone surrogate matches inside a pair that
// starts or ends with it. The key is not text to show a client.
func UTF16LowerKey(s string) string {
	units := UTF16Units(s)
	b := make([]byte, 0, len(s)+len(s)/2)
	for i := 0; i < len(units); i++ {
		r := rune(units[i])
		if utf16.IsSurrogate(r) && i+1 < len(units) {
			if pair := utf16.DecodeRune(r, rune(units[i+1])); pair != utf8.RuneError {
				r = pair
				i++
			}
		}
		if r == 'İ' {
			// Its full lower case is two code points: i and a combining dot above.
			b = append(b, 'i')
			r = '\u0307'
		} else {
			r = unicode.ToLower(r)
		}
		if r >= 0x10000 {
			high, low := utf16.EncodeRune(r)
			b = appendUnitRune(appendUnitRune(b, high), low)
			continue
		}
		b = appendUnitRune(b, r)
	}
	return string(b)
}

// decodeUnitRune decodes the code point s starts with and its byte width:
// an unpaired surrogate in its three-byte form as that unit's value, an
// invalid byte as utf8.RuneError of width 1.
func decodeUnitRune(s string) (rune, int) {
	if len(s) >= 3 && s[0] == 0xED && s[1]&0xE0 == 0xA0 && s[2]&0xC0 == 0x80 {
		return 0xD000 | rune(s[1]&0x3F)<<6 | rune(s[2]&0x3F), 3
	}
	return utf8.DecodeRuneInString(s)
}

// appendUnitRune appends r, which may be a surrogate, in the generalized
// UTF-8 form.
func appendUnitRune(b []byte, r rune) []byte {
	if utf16.IsSurrogate(r) {
		return append(b, 0xED, 0x80|byte(r>>6&0x3F), 0x80|byte(r&0x3F))
	}
	return utf8.AppendRune(b, r)
}

func unitsOf(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}
