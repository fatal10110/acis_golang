package network

import (
	"math/bits"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// The layout rules of the hash table a memo of AI parameters keeps its
// keys in.
const (
	memoDefaultCapacity    = 16
	memoTreeifyThreshold   = 8
	memoUntreeifyThreshold = 6
	memoMinTreeifyCapacity = 64
	memoMaxCapacity        = 1 << 30
)

// memoKey is one key of a memoTable bin with its spread hash.
type memoKey struct {
	key  string
	hash int32
}

// memoBin is one slot of a memoTable: its keys in chain order. A tree bin
// takes a new key at the front of its chain.
type memoBin struct {
	keys []memoKey
	tree bool
}

// memoTable lays out keys the way the hash table of an AI parameter memo
// does, so an admin page lists them in the order the memo iterates them:
// slot by slot, each slot's chain in order. The order depends on the
// insertion order and on every resize on the way, both of which it
// replays.
type memoTable struct {
	bins    []memoBin
	count   int
	sizeCtl int
}

// memoOrder is the order a memo filled with keys, in insertion order,
// lists them. A repeated key keeps its first place.
func memoOrder(keys []string) []string {
	var t memoTable
	for _, k := range keys {
		t.put(k)
	}
	return t.keys()
}

// memoCopyOrder is the order of a memo built as a copy of one filled with
// keys: the copy is sized for the whole source first, then takes the
// source's keys in the order the source lists them.
func memoCopyOrder(keys []string) []string {
	src := memoOrder(keys)
	t := memoTable{sizeCtl: memoDefaultCapacity}
	t.presize(len(src))
	for _, k := range src {
		t.put(k)
	}
	return t.keys()
}

// keys lists the table's keys slot by slot.
func (t *memoTable) keys() []string {
	out := make([]string, 0, t.count)
	for _, b := range t.bins {
		for _, k := range b.keys {
			out = append(out, k.key)
		}
	}
	return out
}

func memoSpread(h int32) int32 {
	return (h ^ int32(uint32(h)>>16)) & 0x7fffffff
}

// memoTableSizeFor is the smallest power of two at least c.
func memoTableSizeFor(c int) int {
	if c <= 1 {
		return 1
	}
	if c >= memoMaxCapacity {
		return memoMaxCapacity
	}
	return 1 << bits.Len(uint(c-1))
}

func (t *memoTable) allocate(n int) {
	t.bins = make([]memoBin, n)
	t.sizeCtl = n - n>>2
}

// put adds key; a key already present keeps its place.
func (t *memoTable) put(key string) {
	h := memoSpread(commons.LegacyStringHash(key))
	if t.bins == nil {
		n := memoDefaultCapacity
		if t.sizeCtl > 0 {
			n = t.sizeCtl
		}
		t.allocate(n)
	}
	n := len(t.bins)
	i := int(h) & (n - 1)
	b := &t.bins[i]
	switch {
	case len(b.keys) == 0:
		b.keys = []memoKey{{key, h}}
	case b.tree:
		for _, k := range b.keys {
			if k.key == key {
				return
			}
		}
		b.keys = append([]memoKey{{key, h}}, b.keys...)
	default:
		// The chain length walked: where key was found, else the length
		// before key was appended.
		walked := len(b.keys)
		found := false
		for j, k := range b.keys {
			if k.key == key {
				walked, found = j+1, true
				break
			}
		}
		if !found {
			b.keys = append(b.keys, memoKey{key, h})
		}
		if walked >= memoTreeifyThreshold {
			t.treeify(i)
		}
		if found {
			return
		}
	}
	t.count++
	for t.count >= t.sizeCtl && len(t.bins) < memoMaxCapacity {
		t.transfer()
	}
}

// treeify turns the long chain of slot i into a tree bin, or grows a table
// still too small for one instead.
func (t *memoTable) treeify(i int) {
	n := len(t.bins)
	if n < memoMinTreeifyCapacity {
		t.presize(n << 1)
		return
	}
	t.bins[i].tree = true
}

// presize grows the table, allocating it when there is none yet, until it
// is large enough for size keys.
func (t *memoTable) presize(size int) {
	c := memoMaxCapacity
	if size < memoMaxCapacity>>1 {
		c = memoTableSizeFor(size + size>>1 + 1)
	}
	for {
		if t.bins == nil {
			t.allocate(max(t.sizeCtl, c))
			continue
		}
		if c <= t.sizeCtl || len(t.bins) >= memoMaxCapacity {
			return
		}
		t.transfer()
	}
}

// transfer doubles the table. A chain splits by the new hash bit: the
// trailing run of keys that land in one half keeps its order, and each key
// before it goes in front of its half, so those come out reversed. A tree
// bin splits keeping its order and stays a tree while it is long enough.
func (t *memoTable) transfer() {
	n := len(t.bins)
	next := make([]memoBin, n<<1)
	for i, b := range t.bins {
		if len(b.keys) == 0 {
			continue
		}
		var lo, hi []memoKey
		if b.tree {
			for _, k := range b.keys {
				if int(k.hash)&n == 0 {
					lo = append(lo, k)
				} else {
					hi = append(hi, k)
				}
			}
			next[i] = memoBin{keys: lo, tree: len(lo) > memoUntreeifyThreshold}
			next[i+n] = memoBin{keys: hi, tree: len(hi) > memoUntreeifyThreshold}
			continue
		}
		last := 0
		runBit := int(b.keys[0].hash) & n
		for j := 1; j < len(b.keys); j++ {
			if bit := int(b.keys[j].hash) & n; bit != runBit {
				runBit, last = bit, j
			}
		}
		run := append([]memoKey(nil), b.keys[last:]...)
		if runBit == 0 {
			lo = run
		} else {
			hi = run
		}
		for _, k := range b.keys[:last] {
			if int(k.hash)&n == 0 {
				lo = append([]memoKey{k}, lo...)
			} else {
				hi = append([]memoKey{k}, hi...)
			}
		}
		next[i] = memoBin{keys: lo}
		next[i+n] = memoBin{keys: hi}
	}
	t.bins = next
	t.sizeCtl = n<<1 - n>>1
}
