package hashorder

import (
	"slices"
	"testing"
)

// TestClearKeepsTheTableSize: a cleared set is empty but keeps the 32
// buckets twelve ids grew it to, so ids sharing a bucket of 16 but not of
// 32 list as the larger table puts them.
func TestClearKeepsTheTableSize(t *testing.T) {
	const a, c, d = 0x70000020, 0x70000010, 0x70000001 // buckets 0, 0, 1 of 16; 0, 16, 1 of 32
	var s Set
	for id := int32(0x70000040); s.Len() < 12; id++ {
		s.Add(id)
	}
	s.Clear()
	if s.Len() != 0 || s.Has(0x70000040) || len(s.Order()) != 0 {
		t.Fatalf("cleared set holds %v", s.Order())
	}
	for _, id := range []int32{a, c, d} {
		s.Add(id)
	}
	if got, want := s.Order(), []int32{a, d, c}; !slices.Equal(got, want) {
		t.Fatalf("order after clear = %x, want %x", got, want)
	}
	var fresh Set
	for _, id := range []int32{a, c, d} {
		fresh.Add(id)
	}
	if got, want := fresh.Order(), []int32{a, c, d}; !slices.Equal(got, want) {
		t.Fatalf("fresh order = %x, want %x", got, want)
	}
}
