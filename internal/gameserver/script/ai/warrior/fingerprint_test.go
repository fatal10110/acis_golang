package warrior

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// npcStrings are the NpcString ids the classes say: a named constant in the
// reference, a number in Go.
var npcStrings = []int{1000286, 1000287, 1000288, 1000290, 1000291, 1000292, 1000294, 1000307, 1000381, 1000382, 1000383, 1000384, 1000385, 1000386, 1000387, 1000388, 1000389, 1000390, 1000391, 1000392, 1000393, 1000394, 1000395, 1000396, 1000397, 1000398, 1000399, 1000400, 1000401, 1000402, 1000403, 1000404, 1000405, 1000406, 1000407, 1000408, 1000409, 1000410, 1000411, 1000412, 1000413, 1000414, 1000427, 1000428}

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The panic of a berserker
// whose template lists no fury skill stands for the reference's null
// dereference.
func TestMatchesReferenceFingerprints(t *testing.T) {
	exempt := []string{`string "warrior: a berserker's template lists no fury skill"`}
	for _, id := range npcStrings {
		exempt = append(exempt, fmt.Sprintf("number %d", id))
	}
	problems, err := scriptfp.Check(".", []string{
		"script.ai.individual.Monster.WarriorBase.WarriorBase",
		"script.ai.individual.Monster.WarriorBase.Warrior.Warrior",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorHold.WarriorHold",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecial",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorFishingBlock",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderWarrior",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateWarrior",
		"script.ai.individual.Monster.WarriorBase.Chests.TreasureChest",
		"script.ai.individual.Monster.WarriorBase.Chests.TreasureChestMimic",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderSplit",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderSplitAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserker",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserkerPhysicalSpecial",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserkerPhysicalSpecialAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserkerPhysicalSpecialStoneAggressive",
	}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
