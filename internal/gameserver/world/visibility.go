package world

import (
	"fmt"
	"slices"

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
// around their own. Callbacks run after the subject has entered its destination
// region or left the grid, with the world lock released. Discover and Forget run
// on whichever goroutine drives the region transition, so implementations must
// be safe to call concurrently and return promptly without blocking. While they
// run, the subject's placement is still being delivered: a callback must not
// reposition the subject, or another object whose placement is being
// delivered, since that would wait on itself. Read-only queries such as Knows,
// AppendKnown and RegionActivity are safe. Panics propagate and skip remaining
// callbacks, but region membership remains consistent.
type Observer interface {
	// Discover tells the observer that obj just became visible to it.
	Discover(obj Tracked)
	// Forget tells the observer that obj just left its visible range.
	Forget(obj Tracked)
}

// Player is implemented by tracked objects that are player characters —
// the only objects whose presence keeps a Region active. Entering or
// leaving a region's 3x3 neighborhood as a Player toggles Region.Active
// for the regions that lose or gain a nearby player.
//
// WorldPlayer takes no arguments and returns nothing: it is a pure type
// marker. Implementing it at all is what makes a type count as a Player —
// there is no way to implement it and opt out, unlike a boolean-returning
// method a caller might reasonably expect to report false sometimes.
type Player interface {
	Tracked
	WorldPlayer()
}

// Spawn places t in the world at (x, y, z) facing heading, clamping x and
// y to the world bounds, registers it, and notifies observers around the
// landing region that t entered their sight (and t of everything it now
// sees).
func (s *State) Spawn(t Tracked, x, y, z, heading int) {
	x = min(max(x, MinX), MaxX)
	y = min(max(y, MinY), MaxY)

	p := t.presence()
	s.mu.Lock()
	s.awaitIdleLocked(p)
	p.setPosition(x, y, z)
	p.heading.Store(int64(heading))
	p.visible.Store(true)
	next, _ := s.RegionAt(x, y) // clamped coordinates always land on the grid
	s.relocateAndUnlock(t, next, func() { s.AddObject(t) })
}

// Move updates t's position and, when the new coordinates land in a
// different region, migrates it there, notifying observers that entered or
// left its surroundings. An object that is not visible only gets its
// position updated. The position is updated even when the move fails
// because a visible object was sent outside the world bounds.
func (s *State) Move(t Tracked, x, y, z int) error {
	p := t.presence()
	s.mu.Lock()
	s.awaitIdleLocked(p)
	p.setPosition(x, y, z)
	prev := p.region.Load()
	if prev == nil || !p.visible.Load() {
		s.mu.Unlock()
		return nil
	}
	next, ok := s.RegionAt(x, y)
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("move object %d: (%d, %d) is outside the world bounds", t.ObjectID(), x, y)
	}
	if next == prev {
		s.mu.Unlock()
		return nil
	}
	s.relocateAndUnlock(t, next, nil)
	return nil
}

// Despawn removes t from the world: it leaves its region, observers that
// could see it are told to forget it (and it forgets them), and it is
// dropped from the object registry.
func (s *State) Despawn(t Tracked) {
	p := t.presence()
	s.mu.Lock()
	s.awaitIdleLocked(p)
	p.visible.Store(false)
	s.relocateAndUnlock(t, nil, func() { s.removeObjectIfSame(t) })
}

