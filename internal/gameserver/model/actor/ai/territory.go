package ai

import (
	"sync"
	"time"
)

// TerritoryMemory is what an NPC keeps of being out of its territory from
// one life to the next: whether its last arrival was outside, and the
// deadline of the next firing of the stale-hate sweep that the first such
// arrival armed. The sweep keeps its fixed-rate grid across a death and a
// respawn, and only an arrival back inside ends it, so a new life whose
// first arrival is outside again raises no out-of-territory point. A spawn
// slot owns one and hands it to every life; the zero value is an NPC whose
// last arrival was inside. It is safe for concurrent use.
type TerritoryMemory struct {
	mu      sync.Mutex
	outside bool
	due     time.Time
}

// Load returns whether the last arrival was outside and, if so, the next
// sweep deadline.
func (m *TerritoryMemory) Load() (outside bool, due time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.outside, m.due
}

// Store records whether the last arrival was outside and the next sweep
// deadline (zero for none).
func (m *TerritoryMemory) Store(outside bool, due time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outside, m.due = outside, due
}

// NextOnGrid returns the first deadline after now on the grid of period
// through due, and the delay from now to it.
func NextOnGrid(due, now time.Time, period time.Duration) (time.Time, time.Duration) {
	next := due
	if !next.After(now) {
		next = due.Add((now.Sub(due)/period + 1) * period)
	}
	return next, next.Sub(now)
}
