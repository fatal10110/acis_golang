package task

import (
	"sync"
)

// activeRegistry is the shared mutex+map+ticker-snapshot structure behind
// AI and PositionUpdates: a map keyed by K holding a value V, ticked by
// taking a scratch-slice snapshot of the current values under lock and
// iterating it outside the lock so Add/Remove stay safe to call from within
// a tick's per-actor callback.
//
// entries and scratch are guarded by mu. The embedded tickGuard enforces
// the documented single-goroutine, one-call-at-a-time Tick contract;
// endTick (promoted from tickGuard) only releases that guard, so a
// successful Tick must also call releaseSnapshot to clear scratch — the
// two are separate steps on purpose, since tickGuard is shared with
// serialDeadlineRegistry, whose tickDue clears its own scratch internally
// and has no equivalent of releaseSnapshot.
type activeRegistry[K comparable, V any] struct {
	mu      sync.Mutex
	entries map[K]V
	scratch []V

	tickGuard
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

// releaseSnapshot clears the scratch buffer's element references after a
// snapshot has been consumed. It is distinct from endTick (promoted from
// tickGuard, which only releases the guard) since a Tick must call both.
func (r *activeRegistry[K, V]) releaseSnapshot() {
	clear(r.scratch)
}

// snapshot copies the current entries into the reused scratch buffer under
// lock and returns it for the caller to range over outside the lock. The
// caller must hold the beginTick/endTick guard for the duration of the
// call, since endTick clears this same buffer.
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
