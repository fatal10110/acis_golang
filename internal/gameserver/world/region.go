package world

import (
	"sync"
	"sync/atomic"
)

// Region is one cell of the world grid. It tracks which objects are
// currently visible within its bounds, and whether it is active — see
// Active.
//
// mu guards objects and index as one unit. playersCount and active are
// updated outside mu (by State, which coordinates across several regions
// at once during a relocation) so they are atomics rather than fields mu
// also guards.
type Region struct {
	tileX, tileY int

	mu      sync.RWMutex
	objects []Tracked
	index   map[int32]int

	activityMu      sync.Mutex
	activityVersion uint64
	activityPending uint64
	playersCount    atomic.Int32
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
// reporting whether it changed. The caller decides when to run
// notifyActivity for a change it reports — see relocate, which defers that
// work until after releasing regionActivityMu.
func (r *Region) setActive(value bool) bool {
	r.activityMu.Lock()
	defer r.activityMu.Unlock()
	if !r.active.CompareAndSwap(!value, value) {
		return false
	}
	r.activityVersion++
	r.activityPending++
	return true
}

func (r *Region) notifyActivity(active bool) {
	r.activityMu.Lock()
	objects := r.Objects()
	r.activityPending--
	r.activityMu.Unlock()
	for _, obj := range objects {
		notifyObjectActivity(obj, active)
	}
}

func (r *Region) notifyArrivalActivity(obj Tracked, arrival regionActivityArrival) {
	r.activityMu.Lock()
	changed := r.activityVersion != arrival.version
	pending := r.activityPending != 0
	active := r.Active()
	r.activityMu.Unlock()
	if !arrival.pending && !changed && !pending {
		notifyObjectActivity(obj, active)
	}
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

// Add registers obj as visible within r. A second Add under the same id
// replaces the occupant; the set stays unique by id.
func (r *Region) Add(obj Tracked) regionActivityArrival {
	r.activityMu.Lock()
	defer r.activityMu.Unlock()
	r.mu.Lock()
	id := obj.ObjectID()
	if i, ok := r.index[id]; ok {
		r.objects[i] = obj
	} else {
		r.index[id] = len(r.objects)
		r.objects = append(r.objects, obj)
	}
	r.mu.Unlock()
	if _, ok := obj.(Player); ok {
		r.playersCount.Add(1)
	}
	return regionActivityArrival{r.activityVersion, r.activityPending != 0}
}

// Remove drops the object with the given id from r, if present.
func (r *Region) Remove(id int32) {
	r.mu.Lock()
	i, ok := r.index[id]
	var obj Tracked
	if ok {
		obj = r.objects[i]
		r.removeAtLocked(i)
	}
	r.mu.Unlock()
	if ok {
		if _, isPlayer := obj.(Player); isPlayer {
			r.playersCount.Add(-1)
		}
	}
}

// removeIfSame drops the object registered under id only if it is still
// obj. A caller that lost a race — e.g. a deferred despawn firing after a
// pickup-and-re-drop already reused id under a different object — gets a
// safe no-op instead of evicting the object that legitimately owns id now.
func (r *Region) removeIfSame(id int32, obj Tracked) bool {
	r.mu.Lock()
	i, ok := r.index[id]
	if !ok || r.objects[i] != obj {
		r.mu.Unlock()
		return false
	}
	r.removeAtLocked(i)
	r.mu.Unlock()
	if _, isPlayer := obj.(Player); isPlayer {
		r.playersCount.Add(-1)
	}
	return true
}

func (r *Region) removeAtLocked(i int) {
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

// Objects returns a snapshot of every object currently visible within r.
func (r *Region) Objects() []Tracked {
	return r.AppendObjects(nil)
}

// objectCount returns how many objects currently sit in r. relocate uses it
// to pre-size the Discover/Forget notification slice and the per-region
// scan buffer before scanning.
func (r *Region) objectCount() int {
	r.mu.RLock()
	n := len(r.objects)
	r.mu.RUnlock()
	return n
}

// AppendObjects appends a snapshot of every object currently visible within
// r to out and returns the extended slice. Callers that repeatedly scan
// regions can reuse out to avoid one allocation per region.
func (r *Region) AppendObjects(out []Tracked) []Tracked {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append(out, r.objects...)
}

func (r *Region) appendObjectsExcept(out []Tracked, except int32) []Tracked {
	r.mu.RLock()
	defer r.mu.RUnlock()
	i, ok := r.index[except]
	if !ok {
		return append(out, r.objects...)
	}
	return append(append(out, r.objects[:i]...), r.objects[i+1:]...)
}
