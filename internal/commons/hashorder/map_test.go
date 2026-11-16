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
