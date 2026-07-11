package world

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
)

// Tracked is anything that can be placed on the world grid: an
// identifiable object carrying a Presence.
type Tracked interface {
	worldobject.Object
	presence() *Presence
}

// Observer is implemented by tracked objects that react when another
// object enters or leaves their sight range — the 3x3 block of regions
// around their own. Discover and Forget run on whichever goroutine drives
// the region transition, so implementations must be safe to call
// concurrently.
type Observer interface {
	// Discover tells the observer that obj just became visible to it.
	Discover(obj Tracked)
	// Forget tells the observer that obj just left its visible range.
	Forget(obj Tracked)
}

// Spawn places t in the world at (x, y, z) facing heading, clamping x and
// y to the world bounds, registers it, and notifies observers around the
// landing region that t entered their sight (and t of everything it now
// sees).
func (s *State) Spawn(t Tracked, x, y, z, heading int) {
	x = min(max(x, MinX), MaxX)
	y = min(max(y, MinY), MaxY)

	p := t.presence()
	p.mu.Lock()
	p.x, p.y, p.z, p.heading = x, y, z, heading
	p.visible = true
	p.mu.Unlock()

	next, _ := s.RegionAt(x, y) // clamped coordinates always land on the grid
	s.relocate(t, next)

	s.AddObject(t)
}

// Move updates t's position and, when the new coordinates land in a
// different region, migrates it there, notifying observers that entered or
// left its surroundings. An object that is not visible only gets its
// position updated. The position is updated even when the move fails
// because a visible object was sent outside the world bounds.
func (s *State) Move(t Tracked, x, y, z int) error {
	p := t.presence()
	p.mu.Lock()
	p.x, p.y, p.z = x, y, z
	visible := p.region != nil && p.visible
	prev := p.region
	p.mu.Unlock()

	if !visible {
		return nil
	}

	next, ok := s.RegionAt(x, y)
	if !ok {
		return fmt.Errorf("move object %d: (%d, %d) is outside the world bounds", t.ObjectID(), x, y)
	}
	if next != prev {
		s.relocate(t, next)
	}
	return nil
}

// Despawn removes t from the world: it leaves its region, observers that
// could see it are told to forget it (and it forgets them), and it is
// dropped from the object registry.
func (s *State) Despawn(t Tracked) {
	p := t.presence()
	p.mu.Lock()
	p.visible = false
	p.mu.Unlock()

	s.relocate(t, nil)

	s.RemoveObject(t.ObjectID())
}

// relocate moves t between grid regions: out of its current one, if any,
// and into next, unless nil. Every object in a region that leaves t's
// surroundings exchanges Forget notifications with t, and every object in
// a region that enters them exchanges Discover notifications; regions
// shared by both neighborhoods stay silent. For each affected object the
// other party is notified before t itself.
func (s *State) relocate(t Tracked, next *Region) {
	p := t.presence()
	p.mu.RLock()
	prev := p.region
	p.mu.RUnlock()

	var oldAreas, newAreas []*Region
	if prev != nil {
		prev.Remove(t.ObjectID())
		oldAreas = s.Neighbors(prev, 1)
	}
	if next != nil {
		next.Add(t)
		newAreas = s.Neighbors(next, 1)
	}

	tObs, tObserves := t.(Observer)

	for _, r := range oldAreas {
		if containsRegion(newAreas, r) {
			continue
		}
		for _, o := range r.Objects() {
			if o.ObjectID() == t.ObjectID() {
				continue
			}
			if w, ok := o.(Observer); ok {
				w.Forget(t)
			}
			if tObserves {
				tObs.Forget(o)
			}
		}
	}

	for _, r := range newAreas {
		if containsRegion(oldAreas, r) {
			continue
		}
		for _, o := range r.Objects() {
			if o.ObjectID() == t.ObjectID() {
				continue
			}
			if w, ok := o.(Observer); ok {
				w.Discover(t)
			}
			if tObserves {
				tObs.Discover(o)
			}
		}
	}

	p.mu.Lock()
	p.region = next
	p.mu.Unlock()
}

func containsRegion(regions []*Region, r *Region) bool {
	for _, candidate := range regions {
		if candidate == r {
			return true
		}
	}
	return false
}

// Knows reports whether target currently occupies one of the regions
// surrounding t's own (the 3x3 block) — the range within which the two
// objects see each other. Objects off the grid know nothing.
func Knows(t, target Tracked) bool {
	a := t.presence().currentRegion()
	if a == nil {
		return false
	}
	b := target.presence().currentRegion()
	if b == nil {
		return false
	}
	dx, dy := a.tileX-b.tileX, a.tileY-b.tileY
	return dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1
}

// ForEachKnown calls fn for every object in t's surrounding regions,
// excluding t itself. It does nothing when t is off the grid.
func (s *State) ForEachKnown(t Tracked, fn func(Tracked)) {
	r := t.presence().currentRegion()
	if r == nil {
		return
	}
	for _, region := range s.Neighbors(r, 1) {
		for _, o := range region.Objects() {
			if o.ObjectID() == t.ObjectID() {
				continue
			}
			fn(o)
		}
	}
}

// ForEachKnownInRadius calls fn for every object within radius units of t
// in 3D, excluding t itself. The search widens to as many region rings as
// the radius spans, and a radius of -1 matches every object in the
// searched regions. It does nothing when t is off the grid.
func (s *State) ForEachKnownInRadius(t Tracked, radius int, fn func(Tracked)) {
	r := t.presence().currentRegion()
	if r == nil {
		return
	}

	for _, region := range s.Neighbors(r, searchDepth(radius)) {
		for _, o := range region.Objects() {
			if o.ObjectID() == t.ObjectID() || !inRange(radius, t, o) {
				continue
			}
			fn(o)
		}
	}
}

// searchDepth returns how many region rings a radius search must cover so
// that no object within radius units can sit outside the searched block.
func searchDepth(radius int) int {
	if radius <= regionSize {
		return 1
	}
	return radius/regionSize + 1
}

// inRange reports whether a and b are within rng units of each other,
// comparing squared 3D distances. A rng of -1 means unlimited; any other
// value is squared, so a negative rng behaves like its absolute value.
// Objects that occupy physical space will eventually widen the allowance
// by their body radius; until then both bodies count as points.
func inRange(rng int, a, b Tracked) bool {
	if rng == -1 {
		return true
	}

	ax, ay, az := a.presence().Position()
	bx, by, bz := b.presence().Position()

	dx := int64(ax) - int64(bx)
	dy := int64(ay) - int64(by)
	dz := int64(az) - int64(bz)
	distSq := dx*dx + dy*dy + dz*dz

	return distSq <= int64(rng)*int64(rng)
}
