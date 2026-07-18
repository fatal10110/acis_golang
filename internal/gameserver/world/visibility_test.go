package world

import (
	"fmt"
	"slices"
	"sync"
	"testing"
)

// trackedStub is a minimal grid occupant.
type trackedStub struct {
	Presence
	id int32
}

func (t *trackedStub) ObjectID() int32 { return t.id }

// sightLog records sight notifications in arrival order, as
// "<observer> <verb> <object>" strings.
type sightLog struct {
	mu      sync.Mutex
	entries []string
}

func (l *sightLog) add(who int32, verb string, obj Tracked) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, fmt.Sprintf("%d %s %d", who, verb, obj.ObjectID()))
}

func (l *sightLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.entries
	l.entries = nil
	return out
}

// observerStub records every sight change it is notified about.
type observerStub struct {
	trackedStub
	log *sightLog
}

func newObserver(id int32, log *sightLog) *observerStub {
	return &observerStub{trackedStub: trackedStub{id: id}, log: log}
}

func (o *observerStub) Discover(obj Tracked) { o.log.add(o.id, "discover", obj) }
func (o *observerStub) Forget(obj Tracked)   { o.log.add(o.id, "forget", obj) }

type observerFuncStub struct {
	trackedStub
	discover func(Tracked)
}

func (o *observerFuncStub) Discover(obj Tracked) {
	if o.discover != nil {
		o.discover(obj)
	}
}

func (o *observerFuncStub) Forget(Tracked) {}

func TestSpawnNotifiesResidentFirstThenArrival(t *testing.T) {
	s := New()
	log := &sightLog{}

	a := newObserver(1, log)
	s.Spawn(a, 0, 0, 0, 0)
	if got := log.take(); len(got) != 0 {
		t.Fatalf("spawn into an empty world logged %v, want nothing", got)
	}
	if !a.Visible() {
		t.Fatal("a.Visible() = false after Spawn")
	}

	// One region east of a: inside a's 3x3 surroundings.
	b := newObserver(2, log)
	s.Spawn(b, 2048, 0, -50, 0)

	want := []string{"1 discover 2", "2 discover 1"}
	if got := log.take(); !slices.Equal(got, want) {
		t.Fatalf("spawn notifications = %v, want %v", got, want)
	}
	if _, ok := s.Object(2); !ok {
		t.Fatal("Object(2) missing after Spawn")
	}
}

func TestSpawnRegistersObjectBeforeDiscover(t *testing.T) {
	s := New()
	observer := &observerFuncStub{trackedStub: trackedStub{id: 1}}
	observer.discover = func(obj Tracked) {
		if _, ok := s.Object(obj.ObjectID()); !ok {
			t.Fatalf("Object(%d) missing during Discover", obj.ObjectID())
		}
	}

	s.Spawn(observer, 0, 0, 0, 0)
	s.Spawn(&trackedStub{id: 2}, 100, 0, 0, 0)
}

func TestSpawnPlainObjectNotifiesOnlyObservers(t *testing.T) {
	s := New()
	log := &sightLog{}

	near := newObserver(1, log)
	s.Spawn(near, 0, 0, 0, 0)
	far := newObserver(2, log)
	s.Spawn(far, 8192, 0, 0, 0)
	log.take()

	obj := &trackedStub{id: 3}
	s.Spawn(obj, 100, 200, 0, 0)

	want := []string{"1 discover 3"}
	if got := log.take(); !slices.Equal(got, want) {
		t.Fatalf("spawn notifications = %v, want %v", got, want)
	}
}

func TestSpawnClampsToWorldBounds(t *testing.T) {
	s := New()

	obj := &trackedStub{id: 1}
	s.Spawn(obj, MaxX+5000, MinY-4000, 123, 7)

	x, y, z := obj.Position()
	if x != MaxX || y != MinY || z != 123 {
		t.Fatalf("Position() = (%d, %d, %d), want (%d, %d, 123)", x, y, z, MaxX, MinY)
	}
	if got := obj.Heading(); got != 7 {
		t.Fatalf("Heading() = %d, want 7", got)
	}
	if !obj.Visible() {
		t.Fatal("Visible() = false after a clamped Spawn")
	}
}

