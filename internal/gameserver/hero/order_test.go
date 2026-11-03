package hero

import (
	"slices"
	"testing"
)

// TestHeroTableOrder pins heroTable's listing against a JDK 21 HashMap
// keyed by Integer, probed with the same ids and calls: put one at a time,
// then clear and putAll of a second map.
func TestHeroTableOrder(t *testing.T) {
	t.Parallel()
	fill := func(ids ...int32) *heroTable {
		var tb heroTable
		for _, id := range ids {
			tb.put(id)
		}
		return &tb
	}
	low := []int32{17, 3, 20, 1, 30, 5, 16, 9, 2, 25, 12, 7, 18}
	thirteen := make([]int32, len(low))
	for k, b := range low {
		thirteen[k] = 0x10000000 + 0x100*int32(k) + b
	}
	grown := fill(thirteen...)
	if got, want := grown.order(), []int32{
		0x10000301, 0x10000802, 0x10000103, 0x10000505, 0x10000b07, 0x10000709, 0x10000a0c,
		0x10000610, 0x10000011, 0x10000c12, 0x10000214, 0x10000919, 0x1000041e,
	}; !slices.Equal(got, want) {
		t.Fatalf("13 rows = %x, want %x", got, want)
	}
	if got, want := fill(0x10000001, 0x10000011).order(), []int32{0x10000001, 0x10000011}; !slices.Equal(got, want) {
		t.Fatalf("shared bucket = %x, want %x", got, want)
	}

	// A table that never held a row is sized for the copy: 4 buckets for
	// 3 rows, which lists the source's 16-bucket order differently.
	src := fill(1, 5, 2)
	if got, want := src.order(), []int32{1, 2, 5}; !slices.Equal(got, want) {
		t.Fatalf("source = %v, want %v", got, want)
	}
	var fresh heroTable
	fresh.replace(src.order())
	if got, want := fresh.order(), []int32{1, 5, 2}; !slices.Equal(got, want) {
		t.Fatalf("copy into a new table = %v, want %v", got, want)
	}

	// A cleared table keeps the 32 buckets it grew to.
	src = fill(0x11, 0x01)
	grown.replace(src.order())
	if got, want := grown.order(), []int32{0x01, 0x11}; !slices.Equal(got, want) {
		t.Fatalf("copy into the grown table = %x, want %x", got, want)
	}
	if got, want := src.order(), []int32{0x11, 0x01}; !slices.Equal(got, want) {
		t.Fatalf("source = %x, want %x", got, want)
	}
}
