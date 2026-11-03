package orfen

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes it ports: literals, engine calls and hook shapes. The NpcString
// ids Orfen says are named constants in the reference, numbers in Go.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.boss.orfen.Orfen",
		"script.ai.boss.orfen.OrfenPrivateRibaIren",
	}, "number 1000028", "number 1000029", "number 1000030", "number 1000031")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