// DespawnAll removes every object in ts from the world in one pass. Objects
// that share a departure region trigger a single neighbor scan and a single
// Forget per observer instead of one scan per object, which matters when
// many objects expire in the same tick (e.g. co-located ground-item
// cleanup): scanning each neighbor region's contents once per despawn is
// quadratic in same-region batch size. Like Despawn, it updates the grid
// under the world lock and delivers every callback after releasing it.
//
// ponytail: ts must not themselves implement Observer (the reciprocal
// tObs.Forget(o) that relocate does for a single despawning observer isn't
// replicated here) — fine for today's only caller (ground items, which
// never observe), revisit if a future caller despawns Observers in bulk.
func (s *State) DespawnAll(ts []Tracked) {
	s.mu.Lock()
	for slices.ContainsFunc(ts, func(t Tracked) bool { return t.presence().busy }) {
		s.idle.Wait()
	}

	byRegion := make(map[*Region][]Tracked, len(ts))
	for _, t := range ts {
		p := t.presence()
		p.visible.Store(false)
		region := p.region.Load()
		byRegion[region] = append(byRegion[region], t)
	}

	var areaBuf [9]*Region
	var toggles []regionToggle
	var notifications []visibilityNotification
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
		if slices.ContainsFunc(left, func(t Tracked) bool {
			_, ok := t.(Player)
			return ok
		}) {
			for _, r := range areas {
				if s.regionNeighborhoodEmpty(r) && r.setActive(false) {
					toggles = append(toggles, regionToggle{r, false})
				}
			}
		}
		for _, r := range areas {
			for _, o := range r.objects {
				w, ok := o.(Observer)
				if !ok {
					continue
				}
				for _, t := range left {
					if o.ObjectID() != t.ObjectID() {
						notifications = append(notifications, visibilityNotification{w, t, false})
					}
				}
			}
		}
	}
	for _, t := range ts {
		p := t.presence()
		p.region.Store(nil)
		p.busy = true
	}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		for _, t := range ts {
			t.presence().busy = false
		}
		s.idle.Broadcast()
		s.mu.Unlock()
	}()
	for _, notification := range notifications {
		notification.notify()
	}
	for _, tg := range toggles {
		s.notifyActivity(tg.region, tg.active)
	}
	for _, t := range ts {
		s.removeObjectIfSame(t)
	}
}

// awaitIdleLocked waits until no earlier placement of p is still delivering
// its callbacks, so two placements of one object never interleave their
// notifications. The caller holds s.mu for writing; waiting releases it.
func (s *State) awaitIdleLocked(p *Presence) {
	for p.busy {
		s.idle.Wait()
	}
}

// relocateAndUnlock moves t between grid regions: out of its current one, if
// any, and into next, unless nil. The caller holds s.mu for writing and has
// already updated t's position and visibility under it; relocateAndUnlock
// finishes the grid update, releases s.mu, and only then delivers callbacks
// and runs after (when non-nil), keeping t busy until they finish.
//
// Every object in a region that leaves t's surroundings exchanges Forget
// notifications with t, and every object in a region that enters them
// exchanges Discover notifications; regions shared by both neighborhoods
// stay silent. For each affected object the other party is notified before
// t itself. A non-player arriving in an already active or inactive region is
// told that region's activity first; a player's region-activity changes are
// delivered before its visibility notifications.
func (s *State) relocateAndUnlock(t Tracked, next *Region, after func()) {
	p := t.presence()
	_, tIsPlayer := t.(Player)
	prev := p.region.Load()

	var oldAreaBuf, newAreaBuf [9]*Region
	var oldAreas, newAreas []*Region
	if prev != nil && prev.removeIfSame(t.ObjectID(), t) {
		oldAreas = s.AppendNeighbors(oldAreaBuf[:0], prev, 1)
	}
	var arrival regionActivityArrival
	if next != nil {
		arrival = next.add(t)
		newAreas = s.AppendNeighbors(newAreaBuf[:0], next, 1)
	}
	p.region.Store(next)
	// A non-player entering a region that was already active or inactive
	// sees no setActive transition, so it is notified directly.
	notifyArrival := next != nil && !tIsPlayer && prev != nil

	var notificationBuf [16]visibilityNotification
	var notifications []visibilityNotification
	var scratch *RelocateScratch
	n := 0
	if tIsPlayer {
		if owner, ok := t.(relocateScratchOwner); ok {
			scratch = owner.relocateScratch()
		}
		// Same unshared-region skip as appendCrossing.
		for _, r := range oldAreas {
			if !containsRegion(newAreas, r) {
				n += len(r.objects)
			}
		}
		for _, r := range newAreas {
			if !containsRegion(oldAreas, r) {
				n += len(r.objects)
			}
		}
	}
	// The scratch and stack-buffer paths keep separate slice variables so
	// the stack buffer never flows into the heap-resident scratch.
	if scratch != nil {
		if cap(scratch.notifications) < n*2 {
			scratch.notifications = make([]visibilityNotification, 0, n*2)
		}
		scratch.notifications = appendCrossing(scratch.notifications[:0], t, oldAreas, newAreas)
		notifications = scratch.notifications
	} else {
		buf := notificationBuf[:0]
		if cap(buf) < n*2 {
			buf = make([]visibilityNotification, 0, n*2)
		}
		notifications = appendCrossing(buf, t, oldAreas, newAreas)
	}

	var toggleBuf [18]regionToggle
	toggles := toggleBuf[:0]
	if tIsPlayer {
		for _, r := range oldAreas {
			if !containsRegion(newAreas, r) && s.regionNeighborhoodEmpty(r) && r.setActive(false) {
				toggles = append(toggles, regionToggle{r, false})
			}
		}
		for _, r := range newAreas {
			if !containsRegion(oldAreas, r) && r.setActive(true) {
				toggles = append(toggles, regionToggle{r, true})
			}
		}
	}

	if !notifyArrival && len(toggles) == 0 && len(notifications) == 0 && after == nil {
		s.mu.Unlock()
		return
	}
	p.busy = true
	s.mu.Unlock()

	// Deferred so t leaves busy and scratch is reset to zero-value-clean,
	// empty slices on every return path, including a panic propagating out
	// of a callback below (Observer's doc comment: panics propagate and skip
	// remaining callbacks). clear zeroes the full capacity, not just the
	// length this call used, so a shrinking crossing (widest cap held over
	// from a crowded earlier region) can't keep stale Tracked references
	// reachable from the player's scratch past their despawn.
	defer func() {
		if scratch != nil {
			clear(scratch.notifications[:cap(scratch.notifications)])
			scratch.notifications = scratch.notifications[:0]
		}
		s.mu.Lock()
		p.busy = false
		s.idle.Broadcast()
		s.mu.Unlock()
	}()
	if notifyArrival {
		s.notifyArrivalActivity(next, t, arrival)
	}
	for _, tg := range toggles {
		s.notifyActivity(tg.region, tg.active)
	}
	for _, notification := range notifications {
		notification.notify()
	}
	if after != nil {
		after()
	}
}

