package baium

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The NpcString ids Baium
// says and shows are named constants in the reference, numbers in Go; the
// stone's abort on an empty memo key is the reference's thrown exception.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.boss.baium.Archangel",
		"script.ai.boss.baium.Baium",
		"script.ai.boss.baium.BaiumAngelicVortex",
		"script.ai.boss.baium.BaiumStone",
	}, "number 22937", "number 1000521",
		`string "baium: global memo %q holds no value"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
