package raidpoint

// The client lists a player's bosses in the order the reference record
// keeps them, which is its hash table's: bucket by bucket, and within a
// bucket in chain order. record replays that table. It starts at
// initialBuckets buckets and doubles, keeping each chain's order, when it
// holds more than three quarters of its bucket count: right after an
// insert on restore, and before the next change when points are added. A
// restored boss goes to the end of its chain, an added one to the front.
// While the table is under treeBuckets buckets, a chain reaching the tree
// limit doubles the table instead of becoming a tree. A chain that reaches
// it in a larger table, which takes eight bosses one bucket apart in a
// record of at least 49, keeps its plain chain order here, where the
// reference lists the tree differently.
const (
	initialBuckets = 16
	treeBuckets    = 64
	// restoreTreeLimit and addTreeLimit are how long a chain already is
	// when one more boss makes it a tree, for a restored and for an added
	// boss.
	restoreTreeLimit = 8
	addTreeLimit     = 7
)

// record is one player's points per boss.
type record struct {
	points map[int32]int32
	// table is the bosses in their buckets, each in chain order.
	table [][]int32
}

// restore enters a stored boss with its points.
func (r *record) restore(bossID, points int32) {
	if r.table == nil {
		r.grow()
	}
	if _, ok := r.points[bossID]; ok {
		r.points[bossID] = points
		return
	}
	b := bucket(bossID, len(r.table))
	chain := len(r.table[b])
	r.table[b] = append(r.table[b], bossID)
	r.points[bossID] = points
	if chain >= restoreTreeLimit && len(r.table) < treeBuckets {
		r.grow()
	}
	if len(r.points) > r.threshold() {
		r.grow()
	}
}

// add adds points to the boss and returns its new total, wrapping at 32
// bits.
func (r *record) add(bossID, points int32) int32 {
	if r.table == nil || len(r.points) > r.threshold() {
		r.grow()
	}
	if cur, ok := r.points[bossID]; ok {
		cur += points
		r.points[bossID] = cur
		return cur
	}
	b := bucket(bossID, len(r.table))
	chain := len(r.table[b])
	r.table[b] = append([]int32{bossID}, r.table[b]...)
	r.points[bossID] = points
	if chain >= addTreeLimit && len(r.table) < treeBuckets {
		r.grow()
	}
	return points
}

func (r *record) threshold() int {
	return len(r.table) * 3 / 4
}

// grow creates the table, or doubles it keeping each chain's order.
func (r *record) grow() {
	if r.table == nil {
		r.table = make([][]int32, initialBuckets)
		return
	}
	table := make([][]int32, 2*len(r.table))
	for _, chain := range r.table {
		for _, id := range chain {
			b := bucket(id, len(table))
			table[b] = append(table[b], id)
		}
	}
	r.table = table
}

func bucket(bossID int32, buckets int) int {
	h := uint32(bossID)
	return int((h ^ h>>16) & uint32(buckets-1))
}

// entries returns the record's bosses in listing order.
func (r *record) entries() []Entry {
	out := make([]Entry, 0, len(r.points))
	for _, chain := range r.table {
		for _, id := range chain {
			out = append(out, Entry{BossID: id, Points: r.points[id]})
		}
	}
	return out
}

func (r *record) total() int32 {
	var sum int32
	for _, v := range r.points {
		sum += v
	}
	return sum
}