func TestMoveWithinRegionIsSilent(t *testing.T) {
	s := New()
	log := &sightLog{}

	o := newObserver(1, log)
	s.Spawn(o, 0, 0, 0, 0)
	obj := &trackedStub{id: 2}
	s.Spawn(obj, 100, 0, 0, 0)
	log.take()

	if err := s.Move(obj, 200, 300, 10); err != nil {
		t.Fatalf("Move() = %v, want nil", err)
	}
	if got := log.take(); len(got) != 0 {
		t.Fatalf("in-region move logged %v, want nothing", got)
	}
	if x, y, z := obj.Position(); x != 200 || y != 300 || z != 10 {
		t.Fatalf("Position() = (%d, %d, %d), want (200, 300, 10)", x, y, z)
	}
}

func TestMoveBetweenRegionsNotifiesLeavingThenEntering(t *testing.T) {
	s := New()
	log := &sightLog{}

	// Observers two regions to either side of the moving object's path:
	// o1 in the column the object leaves behind, o2 in the one it reaches.
	o1 := newObserver(1, log)
	s.Spawn(o1, 0, 0, 0, 0)
	o2 := newObserver(2, log)
	s.Spawn(o2, 8192, 0, 0, 0)

	mover := newObserver(3, log)
	s.Spawn(mover, 2048, 0, 0, 0)
	log.take()

	// Two-region hop: o1's column leaves the mover's surroundings while
	// o2's enters; the middle column is shared and stays silent.
	if err := s.Move(mover, 6144, 0, 0); err != nil {
		t.Fatalf("Move() = %v, want nil", err)
	}

	want := []string{"1 forget 3", "3 forget 1", "2 discover 3", "3 discover 2"}
	if got := log.take(); !slices.Equal(got, want) {
		t.Fatalf("move notifications = %v, want %v", got, want)
	}
}

func TestMoveOutOfBoundsFailsButRecordsPosition(t *testing.T) {
	s := New()
	log := &sightLog{}

	o := newObserver(1, log)
	s.Spawn(o, 0, 0, 0, 0)
	obj := &trackedStub{id: 2}
	s.Spawn(obj, 100, 0, 0, 0)
	log.take()

	err := s.Move(obj, MaxX+10, 0, 0)
	if err == nil {
		t.Fatal("Move() past the world edge = nil, want an error")
	}
	if x, _, _ := obj.Position(); x != MaxX+10 {
		t.Fatalf("Position() x = %d, want %d", x, MaxX+10)
	}
	if got := log.take(); len(got) != 0 {
		t.Fatalf("failed move logged %v, want nothing", got)
	}
	if !Knows(o, obj) {
		t.Fatal("Knows(o, obj) = false, want the object to stay in its region after a failed move")
	}
}

func TestMoveInvisibleOnlyUpdatesPosition(t *testing.T) {
	s := New()

	obj := &trackedStub{id: 1}
	if err := s.Move(obj, 5, 6, 7); err != nil {
		t.Fatalf("Move() = %v, want nil", err)
	}
	if x, y, z := obj.Position(); x != 5 || y != 6 || z != 7 {
		t.Fatalf("Position() = (%d, %d, %d), want (5, 6, 7)", x, y, z)
	}
	if obj.Visible() {
		t.Fatal("Visible() = true, want false for a never-spawned object")
	}
}

func TestDespawnNotifiesAndUnregisters(t *testing.T) {
	s := New()
	log := &sightLog{}

	o := newObserver(1, log)
	s.Spawn(o, 0, 0, 0, 0)
	obj := &trackedStub{id: 5}
	s.Spawn(obj, 100, 0, 0, 0)
	log.take()

	s.Despawn(obj)

	want := []string{"1 forget 5"}
	if got := log.take(); !slices.Equal(got, want) {
		t.Fatalf("despawn notifications = %v, want %v", got, want)
	}
	if _, ok := s.Object(5); ok {
		t.Fatal("Object(5) still registered after Despawn")
	}
	if obj.Visible() {
		t.Fatal("Visible() = true after Despawn")
	}

	// Despawning again is a harmless no-op.
	s.Despawn(obj)
	if got := log.take(); len(got) != 0 {
		t.Fatalf("second despawn logged %v, want nothing", got)
	}
}

