package world

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
)

type forgetRecorder struct {
	Presence
	id     int32
	forgot []int32
}

func (o *forgetRecorder) ObjectID() int32    { return o.id }
func (*forgetRecorder) Kind() actor.Kind     { return actor.KindNPC }
func (*forgetRecorder) Discover(Tracked)     {}
func (o *forgetRecorder) Forget(obj Tracked) { o.forgot = append(o.forgot, obj.ObjectID()) }

// An observer despawned in bulk forgets what it knew, as a single despawn
// does, and the objects around it still forget it.
func TestDespawnAllObserverForgetsWhatItKnew(t *testing.T) {
	s := New()
	x, y := regionCenter(10, 10)
	watcher := &forgetRecorder{id: 1}
	item := &regionTestObject{id: 2}
	bystander := &forgetRecorder{id: 3}
	s.Spawn(watcher, x, y, 0, 0)
	s.Spawn(item, x, y, 0, 0)
	s.Spawn(bystander, x, y, 0, 0)

	s.DespawnAll([]Tracked{watcher, item})

	if !slices.Contains(watcher.forgot, item.id) || !slices.Contains(watcher.forgot, bystander.id) {
		t.Fatalf("despawned watcher forgot %v, want %d and %d", watcher.forgot, item.id, bystander.id)
	}
	if !slices.Contains(bystander.forgot, watcher.id) || !slices.Contains(bystander.forgot, item.id) {
		t.Fatalf("bystander forgot %v, want %d and %d", bystander.forgot, watcher.id, item.id)
	}
	if _, ok := s.Object(watcher.id); ok {
		t.Fatal("watcher still tracked after DespawnAll")
	}
	if _, ok := s.Object(item.id); ok {
		t.Fatal("item still tracked after DespawnAll")
	}
}

// A player's relocation scratch drops the capacity a crowded crossing grew
// once a later crossing needs a small fraction of it.
func TestRelocateScratchDropsCrowdCapacity(t *testing.T) {
	s := New()
	x, y := regionCenter(10, 10)
	for i := range 300 {
		s.Spawn(&relocateBenchObserver{id: int32(i + 2)}, x, y, 0, 0)
	}
	p := &relocateBenchPlayer{id: 1}
	s.Spawn(p, x, y, 0, 0)
	if c := cap(p.notifications); c <= relocateScratchKeep {
		t.Fatalf("scratch cap after spawning in a crowd = %d, want above %d", c, relocateScratchKeep)
	}

	// Leaving the crowd still uses most of the buffer; the next crossing,
	// through empty regions, does not.
	farX, farY := regionCenter(30, 30)
	if err := s.Teleport(p, farX, farY, 0); err != nil {
		t.Fatal(err)
	}
	nextX, _ := regionCenter(31, 30)
	if err := s.Move(p, nextX, farY, 0); err != nil {
		t.Fatal(err)
	}
	if c := cap(p.notifications); c != 0 {
		t.Fatalf("scratch cap after an empty crossing = %d, want the crowd buffer dropped", c)
	}
}
