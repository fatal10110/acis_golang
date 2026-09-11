package world

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---- from grid_test.go ----
// RegionsX/RegionsY are a known-good vector: the aCis Interlude world grid is
// 176 by 256 regions.
func TestGridDimensions(t *testing.T) {
	if RegionsX != 176 {
		t.Errorf("RegionsX = %d, want 176", RegionsX)
	}
	if RegionsY != 256 {
		t.Errorf("RegionsY = %d, want 256", RegionsY)
	}
}

func TestGrid_RegionAt(t *testing.T) {
	g := NewGrid()

	tests := []struct {
		name   string
		x, y   int
		wantOK bool
		wantTX int
		wantTY int
	}{
		{"min corner", MinX, MinY, true, 0, 0},
		{"max corner", MaxX, MaxY, true, RegionsX - 1, RegionsY - 1},
		{"one below min x", MinX - 1, MinY, false, 0, 0},
		{"one above max x", MaxX + 1, MinY, false, 0, 0},
		{"one below min y", MinX, MinY - 1, false, 0, 0},
		{"one above max y", MinX, MaxY + 1, false, 0, 0},
		{"second region boundary", MinX + regionSize, MinY, true, 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, ok := g.RegionAt(tt.x, tt.y)
			if ok != tt.wantOK {
				t.Fatalf("RegionAt(%d, %d) ok = %v, want %v", tt.x, tt.y, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if r.tileX != tt.wantTX || r.tileY != tt.wantTY {
				t.Errorf("RegionAt(%d, %d) = tile (%d, %d), want (%d, %d)", tt.x, tt.y, r.tileX, r.tileY, tt.wantTX, tt.wantTY)
			}
			if r != g.regions[tt.wantTX][tt.wantTY] {
				t.Errorf("RegionAt(%d, %d) did not return the grid's own Region instance", tt.x, tt.y)
			}
		})
	}
}

func TestGrid_Neighbors(t *testing.T) {
	g := NewGrid()

	tests := []struct {
		name      string
		tileX     int
		tileY     int
		depth     int
		wantCount int
	}{
		{"corner depth 1", 0, 0, 1, 4},
		{"center depth 1", 10, 10, 1, 9},
		{"depth 0 is self only", 10, 10, 0, 1},
		{"edge depth 1", RegionsX - 1, 10, 1, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := g.regions[tt.tileX][tt.tileY]
			neighbors := g.Neighbors(r, tt.depth)
			if len(neighbors) != tt.wantCount {
				t.Errorf("Neighbors(tile %d,%d, depth %d) returned %d regions, want %d", tt.tileX, tt.tileY, tt.depth, len(neighbors), tt.wantCount)
			}

			found := false
			for _, n := range neighbors {
				if n == r {
					found = true
				}
			}
			if !found {
				t.Error("Neighbors did not include the region itself")
			}
		})
	}
}

type regionTestObject struct {
	Presence
	id int32
}

func (o *regionTestObject) ObjectID() int32 { return o.id }

type regionTestPlayer struct {
	regionTestObject
}

func (p *regionTestPlayer) WorldPlayer() {}

func TestRegion_AddReplaceSameID(t *testing.T) {
	r := newRegion(0, 0)
	first := &regionTestObject{id: 7}
	second := &regionTestObject{id: 7}
	r.add(first)
	r.add(second)

	got := r.appendObjects(nil)
	if len(got) != 1 {
		t.Fatalf("appendObjects after same-id add = %d objects, want 1", len(got))
	}
	if got[0] != second {
		t.Fatalf("same-id Add kept %p, want later object %p", got[0], second)
	}
}

