package antharas

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The NpcString id Antharas
// shows is a named constant in the reference, a number in Go.
// Its even draws (Rnd.nextBoolean) are made by the shared target ranker of
// package bossrank.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.ai.boss.antharas.Antharas", "script.ai.boss.antharas.HeartOfWarding"}, "call Rnd.nextBoolean", "number 1000520")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
