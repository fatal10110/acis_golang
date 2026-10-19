package individual

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// npcStrings are the NpcString ids the classes say: a named constant in the
// reference, a number in Go.
var npcStrings = []int{10079, 1000455, 1000456, 1000457, 1000458, 1000459, 1000460, 1000461, 1000462, 1000463, 1000500, 1000501, 1000503, 1010472, 1010483, 1010484}

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The ai/individual page
// directory is the package's dir, and AltarDoorController's first-talk
// registration is its OnFirstTalk field. The quest names WarriorPetForPC
// reads are the reference's quest name constants, and the tamed beast's
// abnormal level read of no owner panics where the reference throws.
func TestMatchesReferenceFingerprints(t *testing.T) {
	exempt := []string{
		`string "Q020_BringUpWithLove"`,
		`string "Q655_AGrandPlanForTamingWildBeasts"`,
		`string "individual: the abnormal level of no creature"`,
	}
	for _, id := range npcStrings {
		exempt = append(exempt, fmt.Sprintf("number %d", id))
	}
	problems, err := scriptfp.Check(".", []string{
		"script.ai.individual.AltarDoorController",
		"script.ai.individual.AreadataSwitch",
		"script.ai.individual.Corpse",
		"script.ai.individual.DaimonTheWhiteEyed",
		"script.ai.individual.ImperialGravekeeper",
		"script.ai.individual.RoyalRushMissionNpc",
		"script.ai.individual.RoyalRushNpc",
		"script.ai.individual.SacrificialVictim",
		"script.ai.individual.TriolCamera",
		"script.ai.individual.WarriorPetForPC",
	}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
