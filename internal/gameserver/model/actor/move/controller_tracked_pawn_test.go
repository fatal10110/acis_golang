package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// trackedWalkSelf is a player-shaped walker on the world grid.
type trackedWalkSelf struct {
	world.Presence
	knowingFollowSelf
}

func (s *trackedWalkSelf) ObjectID() int32           { return 1 }
func (s *trackedWalkSelf) Kind() actor.Kind          { return actor.KindPlayer }
func (s *trackedWalkSelf) Position() (int, int, int) { return s.knowingFollowSelf.Position() }
func (s *trackedWalkSelf) SetHeading(h int)          { s.knowingFollowSelf.SetHeading(h) }

// trackedPawn is a world object that is not a combatant, such as a door.
type trackedPawn struct{ world.Presence }

func (*trackedPawn) ObjectID() int32  { return 2 }
func (*trackedPawn) Kind() actor.Kind { return actor.KindDoor }

// A pawn walk toward a world object that is not a combatant (a door) ends
// where the walker stands, as an arrival, once the walker no longer knows
// it on the world grid (the reference checks knows(_pawn) for every pawn in
// PlayerMove.updatePosition); while known, the walk goes on.
func TestControllerPawnWalkEndsOnUnknownTrackedPawn(t *testing.T) {
	state := world.New()
	self := &trackedWalkSelf{}
	state.Spawn(self, 0, 0, 0, 0)
	pawn := &trackedPawn{}
	state.Spawn(pawn, 500, 0, 0, 0)

	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	sink := &eventLog{}
	controller, err := NewController(mover, self, sink)
	if err != nil {
		t.Fatal(err)
	}
	if !controller.MoveToPawn(pawn, 100) {
		t.Fatal("MoveToPawn() = false, want the walk accepted")
	}
	for range 3 {
		controller.PositionUpdate()
	}
	if !mover.Moving() {
		t.Fatal("walk ended while its pawn was still known")
	}
	state.Despawn(pawn)
	controller.PositionUpdate()
	if mover.Moving() {
		t.Fatal("walk still under way after its pawn left the walker's known list")
	}
	if got := mover.Position(); got != (location.Location{X: 30}) {
		t.Fatalf("position = %+v, want the walk left where it stood (X 30)", got)
	}
	if got := sink.arrivals(); got != 1 {
		t.Fatalf("Arrived events = %d, want 1", got)
	}
}