// appendCrossing appends the notifications t's move from oldAreas to
// newAreas produces: Forget for regions only in oldAreas, then Discover for
// regions only in newAreas. The caller holds s.mu.
func appendCrossing(notes []visibilityNotification, t Tracked, oldAreas, newAreas []*Region) []visibilityNotification {
	notes = appendRegionChange(notes, t, oldAreas, newAreas, false)
	return appendRegionChange(notes, t, newAreas, oldAreas, true)
}

// appendRegionChange appends, for every object in areas outside shared, the
// notification to that object about t and then t's own about that object.
func appendRegionChange(notes []visibilityNotification, t Tracked, areas, shared []*Region, discover bool) []visibilityNotification {
	tObs, tObserves := t.(Observer)
	for _, r := range areas {
		if containsRegion(shared, r) {
			continue
		}
		for _, o := range r.objects {
			if o.ObjectID() == t.ObjectID() {
				continue
			}
			if w, ok := o.(Observer); ok {
				notes = append(notes, visibilityNotification{w, t, discover})
			}
			if tObserves {
				notes = append(notes, visibilityNotification{tObs, o, discover})
			}
		}
	}
	return notes
}

type visibilityNotification struct {
	observer Observer
	object   Tracked
	discover bool
}

func (n visibilityNotification) notify() {
	if n.discover {
		n.observer.Discover(n.object)
		return
	}
	n.observer.Forget(n.object)
}

// regionToggle is a region whose activity flag setActive just flipped,
// queued so its notifyActivity runs after the world lock is released.
type regionToggle struct {
	region *Region
	active bool
}

// notifyActivity tells every object in r that r's activity changed. It
// snapshots r under s.mu and runs the callbacks after releasing it.
func (s *State) notifyActivity(r *Region, active bool) {
	s.mu.Lock()
	objects := r.appendObjects(nil)
	r.activityPending--
	s.mu.Unlock()
	for _, obj := range objects {
		notifyObjectActivity(obj, active)
	}
}

// notifyArrivalActivity tells obj, which arrival placed in r, the activity r
// had then — unless a toggle landed or is still pending since, in which case
// that toggle's notifyActivity reaches obj instead.
func (s *State) notifyArrivalActivity(r *Region, obj Tracked, arrival regionActivityArrival) {
	s.mu.RLock()
	changed := r.activityVersion != arrival.version
	pending := r.activityPending != 0
	active := r.Active()
	s.mu.RUnlock()
	if !arrival.pending && !changed && !pending {
		notifyObjectActivity(obj, active)
	}
}

// regionNeighborhoodEmpty reports whether r and its 3x3 neighborhood
// currently hold no players. The caller holds s.mu.
func (s *State) regionNeighborhoodEmpty(r *Region) bool {
	var buf [9]*Region
	for _, n := range s.AppendNeighbors(buf[:0], r, 1) {
		if n.playersCount != 0 {
			return false
		}
	}
	return true
}

