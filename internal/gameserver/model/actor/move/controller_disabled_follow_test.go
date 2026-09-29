package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// secondFollowTarget is a follow target with a different object id from
// followTarget.
type secondFollowTarget struct{ followTarget }

func (*secondFollowTarget) ObjectID() int32 { return 3 }

func newDisabledFollowController(t *testing.T) (*Controller, *CreatureMove, *playerFollowSelf) {
	t.Helper()
	self := &playerFollowSelf{}
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

func requireFollow(t *testing.T, mover *CreatureMove, mode FollowMode, target int32, what string) {
	t.Helper()
	if got := mover.FollowMode(); got != mode {
		t.Fatalf("%s: follow mode = %v, want %v", what, got, mode)
	}
	if got := mover.FollowTarget(); got != target {
		t.Fatalf("%s: follow target = %d, want %d", what, got, target)
	}
}

// An offensive follow already running toward the target keeps re-issuing its
// move after movement is disabled: the effect that locks movement stops a
// running follow itself, and nothing else does.
func TestControllerOffensiveFollowRunningTowardTargetContinuesWhileDisabled(t *testing.T) {
	controller, mover, self := newDisabledFollowController(t)
	target := &followTarget{x: 100}
	if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want active follow", following, err)
	}

	self.disabled = true
	target.x = 200
	following, err := controller.MaybeStartOffensiveFollow(target, 40)
	if err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() while disabled = %v, %v; want the running follow to continue", following, err)
	}
	if got := len(self.moves); got != 2 {
		t.Fatalf("move broadcasts = %d, want 2: the running follow re-issues its move toward the moved target", got)
	}
	if got := self.moves[1].Destination; got != (location.Location{X: 200}) {
		t.Fatalf("re-issued move destination = %+v, want the target's new position", got)
	}
	requireFollow(t, mover, FollowOffensive, target.ObjectID(), "running offensive follow")
}

// A disabled actor asked to follow a different target waits (reports true)
// but starts no follow and sends no move.
func TestControllerOffensiveFollowNewTargetWhileDisabledStartsNothing(t *testing.T) {
	controller, mover, self := newDisabledFollowController(t)
	first := &followTarget{x: 100}
	if following, err := controller.MaybeStartOffensiveFollow(first, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want active follow", following, err)
	}

	self.disabled = true
	other := &secondFollowTarget{followTarget{x: 300}}
	following, err := controller.MaybeStartOffensiveFollow(other, 40)
	if err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow(other) while disabled = %v, %v; want true so the caller waits", following, err)
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts = %d, want only the first follow's move", got)
	}
	requireFollow(t, mover, FollowOffensive, first.ObjectID(), "follow after a disabled request toward another target")

	fresh, freshMover, freshSelf := newDisabledFollowController(t)
	freshSelf.disabled = true
	following, err = fresh.MaybeStartOffensiveFollow(other, 40)
	if err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() on a disabled idle actor = %v, %v; want true so the caller waits", following, err)
	}
	if got := len(freshSelf.moves); got != 0 {
		t.Fatalf("move broadcasts from a disabled idle actor = %d, want 0", got)
	}
	requireFollow(t, freshMover, FollowNone, 0, "disabled idle actor")
}

// A friendly follow already running toward the target keeps re-issuing its
// move after movement is disabled.
func TestControllerFriendlyFollowRunningTowardTargetContinuesWhileDisabled(t *testing.T) {
	controller, mover, self := newDisabledFollowController(t)
	target := &followTarget{x: 200}
	if following, err := controller.MaybeStartFriendlyFollow(target, 40); err != nil || !following {
		t.Fatalf("MaybeStartFriendlyFollow() = %v, %v; want active follow", following, err)
	}

	self.disabled = true
	target.x = 300
	following, err := controller.MaybeStartFriendlyFollow(target, 40)
	if err != nil || !following {
		t.Fatalf("MaybeStartFriendlyFollow() while disabled = %v, %v; want the running follow to continue", following, err)
	}
	if got := len(self.moves); got != 2 {
		t.Fatalf("move broadcasts = %d, want 2: the running follow re-issues its move toward the moved target", got)
	}
	if got := self.moves[1].Destination; got != (location.Location{X: 300}) {
		t.Fatalf("re-issued move destination = %+v, want the target's new position", got)
	}
	requireFollow(t, mover, FollowFriendly, target.ObjectID(), "running friendly follow")
}

// A disabled actor asked to friendly-follow a different target reports false
// and leaves the follow state and movement as they were.
func TestControllerFriendlyFollowNewTargetWhileDisabledStartsNothing(t *testing.T) {
	controller, mover, self := newDisabledFollowController(t)
	first := &followTarget{x: 200}
	if following, err := controller.MaybeStartFriendlyFollow(first, 40); err != nil || !following {
		t.Fatalf("MaybeStartFriendlyFollow() = %v, %v; want active follow", following, err)
	}

	self.disabled = true
	other := &secondFollowTarget{followTarget{x: 300}}
	following, err := controller.MaybeStartFriendlyFollow(other, 40)
	if err != nil || following {
		t.Fatalf("MaybeStartFriendlyFollow(other) while disabled = %v, %v; want false", following, err)
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts = %d, want only the first follow's move", got)
	}
	requireFollow(t, mover, FollowFriendly, first.ObjectID(), "follow after a disabled request toward another target")
}
