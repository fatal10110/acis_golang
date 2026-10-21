package raid

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.individual.Monster.RaidBoss.RaidBossStandard",
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1",
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateStandard",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
