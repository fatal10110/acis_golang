package idfactory

import (
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
)

// newForTest builds an Allocator over a small id range so exhaustion and
// gap-search behavior can be tested without allocating billions of ids.
func newForTest(first, last int32, preUsed ...int32) *Allocator {
	a := &Allocator{
		used:  make(map[int32]struct{}),
		first: first,
		last:  last,
		next:  first,
		log:   logrus.StandardLogger(),
	}
	for _, id := range preUsed {
		a.used[id] = struct{}{}
	}
	return a
}

func mustNextID(t *testing.T, a *Allocator) int32 {
	t.Helper()
	id, err := a.NextID()
	if err != nil {
		t.Fatalf("NextID() unexpected error: %v", err)
	}
	return id
}

func TestAllocatorNextID_Sequential(t *testing.T) {
	a := newForTest(100, 200)

	for want := int32(100); want <= 102; want++ {
		if got := mustNextID(t, a); got != want {
			t.Fatalf("NextID() = %d, want %d", got, want)
		}
	}
}

func TestAllocatorNextID_SkipsPreloadedIDs(t *testing.T) {
	a := newForTest(100, 200, 100, 101, 103)

	if got := mustNextID(t, a); got != 102 {
		t.Fatalf("NextID() = %d, want 102 (first gap)", got)
	}
	if got := mustNextID(t, a); got != 104 {
		t.Fatalf("NextID() = %d, want 104 (103 preloaded used)", got)
	}
}

func TestAllocatorReleaseID_NotReusedBehindCursor(t *testing.T) {
	a := newForTest(100, 200)

	for i := 0; i < 3; i++ {
		mustNextID(t, a) // 100, 101, 102; cursor now at 103
	}
	a.ReleaseID(100)

	if got := mustNextID(t, a); got != 103 {
		t.Fatalf("NextID() = %d, want 103 (released id 100 behind cursor must not be reused this session)", got)
	}
}

func TestAllocatorReleaseID_ReusedOnceCursorReachesIt(t *testing.T) {
	a := newForTest(100, 200, 100, 105) // 105 preloaded used, ahead of the cursor
	a.ReleaseID(105)                    // freed before the cursor ever reaches it

	for want := int32(101); want <= 104; want++ {
		if got := mustNextID(t, a); got != want {
			t.Fatalf("NextID() = %d, want %d", got, want)
		}
	}
	if got := mustNextID(t, a); got != 105 {
		t.Fatalf("NextID() = %d, want 105 (released ahead of cursor, must be reused once reached)", got)
	}
}

func TestAllocatorReleaseID_InvalidIDIgnored(t *testing.T) {
	a := newForTest(100, 200)
	a.ReleaseID(50) // below first; must not panic or corrupt state

	if got := mustNextID(t, a); got != 100 {
		t.Fatalf("NextID() = %d, want 100 (invalid release must not affect allocation)", got)
	}
}

func TestAllocatorNextID_LastIDInRangeIsUsable(t *testing.T) {
	a := newForTest(100, 101) // two valid ids: 100, 101

	if got := mustNextID(t, a); got != 100 {
		t.Fatalf("NextID() = %d, want 100", got)
	}
	if got := mustNextID(t, a); got != 101 {
		t.Fatalf("NextID() = %d, want 101 (last id in range must still be allocable)", got)
	}
}

func TestAllocatorNextID_ErrorsWhenExhausted(t *testing.T) {
	a := newForTest(100, 100) // single valid id: 100
	mustNextID(t, a)          // consumes the only free id

	_, err := a.NextID()
	if !errors.Is(err, ErrIDSpaceExhausted) {
		t.Fatalf("NextID() on exhausted range: err = %v, want ErrIDSpaceExhausted", err)
	}
}