func TestDespawnStaleCallDoesNotEvictNewOccupant(t *testing.T) {
	s := New()
	log := &sightLog{}

	observer := newObserver(1, log)
	s.Spawn(observer, 0, 0, 0, 0)
	a := &trackedStub{id: 5}
	s.Spawn(a, 100, 0, 0, 0)
	log.take()

	// a is legitimately despawned (e.g. picked up)...
	s.Despawn(a)
	log.take()

	// ...and a different object is registered under the same id (e.g.
	// re-dropped and reassigned id 5 by the allocator).
	b := &trackedStub{id: 5}
	s.Spawn(b, 200, 0, 0, 0)
	log.take()

	// A stale despawn call for the old a (e.g. a deferred cleanup task that
	// captured a before the pickup) must not evict b.
	s.Despawn(a)

	if got, ok := s.Object(5); !ok || got != b {
		t.Fatalf("Object(5) = %v, %v; want b to remain registered after a stale Despawn(a)", got, ok)
	}
	if got := log.take(); len(got) != 0 {
		t.Fatalf("stale despawn notified observers of b's departure: %v", got)
	}
	if !b.Visible() {
		t.Fatal("b.Visible() = false after a stale Despawn(a)")
	}
}

func TestDespawnAllNotifiesEachObserverOncePerCoLocatedObject(t *testing.T) {
	s := New()
	log := &sightLog{}

	observer := newObserver(1, log)
	s.Spawn(observer, 0, 0, 0, 0)
	a := &trackedStub{id: 2}
	s.Spawn(a, 50, 0, 0, 0)
	b := &trackedStub{id: 3}
	s.Spawn(b, 60, 0, 0, 0)
	log.take()

	s.DespawnAll([]Tracked{a, b})

	want := []string{"1 forget 2", "1 forget 3"}
	if got := log.take(); !slices.Equal(got, want) {
		t.Fatalf("DespawnAll notifications = %v, want %v", got, want)
	}
	if _, ok := s.Object(2); ok {
		t.Fatal("Object(2) still registered after DespawnAll")
	}
	if _, ok := s.Object(3); ok {
		t.Fatal("Object(3) still registered after DespawnAll")
	}
}

func TestDespawnAllAcrossRegionsNotifiesEachRegionsObservers(t *testing.T) {
	s := New()
	log := &sightLog{}

	near := newObserver(1, log)
	s.Spawn(near, 0, 0, 0, 0)
	far := newObserver(2, log)
	s.Spawn(far, 8192, 0, 0, 0)
	a := &trackedStub{id: 3}
	s.Spawn(a, 50, 0, 0, 0)
	b := &trackedStub{id: 4}
	s.Spawn(b, 8200, 0, 0, 0)
	log.take()

	s.DespawnAll([]Tracked{a, b})

	// Different departure regions are processed in map order, which Go
	// does not guarantee — compare as a set instead of a fixed sequence.
	want := []string{"1 forget 3", "2 forget 4"}
	got := log.take()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("DespawnAll notifications = %v, want %v (any order)", got, want)
	}
}

func TestKnows(t *testing.T) {
	s := New()

	a := &trackedStub{id: 1}
	b := &trackedStub{id: 2}
	if Knows(a, b) {
		t.Fatal("Knows() = true for two unspawned objects")
	}

	s.Spawn(a, 0, 0, 0, 0)
	if Knows(a, b) || Knows(b, a) {
		t.Fatal("Knows() = true when the target is unspawned")
	}

	tests := []struct {
		name string
		x, y int
		want bool
	}{
		{"same region", 100, 100, true},
		{"adjacent east", 2048, 0, true},
		{"adjacent diagonal", 2048, 2048, true},
		{"two regions east", 4096, 0, false},
		{"two regions diagonal", 4096, 4096, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s.Spawn(b, tc.x, tc.y, 0, 0)
			if got := Knows(a, b); got != tc.want {
				t.Errorf("Knows(a, b) = %v, want %v", got, tc.want)
			}
			if got := Knows(b, a); got != tc.want {
				t.Errorf("Knows(b, a) = %v, want %v", got, tc.want)
			}
			s.Despawn(b)
		})
	}

	if Knows(a, b) {
		t.Fatal("Knows() = true after the target despawned")
	}
}

func knownIDs(s *State, t Tracked) []int32 {
	var ids []int32
	s.ForEachKnown(t, func(o Tracked) { ids = append(ids, o.ObjectID()) })
	slices.Sort(ids)
	return ids
}

func TestForEachKnownCoversSurroundingRegionsOnly(t *testing.T) {
	s := New()

	center := &trackedStub{id: 1}
	s.Spawn(center, 0, 0, 0, 0)
	sameRegion := &trackedStub{id: 2}
	s.Spawn(sameRegion, 100, 0, 0, 0)
	adjacent := &trackedStub{id: 3}
	s.Spawn(adjacent, 2048, 0, 0, 0)
	far := &trackedStub{id: 4}
	s.Spawn(far, 8192, 0, 0, 0)

	if got := knownIDs(s, center); !slices.Equal(got, []int32{2, 3}) {
		t.Fatalf("known ids = %v, want [2 3]", got)
	}

	unspawned := &trackedStub{id: 9}
	if got := knownIDs(s, unspawned); len(got) != 0 {
		t.Fatalf("known ids for an unspawned object = %v, want none", got)
	}
}

