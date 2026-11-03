package hero

import "slices"

// heroTable remembers the order the running era's heroes are listed in:
// the row order of a chained hash map keyed by object id. A row sits in
// the bucket of its id with the id's high half folded in, masked by the
// table size; a bucket keeps its rows in insertion order, and a doubling
// keeps that order. The table starts at 16 buckets and doubles when it
// holds more than three quarters of its size, and also when a bucket
// reaches nine rows while the table is smaller than 64; a table filled
// all at once is sized first (see replace). It never shrinks: an election
// that seats fewer heroes keeps the size the table grew to.
// Not modeled: the tree bucket a ninth colliding row makes in a table of
// 64 or more.
//
// The zero value is an empty table. It is not safe for concurrent use;
// the manager's lock guards it.
type heroTable struct {
	ids     []int32 // in insertion order
	buckets int     // 0 until the first row
}

// put adds id, which the table does not hold yet, one row at a time.
func (t *heroTable) put(id int32) {
	if t.buckets == 0 {
		t.buckets = 16
	}
	t.ids = append(t.ids, id)
	b := t.bucket(id)
	chain := 0
	for _, o := range t.ids {
		if t.bucket(o) == b {
			chain++
		}
	}
	if chain >= 9 && t.buckets < 64 {
		t.buckets *= 2
	}
	if len(t.ids) > t.buckets*3/4 {
		t.buckets *= 2
	}
}

// replace empties the table, keeping its size, then adds ids in order.
// It first sizes the table to hold them all: a table that holds no row
// yet takes the smallest power of two whose three quarters reach
// len(ids), which may be below 16; any other doubles until it holds them.
func (t *heroTable) replace(ids []int32) {
	t.ids = nil
	if len(ids) == 0 {
		return
	}
	if t.buckets == 0 {
		t.buckets = 1
		for t.buckets*3 < 4*len(ids) {
			t.buckets *= 2
		}
	}
	for len(ids) > t.buckets*3/4 {
		t.buckets *= 2
	}
	for _, id := range ids {
		t.put(id)
	}
}

// order lists the ids in row order: by bucket, then insertion order.
func (t *heroTable) order() []int32 {
	out := slices.Clone(t.ids)
	slices.SortStableFunc(out, func(a, b int32) int { return t.bucket(a) - t.bucket(b) })
	return out
}

func (t *heroTable) bucket(id int32) int {
	h := uint32(id)
	return int((h ^ h>>16) & uint32(t.buckets-1))
}
