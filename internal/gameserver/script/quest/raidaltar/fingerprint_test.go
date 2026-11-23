package raidaltar

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the family package with the union
// of its reference classes: literals, engine calls and hook shapes. The ids of
// the raids' chat lines are named constants in the reference.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"quest.Q604_DaimonTheWhiteEyed_Part2",
		"quest.Q610_MagicalPowerOfWater_Part2",
		"quest.Q616_MagicalPowerOfFire_Part2",
		"quest.Q625_TheFinestIngredients_Part2",
	},
		"number 60403", "number 60404",
		"number 61050", "number 61051",
		"number 61650", "number 61651",
		"number 62503", "number 62504")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
