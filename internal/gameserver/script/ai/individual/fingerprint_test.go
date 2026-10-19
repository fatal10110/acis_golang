package individual

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// npcStrings are the NpcString ids the classes say: a named constant in the
// reference, a number in Go.
var npcStrings = []int{10079, 1000458, 1000459, 1000460, 1000461, 1000462, 1000463, 1000503, 1010472, 1010483, 1010484}

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The ai/individual page
// directory is the package's dir, and AltarDoorController's first-talk
// registration is its OnFirstTalk field.
func TestMatchesReferenceFingerprints(t *testing.T) {
	var exempt []string
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
	}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
