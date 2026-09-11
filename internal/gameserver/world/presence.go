package world

import (
	"runtime"
	"sync/atomic"
)

// Presence is an object's footprint on the world grid: its position,
// heading, visibility flag, and the region currently holding it. Embed it
// in any type that enters the world; the zero value is unplaced and
// invisible.
//
// Position, visibility and region are read lock-free from anywhere, so hot
// distance and known-list checks never contend on the world lock. They are
// written only by whoever holds latch: State.Move's lock-free path for a
// move that stays in its region, or a placement holding State.mu for
// everything else. busy marks a placement whose callbacks are still being
// delivered, so a later placement of the same object waits instead of
// interleaving its notifications; it is written under State.mu with latch
// held (set) or under State.mu alone (cleared).
type Presence struct {
	// posSeq is odd while a position write is in progress; readers retry
	// until they see the same even value on both sides of their loads.
	posSeq  atomic.Uint32
	x, y, z atomic.Int64
	heading atomic.Int64
	visible atomic.Bool
	region  atomic.Pointer[Region]

	latch atomic.Bool
	busy  atomic.Bool
}

// presence exposes the embedded footprint to State. Embedding *Presence
// (usually by value, addressed through a pointer receiver) is the only way
// to satisfy Tracked.
func (p *Presence) presence() *Presence { return p }

// Position returns the current world coordinates as one consistent triple.
func (p *Presence) Position() (x, y, z int) {
	for {
		seq := p.posSeq.Load()
		if seq&1 == 0 {
			x, y, z = int(p.x.Load()), int(p.y.Load()), int(p.z.Load())
			if p.posSeq.Load() == seq {
				return x, y, z
			}
		}
		runtime.Gosched()
	}
}

// setPosition publishes new coordinates. The caller holds latch, which is
// what keeps position writers to one at a time.
func (p *Presence) setPosition(x, y, z int) {
	p.posSeq.Add(1)
	p.x.Store(int64(x))
	p.y.Store(int64(y))
	p.z.Store(int64(z))
	p.posSeq.Add(1)
}

// tryLatch takes the exclusive right to write p's placement if it is free.
// A holder never blocks and never takes State.mu while holding it.
func (p *Presence) tryLatch() bool {
	return p.latch.CompareAndSwap(false, true)
}

// acquireLatch takes the placement latch, waiting out a lock-free Move that
// holds it for a few stores. The caller holds State.mu.
func (p *Presence) acquireLatch() {
	for !p.tryLatch() {
		runtime.Gosched()
	}
}

func (p *Presence) releaseLatch() {
	p.latch.Store(false)
}

// X returns the current world X coordinate.
func (p *Presence) X() int {
	x, _, _ := p.Position()
	return x
}

// Y returns the current world Y coordinate.
func (p *Presence) Y() int {
	_, y, _ := p.Position()
	return y
}

// Z returns the current world Z coordinate.
func (p *Presence) Z() int {
	_, _, z := p.Position()
	return z
}

// Heading returns the direction the object faces.
func (p *Presence) Heading() int {
	return int(p.heading.Load())
}

// SetHeading updates the direction the object faces without moving it.
func (p *Presence) SetHeading(heading int) {
	p.heading.Store(int64(heading))
}

// Visible reports whether the object currently sits in a grid region with
// its visibility flag raised, i.e. other objects can see it.
func (p *Presence) Visible() bool {
	return p.region.Load() != nil && p.visible.Load()
}

// currentRegion returns the region holding the object, or nil when the
// object is off the grid.
func (p *Presence) currentRegion() *Region {
	return p.region.Load()
}
