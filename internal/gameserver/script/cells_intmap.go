package script

import "sync"

// IntMapCell maps int32 keys to values, empty at first, and walks them in
// the order the reference's hash map of integer keys does: bucket by bucket,
// the bucket of a key being its hash (the key with its upper 16 bits folded
// into the lower ones) modulo the table size, and in insertion order within
// a bucket. The table starts at 16 buckets, or at the size
// NewIntMapCell sizes it for, and doubles once the cell holds more than
// three quarters of its bucket count, or when a bucket of a table under 64
// buckets gets a ninth key; a doubling keeps each bucket's order. Removing
// a key or clearing the cell never shrinks the table.
//
// The order is the reference's while no bucket of a table of 64 or more
// buckets gets a ninth key; the reference then keeps that bucket as a
// tree, whose walk order this cell does not follow. A script's map of clans
// stays far below that.
//
// The walks return snapshots, so a script may set or delete keys while it
// goes through one, as the reference does through a copy of its keys or an
// iterator's remove.
type IntMapCell[V any] struct {
	mu sync.Mutex
	// buckets is the table, nil until the first Set; initial is the size
	// the first Set allocates, 0 for the default.
	buckets [][]IntMapEntry[V]
	initial int
	size    int
}

// IntMapEntry is one key and its value.
type IntMapEntry[V any] struct {
	Key   int32
	Value V
}

const (
	intMapDefaultBuckets = 16
	intMapTreeifyKeys    = 8
	intMapTreeifyBuckets = 64
	intMapMaxBuckets     = 1 << 30
)

// NewIntMapCell returns an empty cell whose table is sized for expected
// keys: the smallest power of two that holds them below three quarters,
// at least 1, as the reference sizes a map made for that many mappings.
func NewIntMapCell[V any](expected int) *IntMapCell[V] {
	n := 1
	want := (expected*4 + 2) / 3 // ceil(expected / 0.75)
	for n < want && n < intMapMaxBuckets {
		n <<= 1
	}
	return &IntMapCell[V]{initial: n}
}

// intMapHash is the reference's hash of an integer key.
func intMapHash(k int32) uint32 {
	h := uint32(k)
	return h ^ h>>16
}

// Get returns the value of k and whether k is present.
func (c *IntMapCell[V]) Get(k int32) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i, j, ok := c.find(k); ok {
		return c.buckets[i][j].Value, true
	}
	var zero V
	return zero, false
}

// Has reports whether k is present.
func (c *IntMapCell[V]) Has(k int32) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _, ok := c.find(k)
	return ok
}

// Set maps k to v. A present key keeps its place in the walk order.
func (c *IntMapCell[V]) Set(k int32, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.buckets == nil {
		n := c.initial
		if n == 0 {
			n = intMapDefaultBuckets
		}
		c.buckets = make([][]IntMapEntry[V], n)
	}
	if i, j, ok := c.find(k); ok {
		c.buckets[i][j].Value = v
		return
	}
	i := c.bucket(k)
	c.buckets[i] = append(c.buckets[i], IntMapEntry[V]{Key: k, Value: v})
	if len(c.buckets[i]) > intMapTreeifyKeys && len(c.buckets) < intMapTreeifyBuckets {
		c.grow()
	}
	c.size++
	if c.size > len(c.buckets)*3/4 {
		c.grow()
	}
}

// Delete removes k and returns the value it had and whether it was
// present.
func (c *IntMapCell[V]) Delete(k int32) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	i, j, ok := c.find(k)
	if !ok {
		var zero V
		return zero, false
	}
	v := c.buckets[i][j].Value
	c.buckets[i] = append(c.buckets[i][:j:j], c.buckets[i][j+1:]...)
	c.size--
	return v, true
}

// Clear removes every key; the table keeps its size.
func (c *IntMapCell[V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.buckets {
		c.buckets[i] = nil
	}
	c.size = 0
}

// Len returns how many keys are present.
func (c *IntMapCell[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.size
}

// Entries returns every key and its value, in walk order.
func (c *IntMapCell[V]) Entries() []IntMapEntry[V] {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]IntMapEntry[V], 0, c.size)
	for _, b := range c.buckets {
		out = append(out, b...)
	}
	return out
}

// Keys returns every key, in walk order.
func (c *IntMapCell[V]) Keys() []int32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]int32, 0, c.size)
	for _, b := range c.buckets {
		for _, e := range b {
			out = append(out, e.Key)
		}
	}
	return out
}

// Values returns every value, in walk order.
func (c *IntMapCell[V]) Values() []V {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]V, 0, c.size)
	for _, b := range c.buckets {
		for _, e := range b {
			out = append(out, e.Value)
		}
	}
	return out
}

// bucket returns the bucket of k. The caller holds c.mu and the table is
// allocated.
func (c *IntMapCell[V]) bucket(k int32) int {
	return int(intMapHash(k) & uint32(len(c.buckets)-1))
}

// find returns the bucket and the position in it of k. The caller holds
// c.mu.
func (c *IntMapCell[V]) find(k int32) (i, j int, ok bool) {
	if c.buckets == nil {
		return 0, 0, false
	}
	i = c.bucket(k)
	for j, e := range c.buckets[i] {
		if e.Key == k {
			return i, j, true
		}
	}
	return 0, 0, false
}

// grow doubles the table, each bucket splitting into the one at its index
// and the one a table size above, both in the bucket's order. A table at
// its largest size stays as it is. The caller holds c.mu.
func (c *IntMapCell[V]) grow() {
	n := len(c.buckets)
	if n >= intMapMaxBuckets {
		return
	}
	next := make([][]IntMapEntry[V], 2*n)
	for i, b := range c.buckets {
		for _, e := range b {
			if intMapHash(e.Key)&uint32(n) == 0 {
				next[i] = append(next[i], e)
			} else {
				next[i+n] = append(next[i+n], e)
			}
		}
	}
	c.buckets = next
}