// RegionActivity reports whether t is currently placed on the world grid,
// and whether that Region is active — one with a player somewhere in its
// 3x3 neighborhood. Scheduled per-object work (AI, follow, route walking)
// calls this to skip objects in regions with no nearby player. An object
// off the grid is not placed and is never active.
func (s *State) RegionActivity(t Tracked) (placed, active bool) {
	r := t.presence().currentRegion()
	if r == nil {
		return false, false
	}
	return true, r.Active()
}

// RegionActive reports whether t currently sits in an active Region.
func (s *State) RegionActive(t Tracked) bool {
	_, active := s.RegionActivity(t)
	return active
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
	s.mu.RLock()
	defer s.mu.RUnlock()
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
// in 3D, excluding t itself, widened by each bodied side's collision
// radius. The search widens to as many region rings as the radius spans,
// and a radius of -1 matches every object in the searched regions. It does
// nothing when t is off the grid.
func (s *State) ForEachKnownInRadius(t Tracked, radius int, fn func(Tracked)) {
	s.forEachKnownInRadius(t, radius, true, fn)
}

// ForEachKnownInPlainRadius is ForEachKnownInRadius without collision-radius
// widening: it matches a reference check against a plain point distance
// (e.g. EffectConfusion.java:41's distance2D filter), not
// MathUtil.checkIfInRange's body-to-body widening.
func (s *State) ForEachKnownInPlainRadius(t Tracked, radius int, fn func(Tracked)) {
	s.forEachKnownInRadius(t, radius, false, fn)
}

// knownInRadiusObjectCap is the stack buffer for one region's objects during
// a radius scan. 256 covers a crowded single region without spilling; a
// region holding more than 256 objects falls back to a heap slice, reused
// across the remaining regions of the same scan. The array is zeroed per
// call (4 KiB), which costs a sparse scan ~70 ns against a 32-entry buffer
// — paid back from roughly 300 objects up, where the heap spill it
// replaces costs more.
const knownInRadiusObjectCap = 256

func (s *State) forEachKnownInRadius(t Tracked, radius int, widen bool, fn func(Tracked)) {
	r := t.presence().currentRegion()
	if r == nil {
		return
	}

	// regionBuf covers searchDepth up to 3 ((2*3+1)^2 = 49 regions), the
	// deepest live radius today (aggroRange 4096 in the NPC data). A radius
	// beyond that still works via append's normal heap growth.
	var regionBuf [49]*Region
	var objectBuf [knownInRadiusObjectCap]Tracked
	objects := objectBuf[:0]
	for _, region := range s.AppendNeighbors(regionBuf[:0], r, searchDepth(radius)) {
		s.mu.RLock()
		objects = region.appendObjects(objects[:0])
		s.mu.RUnlock()
		for _, o := range objects {
			if o.ObjectID() == t.ObjectID() || !inRange(radius, t, o, widen) {
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

// bodied is implemented by Tracked objects that occupy physical space.
// Objects without it (ground items, static objects) count as points.
type bodied interface {
	CollisionRadius() float64
}

// inRange reports whether a and b are within rng units of each other,
// widened (when widen is true) by each side's collision radius when it has
// one. A rng of -1 means unlimited; any other negative value behaves like
// its absolute value. The comparison stays in float64 space end to end,
// matching MathUtil.checkIfInRange's double totalRadius (MathUtil.java:193-198,
// 214-217): summing collision radii as an int before comparing, as an
// earlier version of this function did, silently truncates fractional
// radii (e.g. 7.5 on female player templates, or Grow-scaled NPC bodies).
func inRange(rng int, a, b Tracked, widen bool) bool {
	if rng == -1 {
		return true
	}
	if rng < 0 {
		rng = -rng
	}
	total := float64(rng)
	if widen {
		if ab, ok := a.(bodied); ok {
			total += ab.CollisionRadius()
		}
		if bb, ok := b.(bodied); ok {
			total += bb.CollisionRadius()
		}
	}

	ax, ay, az := a.presence().Position()
	bx, by, bz := b.presence().Position()
	dx := float64(ax - bx)
	dy := float64(ay - by)
	dz := float64(az - bz)
	return dx*dx+dy*dy+dz*dz <= total*total
}
