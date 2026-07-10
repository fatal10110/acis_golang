package world

import "sync"

// Region is one cell of the world grid. It tracks which objects are
// currently visible within its bounds.
//
// mu guards objects.
type Region struct {
	tileX, tileY int

	mu      sync.RWMutex
	objects map[int32]Tracked
}

func newRegion(tileX, tileY int) *Region {
	return &Region{
		tileX:   tileX,
		tileY:   tileY,
		objects: make(map[int32]Tracked),
	}
}

// Add registers obj as visible within r.
func (r *Region) Add(obj Tracked) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.objects[obj.ObjectID()] = obj
}

// Remove drops the object with the given id from r, if present.
func (r *Region) Remove(id int32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.objects, id)
}

// Objects returns a snapshot of every object currently visible within r.
func (r *Region) Objects() []Tracked {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tracked, 0, len(r.objects))
	for _, o := range r.objects {
		out = append(out, o)
	}
	return out
}
