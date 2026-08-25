package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type playerFollowSelf struct {
	x, y, z int
}

func (s *playerFollowSelf) ObjectID() int32                 { return 1 }
func (s *playerFollowSelf) Position() (int, int, int)       { return s.x, s.y, s.z }
func (s *playerFollowSelf) CollisionRadius() float64        { return 0 }
func (s *playerFollowSelf) SetHeading(int)                  {}
func (s *playerFollowSelf) SyncPosition(location.Location)  {}
func (s *playerFollowSelf) BroadcastMove(Event) error       { return nil }
func (s *playerFollowSelf) BroadcastStop() error            { return nil }
func (s *playerFollowSelf) OffensiveFollowIsPawnMove() bool { return true }

type followTarget struct {
	x, y, z int
}

func (t *followTarget) ObjectID() int32           { return 2 }
func (t *followTarget) SiegeGuard() bool          { return false }
func (t *followTarget) AlikeDead() bool           { return false }
func (t *followTarget) Position() (int, int, int) { return t.x, t.y, t.z }
func (t *followTarget) CollisionRadius() float64  { return 0 }

var _ attackable.Combatant = (*followTarget)(nil)

func TestControllerPlayerOffensiveFollowUses3DRange(t *testing.T) {
	self := &playerFollowSelf{}
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewController(mover, self)
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
