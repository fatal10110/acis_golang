package summon

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
)

// blockedSightStub refuses every plain sight query and, when the ignoring
// query leaves an object out, reports sight to it.
type blockedSightStub struct {
	ignored []dynamic.Object
}

func (*blockedSightStub) CanSeeActor(_, _, _ int, _ float64, _, _, _ int, _ float64) bool {
	return false
}

func (s *blockedSightStub) CanSeeActorIgnoring(_, _, _ int, _ float64, _, _, _ int, _ float64, ignore dynamic.Object) bool {
	s.ignored = append(s.ignored, ignore)
	return ignore != nil
}

// sightTarget is a cast target standing at a fixed point. Only the methods
// CanSeeTarget reads are implemented.
type sightTarget struct {
	skilltarget.Actor
}

func (sightTarget) Position() (int, int, int) { return 100, 0, 0 }
func (sightTarget) CollisionHeight() float64  { return 20 }

// sightDoor is a target that is itself a geodata object, like a closed door.
type sightDoor struct {
	sightTarget
}

func (sightDoor) GeoX() int               { return 0 }
func (sightDoor) GeoY() int               { return 0 }
func (sightDoor) GeoZ() int               { return 0 }
func (sightDoor) Height() int             { return 0 }
func (sightDoor) GeoData() [][]block.NSWE { return nil }

func TestCanSeeTargetLeavesTargetDoorOutOfSightQuery(t *testing.T) {
	los := &blockedSightStub{}
	a := mustServitor(t, ServitorConfig{ObjectID: 1, LOS: los})

	door := sightDoor{}
	if !a.CanSeeTarget(door) {
		t.Fatal("CanSeeTarget(door) = false, want the door left out of its own sight query")
	}
	if len(los.ignored) != 1 || los.ignored[0] != dynamic.Object(door) {
		t.Fatalf("ignored objects = %v, want exactly the target door", los.ignored)
	}

	if a.CanSeeTarget(sightTarget{}) {
		t.Fatal("CanSeeTarget(non-door) = true, want the plain blocked sight query")
	}
	if len(los.ignored) != 1 {
		t.Fatalf("non-door target used the ignoring query (%d calls), want CanSeeActor", len(los.ignored))
	}
}
