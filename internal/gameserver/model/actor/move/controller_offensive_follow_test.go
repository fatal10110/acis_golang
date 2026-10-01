package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type npcFollowSelf struct{ playerFollowSelf }

func (*npcFollowSelf) OffensiveFollowLead() bool { return true }

func TestControllerPlayerOffensiveFollowUses3DRange(t *testing.T) {
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

	following, err := controller.MaybeStartOffensiveFollow(&followTarget{z: 41}, 40)
	if err != nil {
		t.Fatal(err)
	}
	if !following {
		t.Fatal("MaybeStartOffensiveFollow() = false, want true when vertical distance exceeds attack range")
	}
}

// TestControllerPlayerOffensiveFollowRangeIsStrict pins the boundary of the
// player attack approach: a target exactly at the attack range is out of
// reach and starts an approach, one unit closer does not.
func TestControllerPlayerOffensiveFollowRangeIsStrict(t *testing.T) {
	for _, tc := range []struct {
		z    int
		want bool
	}{
		{40, true},
		{39, false},
	} {
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

		following, err := controller.MaybeStartOffensiveFollow(&followTarget{z: tc.z}, 40)
		if err != nil {
			t.Fatal(err)
		}
		if following != tc.want {
			t.Fatalf("target %d units away with range 40: MaybeStartOffensiveFollow() = %v, want %v", tc.z, following, tc.want)
		}
	}
}

func TestControllerNPCOffensiveFollowAddsLeadOnlyForMovingTargets(t *testing.T) {
	tests := []struct {
		name      string
		self      Actor
		target    *followTarget
		following bool
	}{
		{
			name:      "npc moving target is inside lead range",
			self:      &npcFollowSelf{},
			target:    &followTarget{x: 80, moving: true},
			following: false,
		},
		{
			name:      "npc stationary target keeps normal range",
			self:      &npcFollowSelf{},
			target:    &followTarget{x: 80},
			following: true,
		},
		{
			name:      "player moving target keeps normal range",
			self:      &playerFollowSelf{},
			target:    &followTarget{x: 80, moving: true},
			following: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
			if err != nil {
				t.Fatal(err)
			}
			mover.SetQueue(newMoveClock().q)
			controller, err := NewController(mover, tt.self, nil)
			if err != nil {
				t.Fatal(err)
			}

			following, err := controller.MaybeStartOffensiveFollow(tt.target, 40)
			if err != nil {
				t.Fatal(err)
			}
			if following != tt.following {
				t.Fatalf("MaybeStartOffensiveFollow() = %v, want %v", following, tt.following)
			}
		})
	}
}

// npcChaseSelf is a hostile NPC: it approaches its target with a plain walk
// the controller's follow recheck re-sends.
type npcChaseSelf struct{ playerFollowSelf }

func (*npcChaseSelf) OffensiveFollowIsPawnMove() bool { return false }

func TestControllerOffensiveFollowRechecksMovingTargetEveryFivePositionUpdates(t *testing.T) {
	self := &npcChaseSelf{}
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}
	target := &followTarget{x: 100}
	if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want active follow", following, err)
	}

	target.x = 200
	for range 5 {
		controller.PositionUpdate()
	}

	if got := len(self.moves); got != 2 {
		t.Fatalf("move broadcasts = %d, want 2 after the 500 ms follow recheck", got)
	}
	if got := self.moves[1].Destination; got != (location.Location{X: 200}) {
		t.Fatalf("follow recheck destination = %+v, want target's latest position", got)
	}
}

// A player's attack approach runs no follow recheck (PlayerAI.thinkAttack
// walks by PlayerMove.maybeMoveToPawn, PlayerAI.java:188; only
// CreatureMove.maybeStartOffensiveFollow, which PlayerAI never calls, starts
// a follow task): its pawn walk re-aims at the moving target itself and no
// MoveToPawn is re-sent until the attack thinks again.
func TestControllerPlayerAttackApproachRunsNoRecheck(t *testing.T) {
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
	target := &followTarget{x: 300}
	if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want the approach", following, err)
	}

	target.x = 400
	for range 10 {
		controller.PositionUpdate()
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts = %d, want only the approach", got)
	}
	if got := mover.Destination(); got != (location.Location{X: 400}) {
		t.Fatalf("walk destination = %+v, want re-aimed at the target's latest position", got)
	}
}

func TestControllerStopCancelsOffensiveFollowRechecks(t *testing.T) {
	self := &npcChaseSelf{}
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}
	target := &followTarget{x: 100}
	if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want active follow", following, err)
	}
	controller.Stop()

	target.x = 200
	for range 5 {
		controller.PositionUpdate()
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts after Stop() = %d, want 1", got)
	}
}

func TestControllerDefersToActorOwnedOffensiveFollowTicker(t *testing.T) {
	self := &tickerOwnedFollowSelf{}
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}
	target := &followTarget{x: 100}
	if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want active follow", following, err)
	}

	target.x = 200
	for range 5 {
		controller.PositionUpdate()
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("controller move broadcasts = %d, want only the initial move when actor owns the follow ticker", got)
	}
}