func TestAppendKnownReusesCallerBuffer(t *testing.T) {
	s := New()

	center := &trackedStub{id: 1}
	s.Spawn(center, 0, 0, 0, 0)
	s.Spawn(&trackedStub{id: 2}, 100, 0, 0, 0)
	s.Spawn(&trackedStub{id: 3}, 2048, 0, 0, 0)
	s.Spawn(&trackedStub{id: 4}, 8192, 0, 0, 0)

	buf := make([]Tracked, 0, 4)
	known := s.AppendKnown(buf[:0], center)
	if len(known) == 0 || &known[0] != &buf[:1][0] {
		t.Fatal("AppendKnown did not reuse caller buffer")
	}
	var ids []int32
	for _, obj := range known {
		ids = append(ids, obj.ObjectID())
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []int32{2, 3}) {
		t.Fatalf("AppendKnown ids = %v, want [2 3]", ids)
	}

	known = s.AppendKnown(known[:0], &trackedStub{id: 99})
	if len(known) != 0 {
		t.Fatalf("AppendKnown unspawned = %v, want none", known)
	}
}

func radiusIDs(s *State, t Tracked, radius int) []int32 {
	var ids []int32
	s.ForEachKnownInRadius(t, radius, func(o Tracked) { ids = append(ids, o.ObjectID()) })
	slices.Sort(ids)
	return ids
}

func TestForEachKnownInRadius(t *testing.T) {
	s := New()

	center := &trackedStub{id: 1}
	s.Spawn(center, 0, 0, 0, 0)
	for id, at := range map[int32][3]int{
		2: {4000, 0, 0},  // one region east, 4000 units away
		3: {4200, 0, 0},  // two regions east, 4200 units away
		4: {0, 0, 4200},  // same region, far below
		5: {100, 0, 100}, // same region, close by
		6: {6000, 0, 0},  // two regions east, 6000 units away
	} {
		s.Spawn(&trackedStub{id: id}, at[0], at[1], at[2], 0)
	}

	tests := []struct {
		name   string
		radius int
		want   []int32
	}{
		// Radius wider than one region ring: the search deepens and the
		// 3D distance decides.
		{"radius 4100", 4100, []int32{2, 5}},
		// Unlimited range still only sweeps the immediate surroundings.
		{"radius -1", -1, []int32{2, 4, 5}},
		// Deep search reaching every spawned object.
		{"radius 10000", 10000, []int32{2, 3, 4, 5, 6}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := radiusIDs(s, center, tc.radius); !slices.Equal(got, tc.want) {
				t.Errorf("ids within %d = %v, want %v", tc.radius, got, tc.want)
			}
		})
	}
}

func TestSearchDepthMatchesOracle(t *testing.T) {
	// Expected values generated by the reference Java implementation's
	// radius-to-region-depth expression, run over these exact radii.
	tests := []struct {
		radius, want int
	}{
		{-600, 1},
		{-1, 1},
		{0, 1},
		{1, 1},
		{600, 1},
		{1024, 1},
		{2047, 1},
		{2048, 1},
		{2049, 2},
		{3000, 2},
		{4095, 2},
		{4096, 3},
		{4097, 3},
		{10000, 5},
		{100000, 49},
	}
	for _, tc := range tests {
		if got := searchDepth(tc.radius); got != tc.want {
			t.Errorf("searchDepth(%d) = %d, want %d", tc.radius, got, tc.want)
		}
	}
}

