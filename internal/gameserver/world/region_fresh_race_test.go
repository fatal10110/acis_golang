package world

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestRegion_concurrentAddRemoveIntoFreshRegion races writers that make a
// fresh region's storage on first add against lock-free scans of it. The
// writers serialize on a mutex standing in for State.mu, as every writer
// does; the scans take only the region lock. Run with -race.
func TestRegion_concurrentAddRemoveIntoFreshRegion(t *testing.T) {
	r := newRegion(7, 8)
	var worldMu sync.Mutex
	var done atomic.Bool
	var writers, readers sync.WaitGroup
	for range 2 {
		readers.Go(func() {
			for !done.Load() {
				for _, o := range r.appendObjectsExcept(nil, 1) {
					_ = o.ObjectID()
				}
				for _, p := range r.appendPlayersExcept(nil, 1) {
					if !isPlayer(p) {
						t.Error("players held a non-player")
						return
					}
				}
			}
		})
	}
	for g := range 4 {
		writers.Go(func() {
			for i := range 200 {
				id := int32(g*1000 + i)
				var obj Tracked = &regionTestObject{id: id}
				if i%2 == 0 {
					obj = &regionTestPlayer{regionTestObject{id: id}}
				}
				worldMu.Lock()
				r.add(obj)
				worldMu.Unlock()
				if i%3 != 0 {
					worldMu.Lock()
					r.removeIfSame(id, obj)
					worldMu.Unlock()
				}
			}
		})
	}
	writers.Wait()
	done.Store(true)
	readers.Wait()

	players := 0
	for i, o := range r.objects {
		if r.index[o.ObjectID()] != i {
			t.Fatalf("index[%d] = %d, want %d", o.ObjectID(), r.index[o.ObjectID()], i)
		}
		if isPlayer(o) {
			players++
		}
	}
	if len(r.index) != len(r.objects) || len(r.players) != players {
		t.Fatalf("index %d, objects %d, players %d (want %d)", len(r.index), len(r.objects), len(r.players), players)
	}
}

// TestState_concurrentSpawnDespawnIntoFreshRegion is the same race through
// the world: goroutines spawn and despawn objects into an untouched region
// while an observer next to it broadcasts from its known players. Run with
// -race.
func TestState_concurrentSpawnDespawnIntoFreshRegion(t *testing.T) {
	s := New()
	x, y := regionCenter(60, 60)
	ox, _ := regionCenter(61, 60)
	observer := &regionTestObject{id: 1}
	s.Spawn(observer, ox, y, 0, 0)
	var done atomic.Bool
	var writers, readers sync.WaitGroup
	readers.Go(func() {
		for !done.Load() {
			for p := range s.KnownPlayers(observer) {
				_ = p.ObjectID()
			}
			_ = s.AppendKnown(nil, observer)
		}
	})
	for g := range 4 {
		writers.Go(func() {
			for i := range 100 {
				id := int32(10000 + g*1000 + i)
				var obj Tracked = &regionTestObject{id: id}
				if i%2 == 0 {
					obj = &regionTestPlayer{regionTestObject{id: id}}
				}
				s.Spawn(obj, x, y, 0, 0)
				s.Despawn(obj)
			}
		})
	}
	writers.Wait()
	done.Store(true)
	readers.Wait()
	if got := objectIDs(s.AppendKnown(nil, observer)); len(got) != 0 {
		t.Fatalf("observer still knows %v after every despawn", got)
	}
}
