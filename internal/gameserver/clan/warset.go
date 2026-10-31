package clan

import "slices"

// Hash-table geometry of the concurrent hash set the client-visible war
// order comes from.
const (
	setMinTable    = 16 // buckets of a fresh table
	setTreeifyAt   = 8  // chain length at which a bucket is treated as crowded
	setTreeMinSize = 64 // smallest table whose crowded bucket becomes a tree
	setUntreeifyAt = 6  // a tree bucket this small or smaller turns back into a chain
)

// warSet is a set of clan ids that remembers the order a concurrent hash
// set lists them in: the table starts at 16 buckets and doubles whenever it
// holds three quarters of its size, a bucket chains its ids in insertion
// order, and a doubling reorders a chain (see grow). Removing an id only
// unlinks it, so the table never shrinks and a clan that once held twelve
// or more wars keeps its larger table after they end.
//
// A chain that already holds eight ids when a ninth arrives is crowded: in
// a table below 64 buckets the table grows until it holds three times the
// current size at three quarters full (presize); from 64 buckets the chain
// becomes a tree bucket, which lists its ids newest first and keeps the
// order of its ids when the table doubles. A tree bucket that has shrunk
// to a few ids turns back into a chain on removal, depending on the shape
// of the tree; that is not modeled (#4362): the bucket stays a tree here
// until the table doubles with six or fewer ids left in it.
//
// The zero value is an empty set. It is not safe for concurrent use; the
// owning clan's lock guards it.
type warSet struct {
	table []warBin
	count int
}

// warBin is one bucket: its ids in listing order.
type warBin struct {
	ids  []int32
	tree bool
}

// len returns how many ids the set holds.
func (s *warSet) len() int { return s.count }

// has reports whether id is in the set.
func (s *warSet) has(id int32) bool {
	if s.count == 0 {
		return false
	}
	return slices.Contains(s.table[s.index(id)].ids, id)
}

func (s *warSet) index(id int32) int {
	return int(foldedHash(id) & uint32(len(s.table)-1))
}

// add puts id in the set; an id already in it keeps its place.
func (s *warSet) add(id int32) {
	if s.table == nil {
		s.table = make([]warBin, setMinTable)
	}
	if s.has(id) {
		return
	}
	bin := &s.table[s.index(id)]
	crowded := !bin.tree && len(bin.ids) >= setTreeifyAt
	if bin.tree {
		bin.ids = slices.Insert(bin.ids, 0, id)
	} else {
		bin.ids = append(bin.ids, id)
	}
	if crowded {
		if len(s.table) < setTreeMinSize {
			// the table is grown to hold three times its size at
			// three quarters full
			for want := 4 * len(s.table); want > threshold(len(s.table)); {
				s.grow()
			}
		} else {
			bin.tree = true
		}
	}
	s.count++
	for s.count >= threshold(len(s.table)) {
		s.grow()
	}
}

// threshold is the count at which a table of n buckets doubles.
func threshold(n int) int { return n - n/4 }

// remove takes id out of the set; the table keeps its size.
func (s *warSet) remove(id int32) {
	if !s.has(id) {
		return
	}
	bin := &s.table[s.index(id)]
	bin.ids = slices.Delete(bin.ids, slices.Index(bin.ids, id), slices.Index(bin.ids, id)+1)
	s.count--
}

// order lists the ids in the order the client is sent them.
func (s *warSet) order() []int32 {
	out := make([]int32, 0, s.count)
	for _, bin := range s.table {
		out = append(out, bin.ids...)
	}
	return out
}

// foldedHash is an id's hash: the id folded with its high half.
func foldedHash(id int32) uint32 {
	h := uint32(id)
	return h ^ h>>16
}

// grow doubles the table. A chain splits by the id's next hash bit into the
// bucket at the same index (low half) and the one n above (high half): the
// chain's trailing run of ids sharing one bit moves as it is, every id
// before that run is put in front of its half's chain one by one, so those
// come out in reverse. A tree bucket splits keeping its order and turns
// into a chain in a half of six ids or fewer.
func (s *warSet) grow() {
	n := len(s.table)
	next := make([]warBin, 2*n)
	for i, bin := range s.table {
		if len(bin.ids) == 0 {
			continue
		}
		bit := func(id int32) bool { return foldedHash(id)&uint32(n) != 0 }
		var lo, hi []int32
		if bin.tree {
			for _, id := range bin.ids {
				if bit(id) {
					hi = append(hi, id)
				} else {
					lo = append(lo, id)
				}
			}
			next[i].tree = len(lo) > setUntreeifyAt
			next[i+n].tree = len(hi) > setUntreeifyAt
		} else {
			chain := bin.ids
			runBit, runStart := bit(chain[0]), 0
			for j := 1; j < len(chain); j++ {
				if b := bit(chain[j]); b != runBit {
					runBit, runStart = b, j
				}
			}
			for _, id := range chain[:runStart] {
				if bit(id) {
					hi = append([]int32{id}, hi...)
				} else {
					lo = append([]int32{id}, lo...)
				}
			}
			if runBit {
				hi = append(hi, chain[runStart:]...)
			} else {
				lo = append(lo, chain[runStart:]...)
			}
		}
		next[i].ids, next[i+n].ids = lo, hi
	}
	s.table = next
}
