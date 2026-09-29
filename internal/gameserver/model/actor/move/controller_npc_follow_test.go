package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// npcSightSelf is an NPC follow actor whose current intention and line of
// sight are set by the test.
type npcSightSelf struct {
	playerFollowSelf
	movesToTarget bool
	sees          bool
}

func (*npcSightSelf) OffensiveFollowIsPawnMove() bool    { return false }
func (s *npcSightSelf) IntentionMovesToTarget() bool     { return s.movesToTarget }
func (s *npcSightSelf) CanSee(attackable.Combatant) bool { return s.sees }

// wideFollowTarget is a follow target with a real footprint.
type wideFollowTarget struct{ followTarget }

func (*wideFollowTarget) CollisionRadius() float64 { return 30 }

// An NPC free to move whose target sits inside its reach resolves the
// in-range branch by line of sight: seen, it reports false and follows
// nothing; unseen but already inside the target's footprint radius, it arms
// the offensive follow toward the target at that footprint and reports true
// without broadcasting a move; unseen and farther than the footprint, it
// walks. A hold intention in reach never checks sight and reports false.
func TestControllerNPCOffensiveFollowInReachBySight(t *testing.T) {
	const attackRange = 100 // reach = 100 + 0 + 30 = 130; footprint = 30 + 0 + 30 = 60.
	tests := []struct {
		name          string
		movesToTarget bool
		sees          bool
		targetX       int
		following     bool
		armed         bool
		moves         int
	}{
		{name: "seen in reach swings", movesToTarget: true, sees: true, targetX: 40},
		{name: "unseen pressed against target", movesToTarget: true, targetX: 40, following: true, armed: true},
		{name: "unseen inside reach walks", movesToTarget: true, targetX: 100, following: true, armed: true, moves: 1},
		{name: "hold unseen in reach swings", targetX: 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			self := &npcSightSelf{movesToTarget: tt.movesToTarget, sees: tt.sees}
			mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
			if err != nil {
				t.Fatal(err)
			}
			mover.SetQueue(newMoveClock().q)
			controller, err := NewController(mover, self, nil)
			if err != nil {
				t.Fatal(err)
			}
			target := &wideFollowTarget{followTarget{x: tt.targetX}}

			following, err := controller.MaybeStartOffensiveFollow(target, attackRange)
			if err != nil || following != tt.following {
				t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want %v, nil", following, err, tt.following)
			}
			if got := len(self.moves); got != tt.moves {
				t.Fatalf("move broadcasts = %d, want %d", got, tt.moves)
			}
			if got := mover.Moving(); got != (tt.moves > 0) {
				t.Fatalf("Moving() = %v, want %v", got, tt.moves > 0)
			}
			if !tt.armed {
				requireFollow(t, mover, FollowNone, 0, tt.name)
				return
			}
			requireFollow(t, mover, FollowOffensive, target.ObjectID(), tt.name)
			if controller.offensiveTarget != attackable.Combatant(target) || controller.offensiveRange != 30 {
				t.Fatalf("offensive follow = %v at %d, want the target at its footprint 30", controller.offensiveTarget, controller.offensiveRange)
			}
		})
	}
}
