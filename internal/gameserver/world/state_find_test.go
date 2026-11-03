package world

import "testing"

// FindObject returns the first tracked object the predicate accepts, stops
// visiting there, and returns nil when none is accepted.
func TestStateFindObjectStopsAtFirstMatch(t *testing.T) {
	s := New()
	for id := int32(1); id <= 64; id++ {
		s.AddObject(&spawnOrderObject{id: id})
	}

	visited := 0
	got := s.FindObject(func(obj Tracked) bool {
		visited++
		return obj.ObjectID()%2 == 0
	})
	if got == nil || got.ObjectID()%2 != 0 {
		t.Fatalf("FindObject = %v, want an even-id object", got)
	}
	// Half the objects match, so a scan that stopped at the first one
	// visits at most the 32 odd ones before it.
	if visited > 33 {
		t.Fatalf("FindObject visited %d objects after a match, want it to stop at the first", visited)
	}

	visited = 0
	if got := s.FindObject(func(Tracked) bool { visited++; return false }); got != nil {
		t.Fatalf("FindObject with no match = %v, want nil", got)
	}
	if visited != 64 {
		t.Fatalf("FindObject with no match visited %d objects, want all 64", visited)
	}
}

// FindObject copies nothing: a full scan of a populated state allocates
// nothing, unlike Objects, which copies every tracked object.
func TestStateFindObjectAllocatesNothing(t *testing.T) {
	s := New()
	for id := int32(1); id <= 4096; id++ {
		s.AddObject(&spawnOrderObject{id: id})
	}
	want := int32(-1)
	allocs := testing.AllocsPerRun(100, func() {
		if s.FindObject(func(obj Tracked) bool { return obj.ObjectID() == want }) != nil {
			t.Fatal("matched an absent id")
		}
	})
	if allocs != 0 {
		t.Fatalf("FindObject full scan allocated %.0f times per call, want 0", allocs)
	}
}
