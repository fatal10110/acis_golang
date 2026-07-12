package world

import "sync"

// Presence is an object's footprint on the world grid: its position,
// heading, visibility flag, and the region currently holding it. Embed it
// in any type that enters the world; the zero value is unplaced and
// invisible.
//
// mu guards every field. State methods perform all region bookkeeping;
// callers that reposition the same object from several goroutines must
// serialize those calls per object, since a region transition is a
// multi-step operation.
type Presence struct {
	mu      sync.RWMutex
	x, y, z int
	heading int
	visible bool
	region  *Region
}

// presence exposes the embedded footprint to State. Embedding *Presence
// (usually by value, addressed through a pointer receiver) is the only way
// to satisfy Tracked.
func (p *Presence) presence() *Presence { return p }

// Position returns the current world coordinates.
func (p *Presence) Position() (x, y, z int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.x, p.y, p.z
}

// Heading returns the direction the object faces.
func (p *Presence) Heading() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.heading
}

// SetHeading updates the direction the object faces without moving it.
func (p *Presence) SetHeading(heading int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.heading = heading
}

// Visible reports whether the object currently sits in a grid region with
// its visibility flag raised, i.e. other objects can see it.
func (p *Presence) Visible() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.region != nil && p.visible
}

// currentRegion returns the region holding the object, or nil when the
// object is off the grid.
func (p *Presence) currentRegion() *Region {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.region
}
