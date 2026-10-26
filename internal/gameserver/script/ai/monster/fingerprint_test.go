package monster

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The panics of an LV3
// wizard private with no master and of a song dancer whose template lists
// no debuff stand for the reference's null dereferences.
func TestMatchesReferenceFingerprints(t *testing.T) {
	exempt := []string{
		`string "monster: an LV3 wizard private has no master to tell"`,
		`string "monster: a song dancer's template lists no debuff skill"`,
	}
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
		"script.ai.individual.Monster.LV3Monster.LV3PartyPrivateMonster",
		"script.ai.individual.Monster.LV3Monster.LV3WizardPrivate",
		"script.ai.individual.Monster.LV3Monster.LV3Ranger",
		"script.ai.individual.Monster.LV3Monster.LV3SongDance",
		"script.ai.individual.Monster.LV3Monster.LV3Summoner",
		"script.ai.individual.Monster.LV3Monster.LV3Wizard",
		"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHold",
		"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHoldAggressive.WarriorDDMagicHoldAggressive",
		"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHoldAggressive.ValakasPrivateLavasaurus",
	}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
