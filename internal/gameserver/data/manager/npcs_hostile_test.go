package manager

import (
	"testing"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
)

func TestCreatureActorRefSatisfiesTargetCreature(t *testing.T) {
	var _ skilltarget.Creature = (*creatureActorRef)(nil)
}
