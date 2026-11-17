package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestScriptInstantTeleportRefreshesTheSummon (Creature.instantTeleportTo,
// WorldObject.refreshKnownlist): a summon a script jumps within its region
// is shown afresh to its owner around it: after the fast TeleportToLocation
// the owner gets its PetInfo.
func TestScriptInstantTeleportRefreshesTheSummon(t *testing.T) {
	t.Parallel()
	w := bootJumpers(t)
	wolf, _ := w.spawnWolf(t)
	drainUntilQuiet(t, w.client)

	frames := w.hitFrom(t, w.puller, wolf)
	jumped, shown := -1, -1
	for i, f := range frames {
		switch {
		case jumped < 0 && f[0] == serverpackets.OpcodeTeleportToLocation:
			jumped = i
		case jumped >= 0 && shown < 0 && f[0] == serverpackets.OpcodePetInfo:
			shown = i
		}
	}
	if jumped < 0 || shown < 0 {
		t.Fatalf("TeleportToLocation at %d, PetInfo after it at %d; want the summon's jump, then its PetInfo", jumped, shown)
	}
}
