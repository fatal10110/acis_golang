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

	r.Add(&trackedStub{id: 1})
	r.Add(&trackedStub{id: 2})

	got := r.Objects()
	if len(got) != 2 {
		t.Fatalf("Objects() returned %d entries, want 2", len(got))
	}

	r.Remove(1)
	got = r.Objects()
	if len(got) != 1 || got[0].ObjectID() != 2 {
		t.Fatalf("Objects() after Remove(1) = %+v, want only id 2", got)
	}

	r.Remove(2)
	if got := r.Objects(); len(got) != 0 {
		t.Fatalf("Objects() after removing everything has %d entries, want 0", len(got))
	}
}

func TestRegion_ActiveToggle(t *testing.T) {
	r := newRegion(0, 0)
	if r.Active() {
		t.Fatal("new region is active, want inactive")
	}

	r.setActive(true)
	if !r.Active() {
		t.Fatal("setActive(true) did not activate the region")
	}

	r.setActive(false)
	if r.Active() {
		t.Fatal("setActive(false) did not deactivate the region")
	}
}

type activeTrackedStub struct {
	trackedStub
	activeCalls   int
	inactiveCalls int
}

func (s *activeTrackedStub) OnActiveRegion() {
	s.activeCalls++
}

func (s *activeTrackedStub) OnInactiveRegion() {
	s.inactiveCalls++
}

func TestRegion_ActiveToggleNotifiesObjectsOncePerTransition(t *testing.T) {
	r := newRegion(0, 0)
	obj := &activeTrackedStub{trackedStub: trackedStub{id: 1}}
	r.Add(obj)
	obj.activeCalls = 0
	obj.inactiveCalls = 0

	r.setActive(true)
	r.setActive(true)

	if obj.activeCalls != 1 {
		t.Fatalf("active calls = %d, want 1", obj.activeCalls)
	}
	if obj.inactiveCalls != 0 {
		t.Fatalf("inactive calls = %d, want 0", obj.inactiveCalls)
	}

	r.setActive(false)
	r.setActive(false)

	if obj.activeCalls != 1 {
		t.Fatalf("active calls after deactivate = %d, want 1", obj.activeCalls)
	}
	if obj.inactiveCalls != 1 {
		t.Fatalf("inactive calls = %d, want 1", obj.inactiveCalls)
	}
}

func TestRegion_Concurrent(t *testing.T) {
	r := newRegion(0, 0)

	var wg sync.WaitGroup
	for i := int32(0); i < 100; i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			r.Add(&trackedStub{id: id})
			r.Objects()
			r.Remove(id)
		}(i)
	}
	wg.Wait()

	if got := r.Objects(); len(got) != 0 {
		t.Fatalf("Objects() after concurrent add/remove has %d entries, want 0", len(got))
	}
}
