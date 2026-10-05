package relation

import "slices"

// pairTable keeps every pair in the order the reference relation store
// walks them, the order a friend or block list is gathered in before
// clientOrder. The store is a concurrent hash table of all pairs held since
// boot, a pair cleared of every flag included, keyed by the pair's hash
// (31*low + high folded onto its own high half): 16 bins to start, doubled
// once it holds three quarters of its size. A bin keeps its pairs in
// insertion order, but a doubling moves the pairs ahead of the bin's last run
// of pairs bound for the same half to the front of their half, in reverse
// (see double). A bin given a ninth pair is turned into a tree when the table
// has 64 bins or more, and below that grows the table (see treeify); a tree
// bin takes a new pair at its front.
//
// Every update of a pair counts, not only adding one: an update that finds
// its pair in eighth place or later of a listed bin treeifies it as above.
// Updates are modelled one at a time; two racing updates of the reference
// store can leave it in an order no sequence of them would.
type pairTable struct {
	bins []pairBin
	held int
	// grow is the number of pairs at which the table doubles.
	grow int
}

type pairBin struct {
	pairs []pair
	tree  bool
}

const tableInitialBins = 16

func pairHash(k pair) uint32 {
	h := 31*uint32(k.low) + uint32(k.high)
	return (h ^ h>>16) & 0x7fffffff
}

// update records an update of k: added is whether the table did not hold k
// yet.
func (t *pairTable) update(k pair, added bool) {
	if t.bins == nil {
		t.bins = make([]pairBin, tableInitialBins)
		t.grow = tableInitialBins * 3 / 4
	}
	i := int(pairHash(k) & uint32(len(t.bins)-1))
	b := &t.bins[i]
	if !added {
		if !b.tree && slices.Index(b.pairs, k)+1 >= treeifyThreshold {
			t.treeify(i)
		}
		return
	}
	if b.tree {
		b.pairs = slices.Insert(b.pairs, 0, k)
	} else if b.pairs = append(b.pairs, k); len(b.pairs) > treeifyThreshold {
		t.treeify(i)
	}
	t.held++
	for t.held >= t.grow {
		t.double()
	}
}

// treeify turns bin i into a tree, which keeps its order. Below
// minTreeifyBuckets bins it doubles the table instead, until three quarters
// of it reach the power of two at or above 3n+1 for n bins: 16 bins grow to
// 128, 32 to 256.
func (t *pairTable) treeify(i int) {
	n := len(t.bins)
	if n >= minTreeifyBuckets {
		t.bins[i].tree = true
		return
	}
	size := 2 * n
	want := 1
	for want < size+size/2+1 {
		want *= 2
	}
	for want > t.grow {
		t.double()
	}
}

// double splits every bin between its own index and the one a table size
// higher. A listed bin's last run of pairs bound for the same half keeps its
// order at the end of that half; the pairs ahead of it go, in reverse, to
// the front of theirs. A tree bin splits in order, each half a tree above
// untreeifyThreshold pairs.
func (t *pairTable) double() {
	n := len(t.bins)
	next := make([]pairBin, 2*n)
	half := func(k pair) uint32 { return pairHash(k) & uint32(n) }
	for i, b := range t.bins {
		var lo, hi []pair
		if b.tree {
			for _, k := range b.pairs {
				if half(k) == 0 {
					lo = append(lo, k)
				} else {
					hi = append(hi, k)
				}
			}
			next[i] = pairBin{lo, len(lo) > untreeifyThreshold}
			next[i+n] = pairBin{hi, len(hi) > untreeifyThreshold}
			continue
		}
		run := len(b.pairs) - 1
		for run > 0 && half(b.pairs[run-1]) == half(b.pairs[run]) {
			run--
		}
		for j := run - 1; j >= 0; j-- {
			if half(b.pairs[j]) == 0 {
				lo = append(lo, b.pairs[j])
			} else {
				hi = append(hi, b.pairs[j])
			}
		}
		if run >= 0 {
			if half(b.pairs[run]) == 0 {
				lo = append(lo, b.pairs[run:]...)
			} else {
				hi = append(hi, b.pairs[run:]...)
			}
		}
		next[i].pairs, next[i+n].pairs = lo, hi
	}
	t.bins = next
	t.grow = 2*n - n/2
}