func TestRegion_RemoveAndRemoveIfSame(t *testing.T) {
	r := newRegion(0, 0)
	a := &regionTestObject{id: 1}
	b := &regionTestObject{id: 2}
	c := &regionTestObject{id: 3}
	r.add(a)
	r.add(b)
	r.add(c)

	r.remove(2)
	if got := objectIDs(r.appendObjects(nil)); !sameIDs(got, []int32{1, 3}) {
		t.Fatalf("after remove(2) ids = %v, want [1 3]", got)
	}

	r.remove(99)
	if got := objectIDs(r.appendObjects(nil)); !sameIDs(got, []int32{1, 3}) {
		t.Fatalf("remove missing id changed set to %v", got)
	}

	other := &regionTestObject{id: 1}
	if r.removeIfSame(1, other) {
		t.Fatal("removeIfSame dropped a different object sharing the id")
	}
	if !r.removeIfSame(1, a) {
		t.Fatal("removeIfSame did not drop the registered object")
	}
	if got := objectIDs(r.appendObjects(nil)); !sameIDs(got, []int32{3}) {
		t.Fatalf("after removeIfSame ids = %v, want [3]", got)
	}
	if r.removeIfSame(3, a) {
		t.Fatal("removeIfSame succeeded for an object that is not registered")
	}
}

func TestRegion_appendObjectsExcept(t *testing.T) {
	r := newRegion(0, 0)
	r.add(&regionTestObject{id: 1})
	r.add(&regionTestObject{id: 2})
	r.add(&regionTestObject{id: 3})

	got := objectIDs(r.appendObjectsExcept(nil, 2))
	if !sameIDs(got, []int32{1, 3}) {
		t.Fatalf("appendObjectsExcept(2) = %v, want [1 3]", got)
	}
	got = objectIDs(r.appendObjectsExcept(nil, 99))
	if !sameIDs(got, []int32{1, 2, 3}) {
		t.Fatalf("appendObjectsExcept missing id = %v, want [1 2 3]", got)
	}
}

func TestRegion_playersCountFollowsPlayerAddRemove(t *testing.T) {
	r := newRegion(0, 0)
	p := &regionTestPlayer{regionTestObject{id: 10}}
	npc := &regionTestObject{id: 11}
	r.add(p)
	r.add(npc)
	if n := r.playersCount; n != 1 {
		t.Fatalf("playersCount after add player+npc = %d, want 1", n)
	}
	r.remove(11)
	if n := r.playersCount; n != 1 {
		t.Fatalf("playersCount after remove npc = %d, want 1", n)
	}
	if !r.removeIfSame(10, p) {
		t.Fatal("removeIfSame did not drop player")
	}
	if n := r.playersCount; n != 0 {
		t.Fatalf("playersCount after removeIfSame player = %d, want 0", n)
	}
}

func objectIDs(objs []Tracked) []int32 {
	ids := make([]int32, len(objs))
	for i, o := range objs {
		ids[i] = o.ObjectID()
	}
	return ids
}

func sameIDs(got, want []int32) bool {
	if len(got) != len(want) {
		return false
	}
	counts := make(map[int32]int, len(want))
	for _, id := range want {
		counts[id]++
	}
	for _, id := range got {
		counts[id]--
		if counts[id] < 0 {
			return false
		}
	}
	return true
}

func knownListFixture(tb testing.TB, n int) (*State, Tracked) {
	tb.Helper()
	return knownListCrowd(tb, n, true)
}

func knownListFixtureOneRegion(tb testing.TB, n int) (*State, Tracked) {
	tb.Helper()
	return knownListCrowd(tb, n, false)
}

func knownListCrowd(tb testing.TB, n int, spread bool) (*State, Tracked) {
	tb.Helper()
	s := New()
	observer := &regionTestObject{id: 1}
	s.Spawn(observer, 0, 0, 0, 0)
	for i := 0; i < n; i++ {
		o := &regionTestObject{id: int32(i + 2)}
		x, y := 0, 0
		if spread {
			bucket := i % 9
			x = (bucket%3 - 1) * regionSize
			y = (bucket/3 - 1) * regionSize
		}
		s.Spawn(o, x, y, 0, 0)
	}
	return s, observer
}

