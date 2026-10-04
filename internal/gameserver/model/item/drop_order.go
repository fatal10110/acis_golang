package item

// dropMerger sums a category roll's quantities per item id and keeps them in
// the order the reference iterates its result: DropCategory.calculateDrop
// collects into a new HashMap<>(1) through merge(id, qty, Integer::sum), and
// Monster.doItemDrop walks its entrySet. That order is the bucket order of
// the map's table, each bucket newest key first.
//
// The zero value is an empty map of initial capacity 1.
type dropMerger struct {
	// table holds the buckets; a nil table is the reference's lazily
	// allocated one.
	table [][]rolledDrop
	size  int
	// threshold is the size past which the next merge first resizes; for an
	// unallocated table it is the initial capacity instead.
	threshold int
}

// merge adds qty to itemID, as HashMap.merge does: the table resizes at
// the start of the call once a previous call left it over its threshold,
// and a new key goes to the head of its bucket.
func (m *dropMerger) merge(itemID, qty int32) {
	if m.table == nil {
		m.threshold = 1
	}
	if m.size > m.threshold || m.table == nil {
		m.resize()
	}
	b := bucketOf(itemID, len(m.table))
	for i := range m.table[b] {
		if m.table[b][i].ItemID == itemID {
			m.table[b][i].Count += qty
			return
		}
	}
	m.table[b] = append([]rolledDrop{{ItemID: itemID, Count: qty}}, m.table[b]...)
	m.size++
}

// resize doubles the table (or allocates it at the initial capacity) and
// recomputes the threshold at load factor 0.75, keeping each bucket's
// relative order as HashMap.resize's split does.
func (m *dropMerger) resize() {
	oldCap := len(m.table)
	newCap, newThreshold := m.threshold, 0
	if oldCap > 0 {
		newCap = oldCap << 1
		if oldCap >= 16 {
			newThreshold = m.threshold << 1
		}
	}
	if newThreshold == 0 {
		newThreshold = int(float32(newCap) * 0.75)
	}
	table := make([][]rolledDrop, newCap)
	for _, bucket := range m.table {
		for _, e := range bucket {
			b := bucketOf(e.ItemID, newCap)
			table[b] = append(table[b], e)
		}
	}
	m.table, m.threshold = table, newThreshold
}

// ordered returns the merged entries in iteration order, or nil when
// nothing merged.
func (m *dropMerger) ordered() []rolledDrop {
	if m.size == 0 {
		return nil
	}
	out := make([]rolledDrop, 0, m.size)
	for _, bucket := range m.table {
		out = append(out, bucket...)
	}
	return out
}

// bucketOf is HashMap's bucket index for an Integer key: its hash spread
// with the high half, masked to the table capacity.
func bucketOf(itemID int32, capacity int) int {
	h := uint32(itemID)
	return int((h ^ h>>16) & uint32(capacity-1))
}
