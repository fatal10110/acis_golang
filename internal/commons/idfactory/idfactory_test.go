package idfactory

import (
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
		log:   logrus.StandardLogger(),
	}
	for _, id := range preUsed {
		a.used[id] = struct{}{}
	}
	a.next = a.nextFreeFrom(a.first)
	return a
}

func TestAllocatorNextID_Sequential(t *testing.T) {
	a := newForTest(100, 200)

	for want := int32(100); want <= 102; want++ {
		if got := a.NextID(); got != want {
			t.Fatalf("NextID() = %d, want %d", got, want)
		}
	}
}

func TestAllocatorNextID_SkipsPreloadedIDs(t *testing.T) {
	a := newForTest(100, 200, 100, 101, 103)

	if got := a.NextID(); got != 102 {
		t.Fatalf("NextID() = %d, want 102 (first gap)", got)
	}
	if got := a.NextID(); got != 104 {
		t.Fatalf("NextID() = %d, want 104 (103 preloaded used)", got)
	}
}

func TestAllocatorReleaseID_NotReusedBehindCursor(t *testing.T) {
	a := newForTest(100, 200)

	for i := 0; i < 3; i++ {
		a.NextID() // 100, 101, 102; cursor now at 103
	}
	a.ReleaseID(100)

	if got := a.NextID(); got != 103 {
		t.Fatalf("NextID() = %d, want 103 (released id 100 behind cursor must not be reused this session)", got)
	}
}

func TestAllocatorReleaseID_ReusedOnceCursorReachesIt(t *testing.T) {
	a := newForTest(100, 200, 100, 105) // 105 preloaded used, ahead of the cursor
	a.ReleaseID(105)                    // freed before the cursor ever reaches it

	for want := int32(101); want <= 104; want++ {
		if got := a.NextID(); got != want {
			t.Fatalf("NextID() = %d, want %d", got, want)
		}
	}
	if got := a.NextID(); got != 105 {
		t.Fatalf("NextID() = %d, want 105 (released ahead of cursor, must be reused once reached)", got)
	}
}

func TestAllocatorReleaseID_InvalidIDIgnored(t *testing.T) {
	a := newForTest(100, 200)
	a.ReleaseID(50) // below first; must not panic or corrupt state

	if got := a.NextID(); got != 100 {
		t.Fatalf("NextID() = %d, want 100 (invalid release must not affect allocation)", got)
	}
}

func TestAllocatorNextID_PanicsWhenExhausted(t *testing.T) {
	a := newForTest(100, 101) // two valid ids: 100, 101
	if got := a.NextID(); got != 100 {
		t.Fatalf("NextID() = %d, want 100", got)
	}

	// Handing out 101 requires precomputing a cursor beyond it, which has
	// nowhere left to go: the very last id in the range can never actually
	// be handed out, it panics instead.
	defer func() {
		if recover() == nil {
			t.Fatal("NextID() consuming the last id in range: expected panic, got none")
		}
	}()
	a.NextID()
}
