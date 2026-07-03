package commons

// LegacyStringHash computes a 32-bit polynomial string hash:
//
//	s[0]*31^(n-1) + s[1]*31^(n-2) + ... + s[n-1]
//
// over the string's UTF-16 code units, with 32-bit wraparound. This exact
// algorithm is required byte-for-byte wherever a hash value crosses into
// game data or reaches the client — e.g. multisell list IDs are keyed by
// the hash of their source filename. Using any other hash (fnv, Go's map
// hashing, etc) here produces different numbers and breaks those
// references.
//
// Go strings are UTF-8; the algorithm is defined over UTF-16 code units, so
// non-BMP runes (outside the Basic Multilingual Plane) are encoded as a
// UTF-16 surrogate pair — two code units, two hash iterations — before
// hashing. Game data such as filenames is expected to be ASCII/BMP in
// practice, but the surrogate-pair handling is included for correctness on
// the general case.
func LegacyStringHash(s string) int32 {
	var h int32
	for _, r := range s {
		if r > 0xFFFF {
			// Encode as a UTF-16 surrogate pair before hashing.
			r -= 0x10000
			high := int32(0xD800 + (r >> 10))
			low := int32(0xDC00 + (r & 0x3FF))
			h = 31*h + high
			h = 31*h + low
			continue
		}
		h = 31*h + int32(r)
	}
	return h
}
