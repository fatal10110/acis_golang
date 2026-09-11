package world

import "sync/atomic"

// Region is one cell of the world grid. It tracks which objects are
// currently visible within its bounds, and whether it is active — see
// Active.
//
// State.mu guards every field except active, which State writes under
// State.mu and anyone may read lock-free through Active.
type Region struct {
	tileX, tileY int

	objects []Tracked
	index   map[int32]int

	activityVersion uint64
	activityPending uint64
	playersCount    int
	active          atomic.Bool
}

type regionActivityArrival struct {
	version uint64
	pending bool
}

type activeRegionActor interface {
	OnActiveRegion()
}

type inactiveRegionActor interface {
	OnInactiveRegion()
}

func newRegion(tileX, tileY int) *Region {
	return &Region{
		tileX: tileX,
		tileY: tileY,
		index: make(map[int32]int),
	}
}

// Active reports whether r currently has a Player somewhere in its 3x3
// neighborhood. Scheduled per-object work (AI, follow, route walking) is
// expected to skip objects sitting in an inactive region.
func (r *Region) Active() bool {
	return r.active.Load()
}

// setActive flips the active flag to value if it isn't already there,
// reporting whether it changed. The caller holds State.mu and runs
// notifyActivity for a reported change after releasing it.
func (r *Region) setActive(value bool) bool {
	if !r.active.CompareAndSwap(!value, value) {
		return false
	}
	r.activityVersion++
	r.activityPending++
	return true
}

func notifyObjectActivity(obj Tracked, active bool) {
	if active {
		if actor, ok := obj.(activeRegionActor); ok {
			actor.OnActiveRegion()
		}
		return
	}
	if actor, ok := obj.(inactiveRegionActor); ok {
		actor.OnInactiveRegion()
	}
}

// add registers obj as visible within r. A second add under the same id
// replaces the occupant; the set stays unique by id. The caller holds
// State.mu.
func (r *Region) add(obj Tracked) regionActivityArrival {
	id := obj.ObjectID()
	if i, ok := r.index[id]; ok {
		r.objects[i] = obj
	} else {
		r.index[id] = len(r.objects)
		r.objects = append(r.objects, obj)
	}
	if _, ok := obj.(Player); ok {
		r.playersCount++
	}
	return regionActivityArrival{r.activityVersion, r.activityPending != 0}
}

// remove drops the object with the given id from r, if present. The caller
// holds State.mu.
func (r *Region) remove(id int32) {
	i, ok := r.index[id]
	if !ok {
		return
	}
	obj := r.objects[i]
	r.removeAt(i)
	if _, isPlayer := obj.(Player); isPlayer {
		r.playersCount--
	}
}

// removeIfSame drops the object registered under id only if it is still
// obj. A caller that lost a race — e.g. a deferred despawn firing after a
// pickup-and-re-drop already reused id under a different object — gets a
// safe no-op instead of evicting the object that legitimately owns id now.
// The caller holds State.mu.
func (r *Region) removeIfSame(id int32, obj Tracked) bool {
	i, ok := r.index[id]
	if !ok || r.objects[i] != obj {
		return false
	}
	r.removeAt(i)
	if _, isPlayer := obj.(Player); isPlayer {
		r.playersCount--
	}
	return true
}

func (r *Region) removeAt(i int) {
	last := len(r.objects) - 1
	delete(r.index, r.objects[i].ObjectID())
	if i != last {
		moved := r.objects[last]
		r.objects[i] = moved
		r.index[moved.ObjectID()] = i
	}
	r.objects[last] = nil
	r.objects = r.objects[:last]
}

// appendObjects appends every object currently visible within r to out and
// returns the extended slice. The caller holds State.mu for reading.
func (r *Region) appendObjects(out []Tracked) []Tracked {
	return append(out, r.objects...)
}

// appendObjectsExcept is appendObjects without the object registered under
// except. The caller holds State.mu for reading.
func (r *Region) appendObjectsExcept(out []Tracked, except int32) []Tracked {
	i, ok := r.index[except]
	if !ok {
		return append(out, r.objects...)
	}
	return append(append(out, r.objects[:i]...), r.objects[i+1:]...)
}
