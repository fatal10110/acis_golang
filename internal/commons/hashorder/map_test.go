package hashorder

import "testing"

// TestMapIndex: a plain hash table has 16 buckets up to twelve ids and
// doubles past every three quarters of its size; an id's bucket is its low
// bits folded with its high half.
func TestMapIndex(t *testing.T) {
	for _, tc := range []struct {
		id    int32
		count int
		want  int
	}{
		{0x10000010, 2, 0}, // 0x10001010 & 15
		{0x10000001, 2, 1}, // 0x10001001 & 15
		{17, 12, 1},        // 16 buckets hold twelve ids
		{17, 13, 17},       // the thirteenth doubles the table
		{17, 24, 17},       // 32 buckets hold 24 ids
		{0x21, 25, 0x21},   // the 25th doubles it again
		{-1, 1, 0},         // 0xffffffff folds into 0x7fff0000
	} {
		if got := MapIndex(tc.id, tc.count); got != tc.want {
			t.Errorf("MapIndex(%#x, %d) = %d, want %d", tc.id, tc.count, got, tc.want)
		}
	}
}

// TestTextHash: a text key hashes over its UTF-16 code units, 31 times the
// hash so far plus the unit, wrapping at 32 bits.
func TestTextHash(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want int32
	}{
		{"", 0},
		{"a", 97},
		{"ab", 97*31 + 98},
		{"127.0.0.1", 1505998205},
		{"gordon", -1240158087},
		{"\U0001F600", 0xd83d*31 + 0xde00}, // a surrogate pair is two units
	} {
		if got := TextHash(tc.s); got != tc.want {
			t.Errorf("TextHash(%q) = %d, want %d", tc.s, got, tc.want)
		}
	}
}