func BenchmarkAppendKnown(b *testing.B) {
	for _, n := range []int{50, 300, 1500} {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			s, observer := knownListFixture(b, n)
			var buf []Tracked
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buf = s.AppendKnown(buf[:0], observer)
			}
		})
	}
}

func TestForEachKnownInRadius_DeepSearchNoAlloc(t *testing.T) {
	s, observer := knownListFixture(t, 50)
	// radius 4097 -> searchDepth 3 -> (2*3+1)^2 = 49 regions, the deepest
	// live radius (issue #2286): regionBuf must not spill to the heap.
	allocs := testing.AllocsPerRun(20, func() {
		s.ForEachKnownInRadius(observer, 4097, func(Tracked) {})
	})
	if allocs != 0 {
		t.Fatalf("ForEachKnownInRadius at searchDepth 3: got %v allocs/op, want 0", allocs)
	}
}

func BenchmarkForEachKnownInRadius(b *testing.B) {
	for _, n := range []int{50, 300, 1500} {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			s, observer := knownListFixture(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.ForEachKnownInRadius(observer, -1, func(Tracked) {})
			}
		})
	}
	// inCap fits knownInRadiusObjectCap in one region; spill is one region over the cap.
	for _, tc := range []struct {
		name string
		n    int
	}{
		{"inCap", 63},
		{"spill", 400},
	} {
		b.Run(tc.name, func(b *testing.B) {
			s, observer := knownListFixtureOneRegion(b, tc.n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.ForEachKnownInRadius(observer, -1, func(Tracked) {})
			}
		})
	}
}

// lockProbe records every callback it receives and fails the test if the
// world lock is held while one runs.
type lockProbe struct {
	Presence
	id    int32
	s     *State
	t     *testing.T
	calls atomic.Int32
}

func (o *lockProbe) ObjectID() int32   { return o.id }
func (o *lockProbe) Discover(Tracked)  { o.check("Discover") }
func (o *lockProbe) Forget(Tracked)    { o.check("Forget") }
func (o *lockProbe) OnActiveRegion()   { o.check("OnActiveRegion") }
func (o *lockProbe) OnInactiveRegion() { o.check("OnInactiveRegion") }

func (o *lockProbe) check(name string) {
	o.calls.Add(1)
	if !o.s.mu.TryLock() {
		o.t.Errorf("%s on object %d ran with the world lock held", name, o.id)
		return
	}
	o.s.mu.Unlock()
}

type lockProbePlayer struct{ lockProbe }

func (*lockProbePlayer) WorldPlayer() {}

// Every placement path — player and non-player Spawn, Move, Despawn and
// DespawnAll — delivers Discover/Forget and region-activity callbacks only
// after releasing the world lock.
func TestPlacementCallbacksRunWithWorldLockReleased(t *testing.T) {
	s := New()
	probe := func(id int32) *lockProbe { return &lockProbe{id: id, s: s, t: t} }
	player := func(id int32) *lockProbePlayer { return &lockProbePlayer{lockProbe{id: id, s: s, t: t}} }
	nearX, nearY := regionCenter(10, 10)
	farX, farY := regionCenter(40, 40)

	watcher := player(1)
	s.Spawn(watcher, nearX, nearY, 0, 0) // activates the near block
	npc := probe(2)
	s.Spawn(npc, nearX, nearY, 0, 0) // Discover both ways
	_ = s.Move(npc, farX, farY, 0)   // Forget both ways; arrives inactive
	_ = s.Move(npc, nearX, nearY, 0) // Discover; arrival activity
	sleeper := probe(3)
	s.Spawn(sleeper, farX, farY, 0, 0)
	visitor := player(4)
	s.Spawn(visitor, farX, farY, 0, 0) // activates the far block: OnActiveRegion
	s.Despawn(visitor)                 // deactivates it: OnInactiveRegion
	walker := player(5)
	s.Spawn(walker, farX, farY, 0, 0)
	s.DespawnAll([]Tracked{npc, walker}) // Forget to watcher; far block deactivates
	s.Despawn(sleeper)
	s.Despawn(watcher)

	for _, o := range []*lockProbe{&watcher.lockProbe, npc, sleeper, &visitor.lockProbe} {
		if o.calls.Load() == 0 {
			t.Errorf("object %d received no callbacks; the scenario no longer exercises it", o.id)
		}
	}
}

