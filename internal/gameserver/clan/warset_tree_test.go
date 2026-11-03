package clan

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// TestWarSetTreeBucketsStayRedBlackTrees adds and removes ids that crowd a
// few buckets and checks after every step that each tree bucket's tree holds
// exactly its ids, in hash-then-id order, and is a valid red-black tree.
func TestWarSetTreeBucketsStayRedBlackTrees(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	trees := 0
	for round := range 200 {
		var set warSet
		stride := []int32{64, 128, 256, 4096}[round%4]
		var held []int32
		for step := range 400 {
			if len(held) > 0 && rng.IntN(5) < 2 {
				at := rng.IntN(len(held))
				set.remove(held[at])
				held = slices.Delete(held, at, at+1)
			} else {
				id := 0x10000000 + stride*int32(rng.IntN(60))
				if round%3 == 0 && rng.IntN(2) == 0 {
					id = 0x10000000 + int32(rng.IntN(0x30000)) // fills the table so it grows
				}
				if !slices.Contains(held, id) {
					held = append(held, id)
				}
				set.add(id)
			}
			for i, bin := range set.table {
				if bin.root == nil {
					continue
				}
				trees++
				var got []int32
				checkRedBlack(t, bin.root, nil, &got)
				want := slices.Clone(bin.ids)
				slices.SortFunc(want, func(a, b int32) int { return 2*treeSide(&warNode{id: b}, a) - 1 })
				if !slices.Equal(got, want) {
					t.Fatalf("round %d step %d bucket %d: tree holds %v, bucket %v", round, step, i, got, bin.ids)
				}
			}
		}
		if set.len() != len(held) {
			t.Fatalf("round %d: len %d, want %d", round, set.len(), len(held))
		}
	}
	if trees == 0 {
		t.Fatal("no round made a tree bucket")
	}
}

// checkRedBlack walks n in order, appending its ids, and returns its black
// height; it fails on a broken parent link, a red root or red pair, unequal
// black heights or ids out of order.
func checkRedBlack(t *testing.T, n, parent *warNode, ids *[]int32) int {
	t.Helper()
	if n == nil {
		return 1
	}
	if n.parent != parent {
		t.Fatalf("node %d: parent link broken", n.id)
	}
	if parent == nil && n.red {
		t.Fatalf("root %d is red", n.id)
	}
	if n.red && (isRed(n.kid[0]) || isRed(n.kid[1])) {
		t.Fatalf("red node %d has a red child", n.id)
	}
	left := checkRedBlack(t, n.kid[0], n, ids)
	if len(*ids) > 0 && treeSide(&warNode{id: (*ids)[len(*ids)-1]}, n.id) == 0 {
		t.Fatalf("node %d out of order", n.id)
	}
	*ids = append(*ids, n.id)
	if right := checkRedBlack(t, n.kid[1], n, ids); right != left {
		t.Fatalf("node %d: black heights %d and %d", n.id, left, right)
	}
	return left + b2i(!n.red)
}
