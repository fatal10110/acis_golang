package ketravarka

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the family package with the union
// of its reference classes: literals, engine calls and hook shapes. The 15
// and 21 are the initial sizes of the alliance quests' chance maps, which Go
// has no use for; the ids of the eyes' chat lines are named constants in the
// reference.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"quest.Q605_AllianceWithKetraOrcs",
		"quest.Q606_WarWithVarkaSilenos",
		"quest.Q607_ProveYourCourage",
		"quest.Q608_SlayTheEnemyCommander",
		"quest.Q609_MagicalPowerOfWater_Part1",
		"quest.Q611_AllianceWithVarkaSilenos",
		"quest.Q612_WarWithKetraOrcs",
		"quest.Q613_ProveYourCourage",
		"quest.Q614_SlayTheEnemyCommander",
		"quest.Q615_MagicalPowerOfFire_Part1",
	},
		"number 15", "number 21",
		"number 60903", "number 60904",
		"number 61503", "number 61504")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
