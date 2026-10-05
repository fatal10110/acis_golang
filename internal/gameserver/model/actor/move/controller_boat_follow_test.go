package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// passengerTarget is a follow target that may ride a boat.
type passengerTarget struct {
	followTarget
	inBoat bool
}

func (p *passengerTarget) InBoat() bool { return p.inBoat }

// npcKindFollowSelf is an NPC follower: a creature that is not playable and
// walks after its follow target with plain movement requests.
type npcKindFollowSelf struct {
	summonFollowSelf
	attackabletest.Combatant
}

func (s *npcKindFollowSelf) ObjectID() int32           { return s.summonFollowSelf.ObjectID() }
func (s *npcKindFollowSelf) Position() (int, int, int) { return s.summonFollowSelf.Position() }
func (s *npcKindFollowSelf) CollisionRadius() float64  { return 0 }
func (s *npcKindFollowSelf) MovementDisabled() bool    { return s.disabled }
func (*npcKindFollowSelf) Heading() int                { return 0 }

var _ attackable.Combatant = (*summonKindFollowSelf)(nil)

// summonKindFollowSelf is npcKindFollowSelf as a summon.
type summonKindFollowSelf struct{ npcKindFollowSelf }

func (*summonKindFollowSelf) Kind() actor.Kind { return actor.KindSummon }

// No NPC walks after a passenger (CreatureMove.friendlyFollowTask returns on
// target.isInBoat()): the follow stays armed with no move, and walks once
// the target is ashore. A summon (SummonMove.friendlyFollowTask) and a
// player (PlayerMove.friendlyFollowTask) have no such check.
func TestFriendlyFollowOfPassenger(t *testing.T) {
	cases := []struct {
		name  string
		self  followSelfActor
		walks bool
	}{
		{name: "npc", self: &npcKindFollowSelf{}, walks: false},
		{name: "summon", self: &summonKindFollowSelf{}, walks: true},
		{name: "player", self: &playerFollowSelf{}, walks: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			controller, mover, self := newDisabledFollowControllerFor(t, tc.self)
			target := &passengerTarget{followTarget: followTarget{x: 300}, inBoat: true}
			if _, err := controller.MaybeStartFriendlyFollow(target, 70); err != nil {
				t.Fatal(err)
			}
			if got := len(self.moves) == 1; got != tc.walks {
				t.Fatalf("moves toward a passenger = %d, want walks %v", len(self.moves), tc.walks)
			}
			requireFollow(t, mover, FollowFriendly, target.ObjectID(), "follow of a passenger")
			if tc.walks {
				return
			}
			target.inBoat = false
			if _, err := controller.MaybeStartFriendlyFollow(target, 70); err != nil {
				t.Fatal(err)
			}
			if len(self.moves) != 1 || self.moves[0].Destination != (location.Location{X: 300}) {
				t.Fatalf("moves once ashore = %+v, want one walk to the target", self.moves)
			}
		})
	}
}

// A summon's follow of anyone but its owner (SummonMove.friendlyFollowTask)
// is armed like any friendly follow but never walks after the target: out of
// reach (offset plus both footprints, strictly, on the ground plane) the target's
// position comes back for the boat-entrance probe, in reach nothing does.
func TestEntranceFollowArmsWithoutWalking(t *testing.T) {
	controller, mover, self := newDisabledFollowControllerFor(t, &summonKindFollowSelf{})
	target := &followTarget{x: 300, z: 40}
	dest, out := controller.MaybeStartEntranceFollow(target, 70)
	if !out || dest != (location.Location{X: 300, Z: 40}) {
		t.Fatalf("MaybeStartEntranceFollow() = %+v, %v; want the target's position, out of reach", dest, out)
	}
	if len(self.moves) != 0 {
		t.Fatalf("moves = %+v, want none", self.moves)
	}
	requireFollow(t, mover, FollowFriendly, target.ObjectID(), "entrance follow")

	target.x = 69
	if _, out := controller.MaybeStartEntranceFollow(target, 70); out {
		t.Fatal("target at the offset reported out of reach")
	}
	if len(self.moves) != 0 {
		t.Fatalf("moves in reach = %+v, want none", self.moves)
	}
}

// A summon that cannot move arms no entrance follow.
func TestEntranceFollowWhileDisabled(t *testing.T) {
	controller, mover, self := newDisabledFollowControllerFor(t, &summonKindFollowSelf{})
	self.disabled = true
	if _, out := controller.MaybeStartEntranceFollow(&followTarget{x: 300}, 70); out {
		t.Fatal("disabled summon reported a target out of reach")
	}
	if mover.FollowMode() != FollowNone {
		t.Fatalf("follow mode = %v, want none", mover.FollowMode())
	}
}
