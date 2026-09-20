package task

import (
	"sync"
	"time"
)

type deadlineEntry[V any] struct {
	actor    V
	deadline time.Time
}

// deadlineRegistry is the shared mutex+map structure behind Decay,
// AttackStance, Respawn, and PvPFlags: a map keyed by K holding a value V
// and a deadline. Its own sweeps (tickDueConcurrent, tickExpiry) allocate a
// fresh due/pending partition on every call, so they are safe to call from
// multiple goroutines at once with no other coordination — required by
// Respawn and PvPFlags, whose Tick has no reentrancy guard.
//
// Decay and AttackStance additionally serialize Tick via a reentrancy
// guard, so they use serialDeadlineRegistry instead, which layers a
// reused scratch buffer and the ticking guard on top of this type. Its
// scratch-reusing tickDue is deliberately not a method of deadlineRegistry
// itself, so Respawn/PvPFlags (embedding this type directly) cannot reach
// it and reintroduce the backing-array race that reuse requires a guard
// against.
//
// entries is guarded by mu.
type deadlineRegistry[K comparable, V any] struct {
	mu      sync.Mutex
	entries map[K]deadlineEntry[V]
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

// tickDueConcurrent sweeps entries whose deadline is not after now, removes
// them, and invokes fire for each removed value. The due partition is
// allocated fresh on every call (starting nil, growing only when something
// is actually due) so this is safe to call from multiple goroutines at
// once with no other coordination.
func (r *deadlineRegistry[K, V]) tickDueConcurrent(now time.Time, fire func(V)) {
	r.mu.Lock()
	var due []V
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

// tickPending partitions entries into due (now strictly after deadline,
// passed to expire) and pending (passed to update), each with the deadline
// the sweep read. Matches PvPFlags's blink semantics, where an entry exactly
// at its deadline is still pending, not yet due. Entries stay tracked: a due
// one is removed by expire through expireIf, once it is ready to apply, so a
// refresh in between wins. Like tickDueConcurrent, both partitions are
// allocated fresh on every call, starting nil, so an all-pending or all-due
// tick costs nothing for the partition that stays empty; this is what makes
// it safe to call from multiple goroutines at once with no other
// coordination.
func (r *deadlineRegistry[K, V]) tickPending(now time.Time, expire func(V, time.Time), update func(V, time.Time)) {
	r.mu.Lock()
	var due, pending []deadlineEntry[V]
	for _, entry := range r.entries {
		if now.After(entry.deadline) {
			due = append(due, entry)
			continue
		}
		pending = append(pending, entry)
	}
	r.mu.Unlock()

	for _, entry := range due {
		expire(entry.actor, entry.deadline)
	}
	for _, entry := range pending {
		update(entry.actor, entry.deadline)
	}
}

// expireIf removes key when it is still tracked with exactly deadline, and
// reports whether it did. A refresh that replaced the entry between the
// sweep and this call leaves the fresh entry tracked, so the expiry the
// sweep observed is dropped instead of applied to it.
func (r *deadlineRegistry[K, V]) expireIf(key K, deadline time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[key]
	if !ok || !entry.deadline.Equal(deadline) {
		return false
	}
	delete(r.entries, key)
	return true
}

// hasDeadline reports whether key is tracked with exactly deadline, so a
// caller can drop a transition it computed from a deadline that has since
// been refreshed.
func (r *deadlineRegistry[K, V]) hasDeadline(key K, deadline time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[key]
	return ok && entry.deadline.Equal(deadline)
}

// serialDeadlineRegistry layers a reused scratch buffer and a
// single-goroutine Tick guard on top of deadlineRegistry, for callers that
// already serialize Tick themselves (Decay, AttackStance). Reusing a
// scratch buffer across sweeps is only safe when overlapping sweeps cannot
// happen; beginTick/endTick enforce that.
type serialDeadlineRegistry[K comparable, V any] struct {
	*deadlineRegistry[K, V]
	tickGuard

	scratch []deadlineEntry[V]
}

func newSerialDeadlineRegistry[K comparable, V any]() *serialDeadlineRegistry[K, V] {
	return &serialDeadlineRegistry[K, V]{deadlineRegistry: newDeadlineRegistry[K, V]()}
}

// sweepDue calls fire for every entry whose deadline is not after now,
// leaving each one tracked: the caller applies the expiry through expireIf,
// which drops it if the entry was refreshed in the meantime. The due
// partition is the reused scratch slice, so the caller must hold the
// beginTick/endTick guard for the duration of the call.
func (r *serialDeadlineRegistry[K, V]) sweepDue(now time.Time, fire func(actor V, deadline time.Time)) {
	r.mu.Lock()
	r.scratch = r.scratch[:0]
	for _, entry := range r.entries {
		if now.Before(entry.deadline) {
			continue
		}
		r.scratch = append(r.scratch, entry)
	}
	due := r.scratch
	r.mu.Unlock()

	defer clear(r.scratch)

	for _, entry := range due {
		fire(entry.actor, entry.deadline)
	}
}

// tickDue sweeps entries whose deadline is not after now, removes them, and
// invokes fire for each removed value. The due partition is a reused
// scratch slice, cleared after fire returns (including on panic) so it
// does not retain values between calls. The caller must hold the
// beginTick/endTick guard for the duration of the call.
func (r *serialDeadlineRegistry[K, V]) tickDue(now time.Time, fire func(V)) {
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
