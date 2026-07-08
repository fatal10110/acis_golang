package commons

import (
	"cmp"
	"slices"
)

// Lookup is an in-memory id-keyed lookup, built once at boot and read for
// the remainder of the process lifetime: the shape shared by every model
// package's "map id to loaded row" table that overwrites on a duplicate
// key and returns All() sorted ascending by key. The zero value is not
// usable; construct with NewLookup or NewLookupFromMap.
type Lookup[K cmp.Ordered, V any] struct {
	byKey map[K]V
}

// NewLookup returns a Lookup backed by items, keyed by key(item). A later
// entry silently overwrites an earlier one with the same key.
func NewLookup[K cmp.Ordered, V any](items []V, key func(V) K) *Lookup[K, V] {
	l := &Lookup[K, V]{byKey: make(map[K]V, len(items))}
	for _, item := range items {
		l.byKey[key(item)] = item
	}
	return l
}

// NewLookupFromMap returns a Lookup wrapping an already-keyed map, for
// callers that build (and validate or mutate) their map before handing it
// off. m is retained, not copied.
func NewLookupFromMap[K cmp.Ordered, V any](m map[K]V) *Lookup[K, V] {
	return &Lookup[K, V]{byKey: m}
}

// Get returns the value stored under key, or false if none was loaded.
func (l *Lookup[K, V]) Get(key K) (V, bool) {
	v, ok := l.byKey[key]
	return v, ok
}

// Len returns the number of entries in the lookup.
func (l *Lookup[K, V]) Len() int {
	return len(l.byKey)
}

// All returns every entry, ordered ascending by key.
func (l *Lookup[K, V]) All() []V {
	keys := make([]K, 0, len(l.byKey))
	for k := range l.byKey {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]V, len(keys))
	for i, k := range keys {
		out[i] = l.byKey[k]
	}
	return out
}
