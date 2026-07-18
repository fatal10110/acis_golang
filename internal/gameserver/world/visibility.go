package world

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
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
	s.AddObject(t)
	s.relocate(t, next)
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

	s.removeObjectIfSame(t)
}

// DespawnAll removes every object in ts from the world in one pass. Objects
// that share a departure region trigger a single neighbor scan and a single
// Forget per observer instead of one scan per object, which matters when
// many objects expire in the same tick (e.g. co-located ground-item
// cleanup): scanning and copying each neighbor region's contents once per
// despawn is quadratic in same-region batch size.
//
// ponytail: ts must not themselves implement Observer (the reciprocal
// tObs.Forget(o) that relocate does for a single despawning observer isn't
// replicated here) — fine for today's only caller (ground items, which
// never observe), revisit if a future caller despawns Observers in bulk.
func (s *State) DespawnAll(ts []Tracked) {
	byRegion := make(map[*Region][]Tracked, len(ts))
	for _, t := range ts {
		p := t.presence()
		p.mu.Lock()
		p.visible = false
		region := p.region
		p.mu.Unlock()
		byRegion[region] = append(byRegion[region], t)
	}

	var areaBuf [9]*Region
	var objectBuf []Tracked
	for region, group := range byRegion {
		if region == nil {
			continue
		}
		left := group[:0]
		for _, t := range group {
			if region.removeIfSame(t.ObjectID(), t) {
				left = append(left, t)
			}
		}
		if len(left) == 0 {
			continue
		}
		areas := s.AppendNeighbors(areaBuf[:0], region, 1)
		for _, r := range areas {
			objectBuf = r.AppendObjects(objectBuf[:0])
			for _, o := range objectBuf {
				w, ok := o.(Observer)
				if !ok {
					continue
				}
				for _, t := range left {
					if o.ObjectID() != t.ObjectID() {
						w.Forget(t)
					}
				}
			}
		}
	}

	for _, t := range ts {
		p := t.presence()
		p.mu.Lock()
		p.region = nil
		p.mu.Unlock()
		s.removeObjectIfSame(t)
	}
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

	var oldAreaBuf, newAreaBuf [9]*Region
	var oldAreas, newAreas []*Region
	if prev != nil && prev.removeIfSame(t.ObjectID(), t) {
		oldAreas = s.AppendNeighbors(oldAreaBuf[:0], prev, 1)
	}
	if next != nil {
		next.Add(t)
		newAreas = s.AppendNeighbors(newAreaBuf[:0], next, 1)
	}

	tObs, tObserves := t.(Observer)
	var objectBuf [32]Tracked
	objects := objectBuf[:0]

	for _, r := range oldAreas {
		if containsRegion(newAreas, r) {
			continue
		}
		objects = r.AppendObjects(objects[:0])
		for _, o := range objects {
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
		objects = r.AppendObjects(objects[:0])
		for _, o := range objects {
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
	var buf [32]Tracked
	for _, o := range s.AppendKnown(buf[:0], t) {
		fn(o)
	}
}

// AppendKnown appends every object in t's surrounding regions to out,
// excluding t itself. It does nothing when t is off the grid. Reusing out lets
// hot broadcast paths keep one grown snapshot buffer instead of allocating a
// fresh known-list slice per event.
func (s *State) AppendKnown(out []Tracked, t Tracked) []Tracked {
	r := t.presence().currentRegion()
	if r == nil {
		return out
	}
	var regionBuf [9]*Region
	for _, region := range s.AppendNeighbors(regionBuf[:0], r, 1) {
		out = region.appendObjectsExcept(out, t.ObjectID())
	}
	return out
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

	var regionBuf [9]*Region
	var objectBuf [32]Tracked
	objects := objectBuf[:0]
	for _, region := range s.AppendNeighbors(regionBuf[:0], r, searchDepth(radius)) {
		objects = region.AppendObjects(objects[:0])
		for _, o := range objects {
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

// inRange reports whether a and b are within rng units of each other.
// A rng of -1 means unlimited; any other negative value behaves like its
// absolute value.
// Objects that occupy physical space will eventually widen the allowance
// by their body radius; until then both bodies count as points.
func inRange(rng int, a, b Tracked) bool {
	if rng == -1 {
		return true
	}
	if rng < 0 {
		rng = -rng
	}

	ax, ay, az := a.presence().Position()
	bx, by, bz := b.presence().Position()
	return location.In3DRange(ax, ay, az, bx, by, bz, rng)
}
