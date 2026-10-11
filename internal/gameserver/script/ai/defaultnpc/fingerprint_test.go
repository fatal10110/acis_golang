package defaultnpc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The helpers no ported
// behavior calls yet (makeAttackEvent, openCloseDoor, getPledgeCastleState)
// land with their first caller (#3597); their calls, and the number 2 only
// getPledgeCastleState returns, are exempt. The private creation loop is
// Spawner.CreatePrivates, and the "-" split of a skill parameter is inside
// NPC.SkillParam.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.ai.individual.DefaultNpc"},
		"call Castle.getSiege", "call Clan.getCastleId", "call Creature.isDead", "call Door.closeMe", "call Door.openMe",
		"call DoorData.getDoor", "call DoorData.getInstance", "call Npc.getCastle", "call Npc.getMaster", "call Npc.getMinions",
		"call Npc.hasMaster", "call Npc.isMaster", "call NpcTemplate.getEventQuests", "call Quest.createOnePrivate",
		"call Quest.onAttacked", "call Quest.onPartyAttacked", "call Residence.getId", "call Siege.checkSides",
		"call Siege.isInProgress", "number 2", `string "-"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
