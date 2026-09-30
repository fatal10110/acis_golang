package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// newRethinkController is a player controller (pawn walks) already walking
// toward a stationary target at x=500, two position updates into the walk.
func newRethinkController(t *testing.T, self *playerFollowSelf) (*Controller, *followTarget) {
	t.Helper()
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}
	target := &followTarget{x: 500}
	if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want the approach", following, err)
	}
	for range 2 {
		controller.PositionUpdate()
	}
	return controller, target
}

// TestControllerPlayerAttackRethinkResendsPawnWalk pins the reference
// player's attack approach (PlayerMove.maybeMoveToPawn): out of reach and
// free to move, every think re-sends the pawn walk from where the player
// stands now, even when the walk under way already heads for the target's
// current position.
func TestControllerPlayerAttackRethinkResendsPawnWalk(t *testing.T) {
	self := &playerFollowSelf{}
	controller, target := newRethinkController(t, self)
	x, y, z := self.Position()
	here := location.Location{X: x, Y: y, Z: z}
	if here == (location.Location{}) {
		t.Fatal("the approach made no progress over two position updates")
	}

	following, err := controller.MaybeStartOffensiveFollow(target, 40)
	if err != nil || !following {
		t.Fatalf("re-think MaybeStartOffensiveFollow() = %v, %v; want still approaching", following, err)
	}
	if got := len(self.moves); got != 2 {
		t.Fatalf("move broadcasts = %d, want 2 (the approach and its re-think)", got)
	}
	ev := self.moves[1]
	if ev.FollowTarget != 2 || ev.FollowOffset != 40 {
		t.Fatalf("re-think move follows %d at %d, want pawn walk toward 2 at 40", ev.FollowTarget, ev.FollowOffset)
	}
	if ev.Origin != here {
		t.Fatalf("re-think move origin = %+v, want the player's current position %+v", ev.Origin, here)
	}
}

// TestControllerRootedPlayerAttackRethinkSendsNoWalk: a player whose
// movement is disabled issues no walk on the think (the reference only
// reports the target out of reach).
func TestControllerRootedPlayerAttackRethinkSendsNoWalk(t *testing.T) {
	self := &playerFollowSelf{}
	controller, target := newRethinkController(t, self)
	self.disabled = true

	following, err := controller.MaybeStartOffensiveFollow(target, 40)
	if err != nil || !following {
		t.Fatalf("re-think MaybeStartOffensiveFollow() = %v, %v; want out of reach", following, err)
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts = %d, want only the first approach", got)
	}
}

// TestControllerNonPlayerAttackRethinkLeavesConvergedWalk keeps a non-player
// actor's walk toward an unmoved target as is: its think sends nothing.
func TestControllerNonPlayerAttackRethinkLeavesConvergedWalk(t *testing.T) {
	self := &summonFollowSelf{}
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}
	target := &followTarget{x: 500}
	for range 2 {
		if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
			t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want the approach", following, err)
		}
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts = %d, want 1: a converged walk is left alone", got)
	}
}
