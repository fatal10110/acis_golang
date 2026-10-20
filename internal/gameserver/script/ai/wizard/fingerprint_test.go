package wizard

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// npcStrings are the NpcString ids the classes say: a named constant in the
// reference, a number in Go.
var npcStrings = []int{1000290, 1000291, 1000292, 1000294, 1000395, 1000396, 1000397, 1000398, 1000399, 1000400, 1000401, 1000402, 1000403, 1000404, 1000405, 1000406, 1000407, 1000408, 1000409, 1000410, 1000411, 1000412, 1000413, 1000414}

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes.
func TestMatchesReferenceFingerprints(t *testing.T) {
	// PartyLeaderWizardSaintAggressive's sight check measures the creature
	// from itself against 40, always under it; the port keeps only the draw.
	exempt := []string{"number 40"}
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
	}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
