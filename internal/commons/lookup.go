package commons

import (
	"cmp"
	"slices"
	"sync/atomic"
)

// Lookup is an in-memory id-keyed lookup, built at boot and read for the
// remainder of the process lifetime: the shape shared by every model
// package's "map id to loaded row" table that overwrites on a duplicate
// key and returns All() sorted ascending by key. An admin reload replaces
// its entries with Swap; every read sees either the old entries or the new
// ones, never a mix. The zero value is not usable; construct with NewLookup
// or NewLookupFromMap.
type Lookup[K cmp.Ordered, V any] struct {
	byKey atomic.Pointer[map[K]V]
}

// NewLookup returns a Lookup backed by items, keyed by key(item). A later
// entry silently overwrites an earlier one with the same key.
func NewLookup[K cmp.Ordered, V any](items []V, key func(V) K) *Lookup[K, V] {
	m := make(map[K]V, len(items))
	for _, item := range items {
		m[key(item)] = item
	}
	return NewLookupFromMap(m)
}

// NewLookupFromMap returns a Lookup wrapping an already-keyed map, for
// callers that build (and validate or mutate) their map before handing it
// off. m is retained, not copied.
func NewLookupFromMap[K cmp.Ordered, V any](m map[K]V) *Lookup[K, V] {
	l := &Lookup[K, V]{}
	l.byKey.Store(&m)
	return l
}

// Swap replaces l's entries with from's, at once for every reader of l.
// from keeps its entries; neither lookup's entries are changed afterwards.
func (l *Lookup[K, V]) Swap(from *Lookup[K, V]) {
	l.byKey.Store(from.byKey.Load())
}

// entries is the map every read of one call works on.
func (l *Lookup[K, V]) entries() map[K]V {
	return *l.byKey.Load()
}

// Get returns the value stored under key, or false if none was loaded.
func (l *Lookup[K, V]) Get(key K) (V, bool) {
	v, ok := l.entries()[key]
	return v, ok
}

// Len returns the number of entries in the lookup.
func (l *Lookup[K, V]) Len() int {
	return len(l.entries())
}

// All returns every entry, ordered ascending by key.
func (l *Lookup[K, V]) All() []V {
	m := l.entries()
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]V, len(keys))
	for i, k := range keys {
		out[i] = m[k]
	}
	return out
}
