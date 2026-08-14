package task

import (
	"sync"
	"sync/atomic"
)

// activeRegistry is the shared mutex+map+ticker-snapshot structure behind
// AI and PositionUpdates: a map keyed by K holding a value V, ticked by
// taking a scratch-slice snapshot of the current values under lock and
// iterating it outside the lock so Add/Remove stay safe to call from within
// a tick's per-actor callback.
//
// entries and scratch are guarded by mu. ticking enforces the documented
// single-goroutine, one-call-at-a-time Tick contract.
type activeRegistry[K comparable, V any] struct {
	mu      sync.Mutex
	entries map[K]V
	scratch []V

	ticking atomic.Bool
}

func newActiveRegistry[K comparable, V any]() *activeRegistry[K, V] {
	return &activeRegistry[K, V]{entries: make(map[K]V)}
}

// add registers value under key, replacing any value already registered.
func (r *activeRegistry[K, V]) add(key K, value V) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[key] = value
}

// remove unregisters key.
func (r *activeRegistry[K, V]) remove(key K) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, key)
}

// contains reports whether key is currently registered.
func (r *activeRegistry[K, V]) contains(key K) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.entries[key]
	return ok
}

// len returns the number of registered entries.
func (r *activeRegistry[K, V]) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

// snapshot copies the current entries into the reused scratch buffer under
// lock and returns it for the caller to range over outside the lock. The
// caller must call release once done iterating, even on panic, so the
// snapshot does not retain values between ticks.
func (r *activeRegistry[K, V]) snapshot() []V {
	r.mu.Lock()
	r.scratch = r.scratch[:0]
	for _, v := range r.entries {
		r.scratch = append(r.scratch, v)
	}
	values := r.scratch
	r.mu.Unlock()
	return values
}

// release clears the scratch buffer's element references after a snapshot
// has been consumed.
func (r *activeRegistry[K, V]) release() {
	clear(r.scratch)
}