func TestInRangeMatchesOracle(t *testing.T) {
	// Expected values generated by the reference Java implementation's
	// 3D range check (Z axis included, no collision radius), run over
	// these exact inputs.
	tests := []struct {
		rng                    int
		x1, y1, z1, x2, y2, z2 int
		want                   bool
	}{
		{-1, 0, 0, 0, 100000, 100000, 100000, true},
		{0, 5, 5, 5, 5, 5, 5, true},
		{0, 5, 5, 5, 5, 5, 6, false},
		{100, 0, 0, 0, 100, 0, 0, true},
		{100, 0, 0, 0, 100, 0, 1, false},
		{100, 0, 0, 0, 60, 80, 0, true},
		{600, 0, 0, 0, 400, 300, 200, true},
		{600, 0, 0, 0, 600, 0, 1, false},
		{1000, -500, -500, 0, 300, 100, -200, false},
		{-100, 0, 0, 0, 50, 0, 0, true},
		{600000, 131000, 262000, 16000, -131000, -262000, -16000, true},
		{580000, 131000, 262000, 16000, -131000, -262000, -16000, false},
	}
	for _, tc := range tests {
		a := &trackedStub{id: 1}
		a.x, a.y, a.z = tc.x1, tc.y1, tc.z1
		b := &trackedStub{id: 2}
		b.x, b.y, b.z = tc.x2, tc.y2, tc.z2
		if got := inRange(tc.rng, a, b); got != tc.want {
			t.Errorf("inRange(%d, (%d,%d,%d), (%d,%d,%d)) = %v, want %v",
				tc.rng, tc.x1, tc.y1, tc.z1, tc.x2, tc.y2, tc.z2, got, tc.want)
		}
	}
}

func TestVisibilityConcurrent(t *testing.T) {
	s := New()
	log := &sightLog{} // exercised concurrently; contents not asserted

	const goroutines = 24
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			o := newObserver(int32(i+1), log)
			x := (i % 6) * 1500
			s.Spawn(o, x, 0, 0, 0)
			for step := 1; step <= 5; step++ {
				if err := s.Move(o, x+step*997, step*1200, step); err != nil {
					t.Errorf("Move() = %v, want nil", err)
				}
				s.ForEachKnown(o, func(Tracked) {})
				s.ForEachKnownInRadius(o, 3000, func(Tracked) {})
				_ = o.Visible()
			}
			s.Despawn(o)
		}(i)
	}
	wg.Wait()

	if got := s.Objects(); len(got) != 0 {
		t.Fatalf("Objects() has %d entries after every goroutine despawned, want 0", len(got))
	}
	for x := 0; x <= 14000; x += 1024 {
		for y := 0; y <= 6144; y += 1024 {
			r, ok := s.RegionAt(x, y)
			if !ok {
				t.Fatalf("RegionAt(%d, %d) not on the grid", x, y)
			}
			if got := r.Objects(); len(got) != 0 {
				t.Fatalf("region at (%d, %d) still holds %d objects", x, y, len(got))
			}
		}
	}
}

func TestAppendNeighborsDepthOneUsesCallerBuffer(t *testing.T) {
	s := New()
	r, ok := s.RegionAt(0, 0)
	if !ok {
		t.Fatal("RegionAt(0, 0) failed")
	}

	var buf [9]*Region
	allocs := testing.AllocsPerRun(100, func() {
		out := s.AppendNeighbors(buf[:0], r, 1)
		if len(out) != 9 {
			t.Fatalf("AppendNeighbors depth 1 returned %d regions, want 9", len(out))
		}
	})
	if allocs != 0 {
		t.Fatalf("AppendNeighbors depth 1 allocations/run = %v, want 0", allocs)
	}
}

func TestForEachKnownCommonScanUsesStackBuffers(t *testing.T) {
	s := New()
	center := &trackedStub{id: 1}
	s.Spawn(center, 0, 0, 0, 0)
	for i := int32(2); i <= 18; i++ {
		s.Spawn(&trackedStub{id: i}, int(i%3)*300, int(i/3)*300, 0, 0)
	}

	var seen int
	visit := func(Tracked) { seen++ }
	allocs := testing.AllocsPerRun(100, func() {
		seen = 0
		s.ForEachKnown(center, visit)
		if seen != 17 {
			t.Fatalf("ForEachKnown visited %d objects, want 17", seen)
		}
	})
	if allocs != 0 {
		t.Fatalf("ForEachKnown allocations/run = %v, want 0 for common region density", allocs)
	}
}

func BenchmarkRelocate(b *testing.B) {
	s := New()
	for i := 0; i < 54; i++ {
		x := (i % 9) * 256
		y := (i / 9) * 256
		s.Spawn(&trackedStub{id: int32(i + 2)}, x, y, 0, 0)
	}
	mover := &trackedStub{id: 1}
	s.Spawn(mover, 0, 0, 0, 0)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			if err := s.Move(mover, 4096, 0, 0); err != nil {
				b.Fatal(err)
			}
			continue
		}
		if err := s.Move(mover, 0, 0, 0); err != nil {
			b.Fatal(err)
		}
	}
}
