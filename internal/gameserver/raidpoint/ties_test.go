package raidpoint

import (
	"slices"
	"testing"

	"github.com/rs/zerolog"
)

// TestTiesRankInHashTableOrder: equal totals keep the order of a hash
// table filled with the players holding points, in the order the records
// table lists them; both are tables of 16 buckets doubled at three
// quarters, the records table already when it holds twelve.
func TestTiesRankInHashTableOrder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		add   []int32 // players given 10 points, in order
		zeros []int32 // players then entered with no points
		want  []int32
	}{
		// 0x10000010 folds into bucket 0, 0x10000001 into bucket 1.
		{"buckets", []int32{0x10000001, 0x10000010}, nil, []int32{0x10000010, 0x10000001}},
		// 17 and 1 share bucket 1 and keep the order they came in.
		{"shared bucket", []int32{17, 1}, nil, []int32{17, 1}},
		// Twelve records grow the records table to 32 buckets, where 1
		// lists before 17; the ranking's table of two still shares a
		// bucket, so 1 stays first.
		{"grown records", []int32{17, 1}, []int32{100, 101, 102, 103, 104, 105, 106, 107, 108, 109}, []int32{1, 17}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := New(&opStore{}, nil, zerolog.Nop())
			for _, id := range tc.add {
				p.Add(id, 25001, 10)
			}
			for _, id := range tc.zeros {
				p.Add(id, 25001, 0)
			}
			if got := p.Winners(); !slices.Equal(got, tc.want) {
				t.Fatalf("winners %#x, want %#x", got, tc.want)
			}
			for i, id := range tc.want {
				if r := p.Record(id); r.Rank != int32(i+1) {
					t.Fatalf("player %#x has rank %d, want %d", id, r.Rank, i+1)
				}
			}
		})
	}
}

// TestCleanUpKeepsRecordsTableSize: a wipe empties the records table but
// keeps its size, so ties still rank by the grown table afterwards.
func TestCleanUpKeepsRecordsTableSize(t *testing.T) {
	p := New(&opStore{}, nil, zerolog.Nop())
	for id := int32(100); id < 112; id++ {
		p.Add(id, 25001, 0)
	}
	p.CleanUp()
	p.Add(17, 25001, 10)
	p.Add(1, 25001, 10)
	if got, want := p.Winners(), []int32{1, 17}; !slices.Equal(got, want) {
		t.Fatalf("winners after a wipe %v, want %v", got, want)
	}
}
