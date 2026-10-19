package siegeguard

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The reference names
// Seal of Ruler by its frequent-skill entry; the port writes its id.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.siegeguards.GludioCleric",
		"script.ai.siegeguards.GludioClericStand",
		"script.ai.siegeguards.GludioWizardStand",
		"script.ai.siegeguards.GludioHold.GludioHold",
		"script.ai.siegeguards.GludioHold.GludioBowGuard",
		"script.ai.siegeguards.GludioHold.GludioCourtGuard",
		"script.ai.siegeguards.GludioHold.GludioDukeLewinWaldner",
		"script.ai.siegeguards.GludioHold.GludioKnight",
	}, "number 246")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
