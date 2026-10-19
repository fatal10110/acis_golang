package monster

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.individual.Monster.MonsterAI",
		"script.ai.individual.Monster.WarriorHero",
		"script.ai.individual.Monster.LV3Monster.LV3Monster",
		"script.ai.individual.Monster.LV3Monster.LV3Dagger",
		"script.ai.individual.Monster.LV3Monster.LV3Knight",
		"script.ai.individual.Monster.LV3Monster.LV3Orc",
		"script.ai.individual.Monster.LV3Monster.LV3PartyLeaderMonster",
		"script.ai.individual.Monster.LV3Monster.LV3Dwarf",
		"script.ai.individual.Monster.LV3Monster.LV3Healer",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
