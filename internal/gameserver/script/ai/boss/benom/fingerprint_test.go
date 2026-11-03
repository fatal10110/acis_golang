package benom

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes it ports: literals, engine calls and hook shapes. Both name their
// chat lines by id, where the reference names them by constant.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.boss.benom.Benom",
		"script.ai.boss.benom.DungeonTeleporter",
	}, "number 1010623", "number 1010626", "number 1010632")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
