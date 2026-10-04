package cast

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// vessel is a boat a test character rides.
type vessel struct{}

func (vessel) ObjectID() int32                 { return 900 }
func (vessel) OustLocation() location.Location { return location.Location{} }

// TestServitorSummonAboardAnswersNotCallPetFromThisLocation pins the boat
// half of the servitor gate, PlayerCast.canCast's
// NOT_CALL_PET_FROM_THIS_LOCATION (PlayerCast.java:285-289): last, after the
// summon-out and mid-swing checks. A cubic summon is never refused by it.
func TestServitorSummonAboardAnswersNotCallPetFromThisLocation(t *testing.T) {
	_, ch := newGatePlayer(t)
	servitor := modelskill.Definition{ID: 1111, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, SkillType: "SUMMON", NpcID: 12600}
	cubic := servitor
	cubic.ID, cubic.IsCubic = 10, true

	ctrl := NewController(PlayerActor{Character: ch}, nil)
	if err := ctrl.CanCast(ch, servitor); err != nil {
		t.Fatalf("ashore: CanCast = %v, want nil", err)
	}
	ch.Board(vessel{})
	if err := ctrl.CanCast(ch, servitor); !errors.Is(err, ErrSummonOnBoat) {
		t.Fatalf("aboard: CanCast = %v, want %v", err, ErrSummonOnBoat)
	}
	if err := ctrl.CanCast(ch, cubic); err != nil {
		t.Fatalf("aboard, cubic: CanCast = %v, want nil", err)
	}
	swinging := NewController(PlayerActor{Character: ch, Attack: swingState(true)}, nil)
	if err := swinging.CanCast(ch, servitor); !errors.Is(err, ErrSummonInCombat) {
		t.Fatalf("aboard mid-swing: CanCast = %v, want %v", err, ErrSummonInCombat)
	}
}
