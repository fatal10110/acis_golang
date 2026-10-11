package wizard

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// npcStrings are the NpcString ids the classes say: a named constant in the
// reference, a number in Go.
var npcStrings = []int{1000290, 1000395, 1000396, 1000397, 1000406, 1000407, 1000408, 1000409, 1000410, 1000411, 1000412, 1000413, 1000414}

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes.
func TestMatchesReferenceFingerprints(t *testing.T) {
	var exempt []string
	for _, id := range npcStrings {
		exempt = append(exempt, fmt.Sprintf("number %d", id))
	}
	problems, err := scriptfp.Check(".", []string{"script.ai.individual.Monster.WizardBase.WizardBase", "script.ai.individual.Monster.WizardBase.Wizard.Wizard"}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
