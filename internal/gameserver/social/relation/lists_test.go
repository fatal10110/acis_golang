package relation

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// walkedList is a list read the slow way: every pair the store holds, in
// the order it walks them, then clientOrder.
func walkedList(m *Manager, id int32, blocks bool) []int32 {
	var ids []int32
	for _, b := range m.order.bins {
		for _, k := range b.pairs {
			rel := m.relations[k]
			switch {
			case !blocks && rel&flagFriends != 0 && id == k.low:
				ids = append(ids, k.high)
			case !blocks && rel&flagFriends != 0 && id == k.high:
				ids = append(ids, k.low)
			case blocks && id == k.low && rel&flagLowBlocksHigh != 0:
				ids = append(ids, k.high)
			case blocks && id == k.high && rel&flagHighBlocksLow != 0:
				ids = append(ids, k.low)
			}
		}
	}
	return clientOrder(ids)
}

// TestIndexedListsMatchStoreWalk pins the per-character index (#5104): after
// loads, adds, blocks, unblocks and removals that grow, split and treeify the
// store, every character's friend and block list holds what a walk of every
// pair gathers, in the same order.
func TestIndexedListsMatchStoreWalk(t *testing.T) {
	r := rand.New(rand.NewPCG(5104, 1))
	const base, chars = 268436456, 300
	id := func() int32 { return base + r.Int32N(chars) }
	var rows []Row
	for range 400 {
		rows = append(rows, Row{CharID: id(), FriendID: id(), Relation: r.Int32N(8)})
	}
	m := NewManager(rows)
	check := func(step int) {
		t.Helper()
		for c := int32(base); c < base+chars; c++ {
			if got, want := m.FriendIDs(c), walkedList(m, c, false); !slices.Equal(got, want) {
				t.Fatalf("step %d: FriendIDs(%d) = %v, walk gathers %v", step, c, got, want)
			}
			if got, want := m.BlockedIDs(c), walkedList(m, c, true); !slices.Equal(got, want) {
				t.Fatalf("step %d: BlockedIDs(%d) = %v, walk gathers %v", step, c, got, want)
			}
		}
	}
	check(0)
	for step := 1; step <= 6000; step++ {
		a, b := id(), id()
		switch r.IntN(4) {
		case 0:
			m.AddFriend(a, b)
		case 1:
			m.RemoveFriend(a, b)
		case 2:
			m.Block(a, b)
		default:
			m.Unblock(a, b)
		}
		if step%500 == 0 {
			check(step)
		}
	}
}

// BenchmarkFriendIDsAmongUnrelatedPairs reads one character's lists while
// the store holds 200,000 pairs of other characters (#5104): the cost
// follows the character's own lists, not the pairs held.
func BenchmarkFriendIDsAmongUnrelatedPairs(b *testing.B) {
	const owner = 268436456
	rows := make([]Row, 0, 200_020)
	for i := range int32(200_000) {
		rows = append(rows, Row{CharID: owner + 1 + i, FriendID: owner + 300_000 + i, Relation: flagFriends | flagLowBlocksHigh})
	}
	for i := range int32(20) {
		rows = append(rows, Row{CharID: owner, FriendID: owner + 500_000 + i, Relation: flagFriends | flagLowBlocksHigh})
	}
	m := NewManager(rows)
	b.ResetTimer()
	for b.Loop() {
		if len(m.FriendIDs(owner)) != 20 || len(m.BlockedIDs(owner)) != 20 {
			b.Fatal("owner's lists lost entries")
		}
	}
}
