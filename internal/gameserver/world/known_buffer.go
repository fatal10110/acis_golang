package world

import "sync"

// KnownBuffer is a reusable scratch buffer for one tracked object's
// known-list snapshots, letting a hot broadcast path reuse the same grown
// slice across calls instead of allocating a fresh known-list every event.
// The zero value is ready to use.
type KnownBuffer struct {
	mu      sync.Mutex
	tracked []Tracked
	free    [][]Tracked
}

// KnownSnapshot is a detached known-list snapshot returned by SnapshotCopy.
type KnownSnapshot struct {
	owner   *KnownBuffer
	tracked []Tracked
}

// SnapshotCopy returns a detached known-list snapshot. Release must be called
// after iteration so concurrent or nested broadcasts cannot overwrite it.
func (b *KnownBuffer) SnapshotCopy(s *State, t Tracked) KnownSnapshot {
	b.mu.Lock()
	b.tracked = s.AppendKnown(b.tracked[:0], t)
	n := len(b.free)
	var snap []Tracked
	if n > 0 {
		snap = b.free[n-1]
		b.free[n-1] = nil
		b.free = b.free[:n-1]
	}
	snap = append(snap[:0], b.tracked...)
	clear(b.tracked)
	b.tracked = b.tracked[:0]
	b.mu.Unlock()
	return KnownSnapshot{owner: b, tracked: snap}
}

func (s KnownSnapshot) Tracked() []Tracked { return s.tracked }

// Release returns the snapshot storage to its owner. It is safe to call twice.
func (s *KnownSnapshot) Release() {
	if s.owner == nil {
		return
	}
	clear(s.tracked)
	s.owner.mu.Lock()
	s.owner.free = append(s.owner.free, s.tracked[:0])
	s.owner.mu.Unlock()
	s.owner = nil
	s.tracked = nil
}
