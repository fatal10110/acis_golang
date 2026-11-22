package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// recordingRegistry records the controllers registered for position ticks.
type recordingRegistry struct {
	active map[PositionUpdater]bool
	adds   int
}

func (r *recordingRegistry) Add(u PositionUpdater) {
	if r.active == nil {
		r.active = make(map[PositionUpdater]bool)
	}
	r.active[u] = true
	r.adds++
}

func (r *recordingRegistry) Remove(u PositionUpdater) { delete(r.active, u) }

// A controller ticks with the registry it was built with: a walk registers
// it for position updates and the arrival takes it off again.
func TestControllerTicksWithRegistryGivenAtConstruction(t *testing.T) {
	mover, clock := newTestMover(t, staticGeo{canMove: true})
	registry := &recordingRegistry{}
	controller, err := NewController(mover, &playerFollowSelf{}, nil, registry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.MoveToLocation(location.Location{X: 300}); err != nil {
		t.Fatal(err)
	}
	if !registry.active[controller] {
		t.Fatalf("walk under way: controller registered = false, want true (adds %d)", registry.adds)
	}
	clock.in.Advance(time.Minute)
	if mover.Moving() {
		t.Fatal("walk still under way after its arrival timer")
	}
	if registry.active[controller] {
		t.Fatal("arrived: controller still registered for position ticks")
	}
}

// switchableQueue is a QueueSource whose queue the test moves.
type switchableQueue struct{ q *sim.Queue }

func (s *switchableQueue) Queue() *sim.Queue { return s.q }

// A mover initialized over its actor's QueueSource follows the queue the
// actor names: a walk started after the actor's queue moved times its
// arrival on the new queue.
func TestCreatureMoveInitFollowsQueueSource(t *testing.T) {
	first, second := newMoveClock(), newMoveClock()
	src := &switchableQueue{q: first.q}
	mover := &CreatureMove{}
	if err := mover.Init(location.Location{}, 100, staticGeo{canMove: true}, src); err != nil {
		t.Fatal(err)
	}
	if got := mover.Queue(); got != first.q {
		t.Fatalf("Queue() = %p, want the source's first queue %p", got, first.q)
	}
	src.q = second.q
	if got := mover.Queue(); got != second.q {
		t.Fatalf("Queue() = %p, want the source's second queue %p", got, second.q)
	}
	if _, err := mover.MoveToLocation(location.Location{X: 300}); err != nil {
		t.Fatal(err)
	}
	first.in.Advance(time.Minute)
	if !mover.Moving() {
		t.Fatal("arrival ran on the queue the source no longer names")
	}
	second.in.Advance(time.Minute)
	if mover.Moving() {
		t.Fatal("walk still under way after its arrival timer on the source's queue")
	}
}
