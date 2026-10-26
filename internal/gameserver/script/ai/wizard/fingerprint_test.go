package wizard

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// npcStrings are the NpcString ids the classes say: a named constant in the
// reference, a number in Go.
var npcStrings = []int{1000290, 1000291, 1000292, 1000294, 1000395, 1000396, 1000397, 1000398, 1000399, 1000400, 1000401, 1000402, 1000403, 1000404, 1000405, 1000406, 1000407, 1000408, 1000409, 1000410, 1000411, 1000412, 1000413, 1000414, 10075, 10076, 10077, 10078}

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes.
func TestMatchesReferenceFingerprints(t *testing.T) {
	// PartyLeaderWizardSaintAggressive's sight check measures the creature
	// from itself against 40, always under it; the port keeps only the draw.
	// DoormanOfDrain's do-nothing desire keeps the literal in the package.
	// WizardHereticPriestAggressive counts a party through
	// Player.PartyMembers, the Go name it shares with the quest helper
	// Quest.getPartyMembers, which no class here calls.
	// WizardDDMagic2Solina names the occupations it greets by their class id
	// constants; the port writes the ids (5, 30 and 90 are literals of the
	// reference already).
	exempt := []string{"call Quest.getPartyMembers", "number 16", "number 97", "number 105"}
	for _, id := range npcStrings {
		exempt = append(exempt, fmt.Sprintf("number %d", id))
	}
	problems, err := scriptfp.Check(".", []string{
		"script.ai.individual.Monster.WizardBase.WizardBase",
		"script.ai.individual.Monster.WizardBase.Wizard.Wizard",
		"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizard",
		"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.Elcroki",
		"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardDD2",
		"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardCastRangeCurseStoneAggressive",
		"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardCorpseNecroAggressive",
		"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardDD2CurseAggressive",
		"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardHereticAggressive",
		"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardDD2Summon",
		"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardRangeCurseAggressive",
		"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardSaintAggressive",
		"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizard",
		"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardDD2",
		"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardDD2Curse",
		"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardDD2Heal",
		"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardHereticCoward",
		"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardHereticDD2",
		"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardHereticDD2Curse",
		"script.ai.individual.Monster.WizardBase.Wizard.Agit02VampireArcherAgit",
		"script.ai.individual.Monster.WizardBase.Wizard.DoormanOfDrain",
		"script.ai.individual.Monster.WizardBase.Wizard.RoyalRushHealer1",
		"script.ai.individual.Monster.WizardBase.Wizard.RoyalRushHealer2",
		"script.ai.individual.Monster.WizardBase.Wizard.RoyalRushWizardDDMagic2Hold",
		"script.ai.individual.Monster.WizardBase.Wizard.RoyalRushWizardShortSelfRange",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardFiendArcherRunAway",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardHereticPriestAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.RoyalRushWizardClanBuff",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.RoyalRushWizardRangeDebuff",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCastClanBuffCurseStoneAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCastClanBuffRangeCurse",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCastClanBuffRangeDDCurseStoneAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2Curse",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2CurseAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardGrowth.WizardGrowthStep3Aggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardGrowth.WizardGrowthStep4Aggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHereticDDMagic2Curse",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHereticDDMagic2CurseAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseNecro.WizardCorpseNecro",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseNecro.WizardCorpseNecroAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseNecro.WizardCorpseNecroSummon",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseNecro.WizardCorpseNecroSummonAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseVampireBasic.WizardCorpseVampireBasic",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseVampireBasic.WizardCorpseVampireBasicAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseVampireBasic.WizardCorpseVampireCurseAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseVampireBasic.WizardCorpseVampireRange",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseVampireBasic.WizardCorpseVampireRangeAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2Heal",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2HealAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2Solina",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHealer",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHealerAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHereticDDMagic2Aggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHereticDDMagicCurse",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2Aggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2DispellAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHallDDMagic2Aggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseVampireBasic.WizardCorpseVampireTeleport",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardGrowth.WizardGrowthStep2",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardFiendArcher.WizardFiendArcher",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardFiendArcher.WizardFiendArcherAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardFiendArcher.WizardFiendArcherSelfBuff",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardFiendArcher.WizardFiendArcherSelfBuffAggressive",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardSaintBasic.WizardSaintBasic",
		"script.ai.individual.Monster.WizardBase.Wizard.WizardSaintBasic.WizardSaintBasicAggressive",
	}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
