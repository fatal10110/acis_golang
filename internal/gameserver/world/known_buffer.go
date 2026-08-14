package world

import "sync"

// KnownBuffer is a reusable scratch buffer for one tracked object's
// known-list snapshots, letting a hot broadcast path reuse the same grown
// slice across calls instead of allocating a fresh known-list every event.
// The zero value is ready to use. mu serializes Snapshot/Release pairs: a
// snapshot is only valid for the caller holding it, between a Snapshot call
// and its matching Release.
type KnownBuffer struct {
	mu      sync.Mutex
	tracked []Tracked
	free    [][]Tracked
}

type KnownSnapshot struct {
	owner   *KnownBuffer
	tracked []Tracked
}

// SnapshotCopy returns a detached known-list snapshot. ReleaseCopy must be
// called after iteration so concurrent or nested broadcasts cannot overwrite it.
func (b *KnownBuffer) SnapshotCopy(s *State, t Tracked) KnownSnapshot {
	b.mu.Lock()
	b.tracked = s.AppendKnown(b.tracked[:0], t)
	n := len(b.free)
	var copy []Tracked
	if n > 0 {
		copy = b.free[n-1]
		b.free = b.free[:n-1]
	}
	copy = append(copy[:0], b.tracked...)
	clear(b.tracked)
	b.tracked = b.tracked[:0]
	b.mu.Unlock()
	return KnownSnapshot{owner: b, tracked: copy}
}

func (s KnownSnapshot) Tracked() []Tracked { return s.tracked }

func (s KnownSnapshot) Release() {
	clear(s.tracked)
	s.owner.mu.Lock()
	s.owner.free = append(s.owner.free, s.tracked[:0])
	s.owner.mu.Unlock()
}

// Snapshot fills the buffer with every object t currently knows in s and
// returns it, locking the buffer until Release. The caller must call
// Release once done iterating the returned slice.
func (b *KnownBuffer) Snapshot(s *State, t Tracked) []Tracked {
	b.mu.Lock()
	b.tracked = s.AppendKnown(b.tracked[:0], t)
	return b.tracked
}

// Release clears and unlocks the buffer after the caller finishes
// iterating a snapshot returned by Snapshot.
func (b *KnownBuffer) Release() {
	clear(b.tracked)
	b.tracked = b.tracked[:0]
	b.mu.Unlock()
}