// gatedObserver blocks inside Discover of gatedID until gate closes and
// records the order of the notifications it gets about that object.
type gatedObserver struct {
	Presence
	gatedID int32
	entered chan struct{}
	gate    chan struct{}
	mu      sync.Mutex
	events  []string
}

func (o *gatedObserver) ObjectID() int32 { return 1 }

func (o *gatedObserver) Discover(obj Tracked) {
	if obj.ObjectID() != o.gatedID {
		return
	}
	close(o.entered)
	<-o.gate
	o.record("discover")
}

func (o *gatedObserver) Forget(obj Tracked) {
	if obj.ObjectID() == o.gatedID {
		o.record("forget")
	}
}

func (o *gatedObserver) record(event string) {
	o.mu.Lock()
	o.events = append(o.events, event)
	o.mu.Unlock()
}

// A second placement of an object waits until the first finished delivering
// its callbacks, so an observer never sees them out of order, while the rest
// of the world keeps moving in the meantime.
func TestPlacementWaitsForSameObjectCallbacksOnly(t *testing.T) {
	s := New()
	observer := &gatedObserver{gatedID: 2, entered: make(chan struct{}), gate: make(chan struct{})}
	s.Spawn(observer, 0, 0, 0, 0)

	subject := &regionTestObject{id: 2}
	spawned := make(chan struct{})
	go func() {
		s.Spawn(subject, 0, 0, 0, 0)
		close(spawned)
	}()
	<-observer.entered

	// A move that stays in the subject's region normally skips the world
	// lock; while the subject is busy it must wait like any placement.
	moved := make(chan struct{})
	go func() {
		_ = s.Move(subject, 5, 5, 0)
		close(moved)
	}()
	despawned := make(chan struct{})
	go func() {
		s.Despawn(subject)
		close(despawned)
	}()
	select {
	case <-moved:
		t.Fatal("same-region Move finished while the object's Spawn callbacks were still running")
	case <-despawned:
		t.Fatal("Despawn finished while the object's Spawn callbacks were still running")
	case <-time.After(20 * time.Millisecond):
	}

	farX, farY := regionCenter(40, 40)
	other := &regionTestObject{id: 3}
	s.Spawn(other, farX, farY, 0, 0)
	if err := s.Move(other, farX+100, farY, 0); err != nil {
		t.Fatal(err)
	}
	s.Despawn(other)
	_ = s.AppendKnown(nil, observer)

	close(observer.gate)
	<-spawned
	<-moved
	<-despawned

	observer.mu.Lock()
	defer observer.mu.Unlock()
	if !slices.Equal(observer.events, []string{"discover", "forget"}) {
		t.Fatalf("observer events = %v, want [discover forget]", observer.events)
	}
	if _, ok := s.Object(subject.id); ok {
		t.Fatal("subject still registered after Despawn")
	}
}

