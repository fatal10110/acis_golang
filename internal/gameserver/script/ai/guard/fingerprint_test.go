package guard

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.individual.Guard.Guard",
		"script.ai.individual.Guard.GuardStand",
		"script.ai.individual.Guard.GuardMoveAround",
		"script.ai.individual.Guard.GuardFixed",
		"script.ai.individual.Guard.GuardMoveAroundFixed",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
