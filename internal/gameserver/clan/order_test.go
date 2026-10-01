package clan

import (
	"slices"
	"testing"
)

// The expected orders are worked by hand from the reference registries,
// ConcurrentHashMap keyed by Integer: a key's bucket is
// (h ^ (h >>> 16)) & 0x7fffffff & (n - 1) for a table of n buckets, 16 at
// first and doubled once the size reaches three quarters of n.

// TestWarListOrder lists four clans in bucket order of a 16-bucket table:
// 0x10000020 and 0x10010001 share bucket 0, then 0x10000013 (bucket 3) and
// 0x10000005 (bucket 5).
func TestWarListOrder(t *testing.T) {
	ids := map[int32]struct{}{0x10000005: {}, 0x10000013: {}, 0x10000020: {}, 0x10010001: {}}
	want := []int32{0x10000020, 0x10010001, 0x10000013, 0x10000005}
	if got := hashSetOrder(ids); !slices.Equal(got, want) {
		t.Fatalf("order = %#x, want %#x", got, want)
	}
}

// TestWarListOrderAfterGrowth lists twelve clans, which grow the table to
// 32 buckets: 0x10000030 moves from bucket 0 to bucket 16, after the
// eleven clans in buckets 0-10.
func TestWarListOrderAfterGrowth(t *testing.T) {
	ids := map[int32]struct{}{0x10000030: {}}
	var want []int32
	for k := int32(0); k <= 10; k++ {
		ids[0x10000000+k] = struct{}{}
		want = append(want, 0x10000000+k)
	}
	want = append(want, 0x10000030)
	if got := hashSetOrder(ids); !slices.Equal(got, want) {
		t.Fatalf("order = %#x, want %#x", got, want)
	}
}

// TestSubunitOrder lists every sub-unit in the order its pledge type hashes
// into 16 buckets: -1 (bucket 0), 2001 (1), 2002 (2), 100 (4), 200 (8),
// 1001 (9), 1002 (10).
func TestSubunitOrder(t *testing.T) {
	cl := newClan(1, "Knights", 1)
	for _, id := range []int{SubunitRoyal1, SubunitKnight2, SubunitAcademy, SubunitKnight4, SubunitRoyal2, SubunitKnight1, SubunitKnight3} {
		cl.subunits[id] = &SubPledge{ID: id}
	}
	var got []int
	for _, sp := range cl.Subunits() {
		got = append(got, sp.ID)
	}
	want := []int{-1, 2001, 2002, 100, 200, 1001, 1002}
	if !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}