// Concurrent placements and scans leave every placed object in exactly the
// region its position maps to, with no data race.
func TestConcurrentPlacementsKeepGridConsistent(t *testing.T) {
	s := New()
	watchers := make([]*relocateBenchPlayer, 4)
	for i := range watchers {
		x, y := regionCenter(10+i, 10)
		watchers[i] = &relocateBenchPlayer{id: int32(1000 + i)}
		s.Spawn(watchers[i], x, y, 0, 0)
	}

	const workers, objectsPerWorker, rounds = 8, 8, 50
	var wg sync.WaitGroup
	for w := range workers {
		objs := make([]*regionTestObject, objectsPerWorker)
		for i := range objs {
			objs[i] = &regionTestObject{id: int32(w*objectsPerWorker + i + 1)}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range rounds {
				for i, o := range objs {
					x, y := regionCenter(9+(r+i)%6, 9+(w+r)%3)
					switch (r + i) % 4 {
					case 0:
						s.Spawn(o, x, y, 0, 0)
					case 1, 2:
						_ = s.Move(o, x, y, 0)
					case 3:
						s.Despawn(o)
					}
				}
				if r%10 == 9 {
					batch := make([]Tracked, len(objs))
					for i, o := range objs {
						batch[i] = o
					}
					s.DespawnAll(batch)
				}
			}
		}()
	}
	// Players walk back and forth across region borders, so region
	// activation races between concurrent player relocations. A second
	// goroutine per walker jitters it in place, racing the lock-free
	// same-region Move against the crossings.
	walkers := make([]*relocateBenchPlayer, 4)
	for i := range walkers {
		x, y := regionCenter(9+i, 11)
		walkers[i] = &relocateBenchPlayer{id: int32(2000 + i)}
		s.Spawn(walkers[i], x, y, 0, 0)
	}
	for i, walker := range walkers {
		for lane := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for r := range rounds * 2 {
					if lane == 0 {
						x, y := regionCenter(9+(i+r)%4, 11)
						_ = s.Move(walker, x, y, 0)
					} else {
						x, y, _ := walker.Position()
						_ = s.Move(walker, x+1-2*(r%2), y, 0)
					}
				}
			}()
		}
	}
	for _, watcher := range watchers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var buf []Tracked
			for range rounds * 4 {
				buf = s.AppendKnown(buf[:0], watcher)
				s.ForEachKnownInRadius(watcher, 4096, func(o Tracked) { _, _, _ = o.presence().Position() })
				for _, o := range buf {
					_ = Knows(watcher, o)
					_, _ = s.RegionActivity(o)
				}
			}
		}()
	}
	wg.Wait()

	s.mu.RLock()
	defer s.mu.RUnlock()
	placed := 0
	for x := range RegionsX {
		for y := range RegionsY {
			r := s.regions[x][y]
			for _, o := range r.objects {
				placed++
				px, py, _ := o.presence().Position()
				if at, _ := s.RegionAt(px, py); at != r || o.presence().currentRegion() != r {
					t.Errorf("object %d sits in region (%d,%d) but its position maps elsewhere", o.ObjectID(), x, y)
				}
			}
		}
	}
	for x := range RegionsX {
		for y := range RegionsY {
			r := s.regions[x][y]
			if want := !s.regionNeighborhoodEmpty(r); r.Active() != want {
				t.Errorf("region (%d,%d) Active() = %v, want %v from its neighborhood's players", x, y, r.Active(), want)
			}
		}
	}
	registered := 0
	for _, o := range s.Objects() {
		if _, ok := o.(Tracked); ok && o.(Tracked).presence().currentRegion() != nil {
			registered++
		}
	}
	if placed != registered {
		t.Errorf("grid holds %d objects, registry tracks %d placed ones", placed, registered)
	}
}

// DespawnAll of an object whose Spawn callbacks are still running waits for
// them, so its Forget never overtakes the Discover.
func TestDespawnAllWaitsForBusyBatchMember(t *testing.T) {
	s := New()
	observer := &gatedObserver{gatedID: 2, entered: make(chan struct{}), gate: make(chan struct{})}
	s.Spawn(observer, 0, 0, 0, 0)

	subject := &regionTestObject{id: 2}
	spawned := make(chan struct{})
	go func() {
		s.Spawn(subject, 0, 0, 0, 0)
		close(spawned)
	}()
	<-observer.entered

	bystander := &regionTestObject{id: 3}
	s.Spawn(bystander, 0, 0, 0, 0)
	despawned := make(chan struct{})
	go func() {
		s.DespawnAll([]Tracked{bystander, subject})
		close(despawned)
	}()
	select {
	case <-despawned:
		t.Fatal("DespawnAll finished while a batch member's Spawn callbacks were still running")
	case <-time.After(20 * time.Millisecond):
	}
	close(observer.gate)
	<-spawned
	<-despawned

	observer.mu.Lock()
	defer observer.mu.Unlock()
	if !slices.Equal(observer.events, []string{"discover", "forget"}) {
		t.Fatalf("observer events = %v, want [discover forget]", observer.events)
	}
}

