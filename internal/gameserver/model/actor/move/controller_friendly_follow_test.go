package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// knowingFollowSelf is a player-shaped follower whose known list the test
// controls.
type knowingFollowSelf struct {
	playerFollowSelf
	forgot  bool
	stopped int
}

func (s *knowingFollowSelf) Knows(attackable.Combatant) bool { return !s.forgot }
func (s *knowingFollowSelf) BroadcastStop()                  { s.stopped++ }

func newPlayerFriendlyFollowController(t *testing.T) (*Controller, *CreatureMove, *knowingFollowSelf) {
	t.Helper()
	self := &knowingFollowSelf{}
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}
	return controller, mover, self
}

// A player's friendly follow walks at once toward a target farther than the
// offset, with the target-relative movement packet at that offset.
func TestControllerPlayerFriendlyFollowStartsPawnMove(t *testing.T) {
	controller, mover, self := newPlayerFriendlyFollowController(t)
	target := &followTarget{x: 200}

	if following, err := controller.MaybeStartFriendlyFollow(target, 70); err != nil || !following {
		t.Fatalf("MaybeStartFriendlyFollow() = %v, %v; want the follow armed", following, err)
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts = %d, want 1", got)
	}
	if ev := self.moves[0]; ev.FollowTarget != target.ObjectID() || ev.FollowOffset != 70 || ev.Destination != (location.Location{X: 200}) {
		t.Fatalf("move = %+v, want a pawn move toward the target at offset 70", ev)
	}
	requireFollow(t, mover, FollowFriendly, target.ObjectID(), "player friendly follow")
}

// Within the offset (footprints not counted) the follow is armed but no walk
// starts; once the target moves away, the next once-a-second tick walks.
func TestControllerPlayerFriendlyFollowInRangeWaitsForTheTick(t *testing.T) {
	controller, mover, self := newPlayerFriendlyFollowController(t)
	target := &followTarget{x: 69}

	if following, err := controller.MaybeStartFriendlyFollow(target, 70); err != nil || !following {
		t.Fatalf("MaybeStartFriendlyFollow() = %v, %v; want the follow armed", following, err)
	}
	if got := len(self.moves); got != 0 {
		t.Fatalf("move broadcasts in range = %d, want 0", got)
	}
	requireFollow(t, mover, FollowFriendly, target.ObjectID(), "armed follow in range")

	target.x = 300
	for range 9 {
		controller.PositionUpdate()
	}
	if got := len(self.moves); got != 0 {
		t.Fatalf("move broadcasts before the 1 s tick = %d, want 0", got)
	}
	controller.PositionUpdate()
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts after the 1 s tick = %d, want 1", got)
	}
	if got := self.moves[0].Destination; got != (location.Location{X: 300}) {
		t.Fatalf("tick move destination = %+v, want the target's new position", got)
	}
}

// A tick that finds the player still walking re-issues nothing; the first
// tick after the walk ends walks toward where the target is now.
func TestControllerPlayerFriendlyFollowTickWaitsForTheWalk(t *testing.T) {
	controller, _, self := newPlayerFriendlyFollowController(t)
	target := &followTarget{x: 200}
	if _, err := controller.MaybeStartFriendlyFollow(target, 70); err != nil {
		t.Fatal(err)
	}
	target.x = 400

	// 200 units at speed 100 take 2 s: the 1 s tick finds it walking.
	for range 15 {
		controller.PositionUpdate()
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts while walking = %d, want 1", got)
	}
	for range 5 {
		controller.PositionUpdate()
	}
	if got := len(self.moves); got != 2 {
		t.Fatalf("move broadcasts after the walk = %d, want 2", got)
	}
	if got := self.moves[1].Destination; got != (location.Location{X: 400}) {
		t.Fatalf("re-issued destination = %+v, want the target's new position", got)
	}
}

// A target the player no longer knows ends the follow and stops the player.
func TestControllerPlayerFriendlyFollowForgottenTargetStops(t *testing.T) {
	controller, mover, self := newPlayerFriendlyFollowController(t)
	target := &followTarget{x: 200}
	if _, err := controller.MaybeStartFriendlyFollow(target, 70); err != nil {
		t.Fatal(err)
	}

	self.forgot = true
	for range 10 {
		controller.PositionUpdate()
	}
	requireFollow(t, mover, FollowNone, 0, "follow of a forgotten target")
	if mover.Moving() {
		t.Fatal("player still walking after its follow target was forgotten")
	}
	if self.stopped != 1 {
		t.Fatalf("stop broadcasts = %d, want 1", self.stopped)
	}
}

// CancelFriendlyFollow ends the follow task and leaves the walk running;
// it leaves an offensive follow alone.
func TestControllerCancelFriendlyFollowKeepsTheWalk(t *testing.T) {
	controller, mover, self := newPlayerFriendlyFollowController(t)
	target := &followTarget{x: 200}
	if _, err := controller.MaybeStartFriendlyFollow(target, 70); err != nil {
		t.Fatal(err)
	}
	controller.CancelFriendlyFollow()
	requireFollow(t, mover, FollowNone, 0, "cancelled friendly follow")
	if !mover.Moving() {
		t.Fatal("CancelFriendlyFollow stopped the walk under way")
	}
	target.x = 600
	for range 40 {
		controller.PositionUpdate()
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts after the cancel = %d, want 1", got)
	}

	if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want active follow", following, err)
	}
	controller.CancelFriendlyFollow()
	requireFollow(t, mover, FollowOffensive, target.ObjectID(), "offensive follow after a friendly cancel")
}

// A pawn move walks toward the target's current position and broadcasts the
// target-relative movement packet at the given offset, without arming a
// follow task that would re-aim or re-issue it.
func TestControllerMoveToPawnBroadcastsPawnMoveWithoutFollow(t *testing.T) {
	controller, mover, self := newPlayerFriendlyFollowController(t)
	target := &followTarget{x: 200}

	if !controller.MoveToPawn(target, 100) {
		t.Fatal("MoveToPawn() = false, want the walk accepted")
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts = %d, want 1", got)
	}
	if ev := self.moves[0]; ev.FollowTarget != target.ObjectID() || ev.FollowOffset != 100 || ev.Destination != (location.Location{X: 200}) {
		t.Fatalf("move = %+v, want a pawn move toward the target at offset 100", ev)
	}
	if !mover.Moving() {
		t.Fatal("Moving() = false after MoveToPawn, want the walk under way")
	}
	if mover.Following() {
		t.Fatalf("follow mode = %v after MoveToPawn, want none", mover.FollowMode())
	}
}
