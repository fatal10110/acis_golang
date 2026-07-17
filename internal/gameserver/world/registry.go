package world

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
)

// registry is a concurrency-safe collection of world objects keyed by an
// externally supplied id. add is a no-op if an entry already exists for the
// given key, so the first registration for an id always wins.
//
// mu guards entries.
type registry struct {
	mu      sync.RWMutex
	entries map[int32]worldobject.Object
}

func newRegistry() *registry {
	return &registry{entries: make(map[int32]worldobject.Object)}
}

func (r *registry) add(key int32, obj worldobject.Object) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[key]; !exists {
		r.entries[key] = obj
	}
}

func (r *registry) remove(key int32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, key)
}

// removeIfCurrent drops the entry for key, but only if obj is still the
// value stored there — a despawn racing a respawn that reused key must not
// evict the new occupant.
func (r *registry) removeIfCurrent(key int32, obj worldobject.Object) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cur, ok := r.entries[key]; ok && cur == obj {
		delete(r.entries, key)
	}
}

func (r *registry) removeAll(keys []int32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, key := range keys {
		delete(r.entries, key)
	}
}

func (r *registry) get(key int32) (worldobject.Object, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	obj, ok := r.entries[key]
	return obj, ok
}

func (r *registry) all() []worldobject.Object {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]worldobject.Object, 0, len(r.entries))
	for _, obj := range r.entries {
		out = append(out, obj)
	}
	return out
}
