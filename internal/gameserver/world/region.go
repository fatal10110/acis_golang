package world

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
)

// Region is one cell of the world grid. It tracks which objects are
// currently visible within its bounds.
//
// mu guards objects.
type Region struct {
	tileX, tileY int

	mu      sync.RWMutex
	objects map[int32]worldobject.Object
}

func newRegion(tileX, tileY int) *Region {
	return &Region{
		tileX:   tileX,
		tileY:   tileY,
		objects: make(map[int32]worldobject.Object),
	}
}

// Add registers obj as visible within r.
func (r *Region) Add(obj worldobject.Object) {
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
func (r *Region) Objects() []worldobject.Object {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]worldobject.Object, 0, len(r.objects))
	for _, o := range r.objects {
		out = append(out, o)
	}
	return out
}
