package task

import (
	"sync"
	"sync/atomic"
	"time"
)

type deadlineEntry[V any] struct {
	actor    V
	deadline time.Time
}

// deadlineRegistry is the shared mutex+map+ticker-sweep structure behind
// Decay, AttackStance, Respawn, and PvPFlags: a map keyed by K holding a
// value V and a deadline, with scratch-slice reuse across sweeps so a
// tick's due (and, for tickExpiry, pending) partitions do not allocate.
//
// entries and the scratch buffers are guarded by mu. ticking is available
// for callers that need to enforce a single-goroutine Tick contract; it is
// unused by types that never guarded reentrancy before this registry
// existed.
type deadlineRegistry[K comparable, V any] struct {
	mu      sync.Mutex
	entries map[K]deadlineEntry[V]
	scratch []deadlineEntry[V]

	ticking atomic.Bool
}

func newDeadlineRegistry[K comparable, V any]() *deadlineRegistry[K, V] {
	return &deadlineRegistry[K, V]{entries: make(map[K]deadlineEntry[V])}
}

// add tracks value under key, replacing any deadline already tracked for it.
func (r *deadlineRegistry[K, V]) add(key K, value V, deadline time.Time) {
	r.mu.Lock()
	r.entries[key] = deadlineEntry[V]{actor: value, deadline: deadline}
	r.mu.Unlock()
}

// remove stops tracking key and reports whether it had been tracked.
func (r *deadlineRegistry[K, V]) remove(key K) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, tracked := r.entries[key]
	if tracked {
		delete(r.entries, key)
	}
	return tracked
}

// tracked reports whether key currently has a pending deadline.
func (r *deadlineRegistry[K, V]) tracked(key K) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.entries[key]
	return ok
}

// deadlineOf returns key's pending deadline, if one is tracked.
func (r *deadlineRegistry[K, V]) deadlineOf(key K) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[key]
	if !ok {
		return time.Time{}, false
	}
	return e.deadline, true
}

// len returns the number of tracked entries.
func (r *deadlineRegistry[K, V]) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

// tickDueConcurrent behaves like tickDue but allocates its due partition
// fresh on every call instead of reusing the shared scratch buffer. Use it
// for registries whose Tick has no reentrancy guard (Respawn): tickDue's
// scratch reuse is only safe when the caller serializes Tick calls, since
// two overlapping sweeps would otherwise mutate the same backing array
// while one is still being iterated by the other.
func (r *deadlineRegistry[K, V]) tickDueConcurrent(now time.Time, fire func(V)) {
	r.mu.Lock()
	due := make([]V, 0, len(r.entries))
	for key, entry := range r.entries {
		if now.Before(entry.deadline) {
			continue
		}
		due = append(due, entry.actor)
		delete(r.entries, key)
	}
	r.mu.Unlock()

	for _, actor := range due {
		fire(actor)
	}
}

// tickDue sweeps entries whose deadline is not after now, removes them, and
// invokes fire for each removed value. The due partition is a reused
// scratch slice, cleared after fire returns (including on panic) so it does
// not retain values between calls. Safe only when the caller serializes
// Tick calls (e.g. via a ticking guard); see tickDueConcurrent otherwise.
func (r *deadlineRegistry[K, V]) tickDue(now time.Time, fire func(V)) {
	r.mu.Lock()
	r.scratch = r.scratch[:0]
	for key, entry := range r.entries {
		if now.Before(entry.deadline) {
			continue
		}
		r.scratch = append(r.scratch, entry)
		delete(r.entries, key)
	}
	due := r.scratch
	r.mu.Unlock()

	defer clear(r.scratch)

	for _, entry := range due {
		fire(entry.actor)
	}
}

// tickExpiry partitions entries into due (now strictly after deadline,
// removed and passed to expire) and pending (left tracked, passed with
// their deadline to update). Matches PvPFlags's blink semantics, where an
// entry exactly at its deadline is still pending, not yet due. Like
// tickDueConcurrent, both partitions are allocated fresh on every call
// rather than reusing a shared scratch buffer, since PvPFlags's Tick has no
// reentrancy guard and concurrent sweeps would otherwise race on shared
// backing arrays.
func (r *deadlineRegistry[K, V]) tickExpiry(now time.Time, expire func(V), update func(V, time.Time)) {
	r.mu.Lock()
	due := make([]deadlineEntry[V], 0, len(r.entries))
	var pending []deadlineEntry[V]
	for key, entry := range r.entries {
		if now.After(entry.deadline) {
			due = append(due, entry)
			delete(r.entries, key)
			continue
		}
		pending = append(pending, entry)
	}
	r.mu.Unlock()

	for _, entry := range due {
		expire(entry.actor)
	}
	for _, entry := range pending {
		update(entry.actor, entry.deadline)
	}
}
