package antharas

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The NpcString id Antharas
// shows is a named constant in the reference, a number in Go; the Heart's
// abort on a memo key naming no NPC is the reference's thrown exception.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.ai.boss.antharas.Antharas", "script.ai.boss.antharas.HeartOfWarding"}, "number 1000520",
		`string "antharas: global memo %q names no NPC"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
