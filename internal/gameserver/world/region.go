package world

import (
	"sync"
	"sync/atomic"
)

// Region is one cell of the world grid. It tracks which objects are
// currently visible within its bounds, and whether it is active — see
// Active.
//
// mu guards objects. playersCount and active are updated outside mu (by
// State, which coordinates across several regions at once during a
// relocation) so they are atomics rather than fields mu also guards.
type Region struct {
	tileX, tileY int

	mu      sync.RWMutex
	objects map[int32]Tracked

	playersCount atomic.Int32
	active       atomic.Bool
}

type activeRegionActor interface {
	OnActiveRegion()
}

type inactiveRegionActor interface {
	OnInactiveRegion()
}

func newRegion(tileX, tileY int) *Region {
	return &Region{
		tileX:   tileX,
		tileY:   tileY,
		objects: make(map[int32]Tracked),
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
	return r.active.CompareAndSwap(!value, value)
}

func (r *Region) notifyActivity(active bool) {
	for _, obj := range r.Objects() {
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

// Add registers obj as visible within r.
func (r *Region) Add(obj Tracked) {
	r.mu.Lock()
	r.objects[obj.ObjectID()] = obj
	r.mu.Unlock()
	if _, ok := obj.(Player); ok {
		r.playersCount.Add(1)
	}
}

// Remove drops the object with the given id from r, if present.
func (r *Region) Remove(id int32) {
	r.mu.Lock()
	obj, ok := r.objects[id]
	if ok {
		delete(r.objects, id)
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
	cur, ok := r.objects[id]
	if !ok || cur != obj {
		r.mu.Unlock()
		return false
	}
	delete(r.objects, id)
	r.mu.Unlock()
	if _, isPlayer := obj.(Player); isPlayer {
		r.playersCount.Add(-1)
	}
	return true
}

// Objects returns a snapshot of every object currently visible within r.
func (r *Region) Objects() []Tracked {
	return r.AppendObjects(nil)
}

// AppendObjects appends a snapshot of every object currently visible within
// r to out and returns the extended slice. Callers that repeatedly scan
// regions can reuse out to avoid one allocation per region.
func (r *Region) AppendObjects(out []Tracked) []Tracked {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, o := range r.objects {
		out = append(out, o)
	}
	return out
}

func (r *Region) appendObjectsExcept(out []Tracked, except int32) []Tracked {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for id, o := range r.objects {
		if id == except {
			continue
		}
		out = append(out, o)
	}
	return out
}
