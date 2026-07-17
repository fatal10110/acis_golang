package world

import (
	"sync"
	"testing"
)

// gen distinguishes two stubs registered under the same id, so tests can
// tell which registration a lookup returned.
type stubObject struct{ id, gen int32 }

func (s stubObject) ObjectID() int32 { return s.id }

func TestRegion_AddRemoveObjects(t *testing.T) {
	r := newRegion(0, 0)

	if got := r.Objects(); len(got) != 0 {
		t.Fatalf("new region has %d objects, want 0", len(got))
	}

	a := &trackedStub{id: 1}
	b := &trackedStub{id: 2}
	r.Add(a)
	r.Add(b)

	got := r.Objects()
	if len(got) != 2 {
		t.Fatalf("Objects() returned %d entries, want 2", len(got))
	}

	r.Remove(a)
	got = r.Objects()
	if len(got) != 1 || got[0].ObjectID() != 2 {
		t.Fatalf("Objects() after Remove(a) = %+v, want only id 2", got)
	}

	r.Remove(b)
	if got := r.Objects(); len(got) != 0 {
		t.Fatalf("Objects() after removing everything has %d entries, want 0", len(got))
	}
}

func TestRegion_RemoveIgnoresStaleIdentity(t *testing.T) {
	r := newRegion(0, 0)

	stale := &trackedStub{id: 1}
	r.Add(stale)

	fresh := &trackedStub{id: 1}
	r.Add(fresh) // id 1 now points at fresh, e.g. a respawn that reused the id

	// A despawn racing that respawn must not evict fresh.
	r.Remove(stale)

	got := r.Objects()
	if len(got) != 1 || got[0] != Tracked(fresh) {
		t.Fatalf("Objects() = %+v, want only the fresh occupant of id 1", got)
	}
}

func TestRegion_Concurrent(t *testing.T) {
	r := newRegion(0, 0)

	var wg sync.WaitGroup
	for i := int32(0); i < 100; i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			obj := &trackedStub{id: id}
			r.Add(obj)
			r.Objects()
			r.Remove(obj)
		}(i)
	}
	wg.Wait()

	if got := r.Objects(); len(got) != 0 {
		t.Fatalf("Objects() after concurrent add/remove has %d entries, want 0", len(got))
	}
}