// blockingForgetObserver blocks inside the first Forget until gate closes.
type blockingForgetObserver struct {
	Presence
	entered chan struct{}
	gate    chan struct{}
	once    sync.Once
}

func (o *blockingForgetObserver) ObjectID() int32  { return 1 }
func (o *blockingForgetObserver) Discover(Tracked) {}
func (o *blockingForgetObserver) Forget(Tracked) {
	o.once.Do(func() {
		close(o.entered)
		<-o.gate
	})
}

// A placement of an object DespawnAll is still delivering Forgets for waits
// until the batch is done, so the respawn's Discover comes after the Forget.
func TestPlacementWaitsForDespawnAllBatch(t *testing.T) {
	s := New()
	observer := &blockingForgetObserver{entered: make(chan struct{}), gate: make(chan struct{})}
	s.Spawn(observer, 0, 0, 0, 0)
	member := &regionTestObject{id: 2}
	s.Spawn(member, 0, 0, 0, 0)

	despawned := make(chan struct{})
	go func() {
		s.DespawnAll([]Tracked{member})
		close(despawned)
	}()
	<-observer.entered

	respawned := make(chan struct{})
	go func() {
		s.Spawn(member, 0, 0, 0, 0)
		close(respawned)
	}()
	select {
	case <-respawned:
		t.Fatal("Spawn of a batch member finished while DespawnAll was still delivering its Forgets")
	case <-time.After(20 * time.Millisecond):
	}
	close(observer.gate)
	<-despawned
	<-respawned
	if _, ok := s.Object(member.id); !ok {
		t.Fatal("respawned member not registered")
	}
	if !Knows(observer, member) {
		t.Fatal("respawned member not placed next to the observer")
	}
}

// A lock-free Move and a locked placement of the same object never
// interleave: both need the object's placement latch. With the latch held
// from here, a crossing Move must stall before writing anything, and a
// same-region Move must fall back to the locked path and stall the same way;
// each completes consistently once the latch is released.
func TestMoveWaitsForPlacementLatch(t *testing.T) {
	s := New()
	x0, y := regionCenter(10, 10)
	x1, _ := regionCenter(12, 10)
	o := &regionTestObject{id: 1}
	s.Spawn(o, x0, y, 0, 0)

	for _, tc := range []struct {
		name string
		x    int
	}{
		{"crossing", x1},
		{"same region", x1 + 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, _, _ := o.Position()
			if !o.tryLatch() {
				t.Fatal("placement latch already held")
			}
			done := make(chan error, 1)
			go func() { done <- s.Move(o, tc.x, y, 0) }()
			select {
			case <-done:
				o.releaseLatch()
				t.Fatal("Move finished while another writer held the object's placement latch")
			case <-time.After(20 * time.Millisecond):
			}
			if x, _, _ := o.Position(); x != before {
				o.releaseLatch()
				t.Fatalf("Move wrote x=%d while the latch was held, want %d untouched", x, before)
			}
			o.releaseLatch()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			want, _ := s.RegionAt(tc.x, y)
			if x, _, _ := o.Position(); x != tc.x || o.currentRegion() != want {
				t.Fatalf("after release: x=%d in region %p, want x=%d in region %p", x, o.currentRegion(), tc.x, want)
			}
		})
	}
}
