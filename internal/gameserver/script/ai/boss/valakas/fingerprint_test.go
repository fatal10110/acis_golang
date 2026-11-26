package valakas

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The NpcString id Valakas
// shows is a named constant in the reference, a number in Go; the global
// memo key is named from the shared bossmemo table.
// Its even draws (Rnd.nextBoolean) are made by the shared target ranker of
// package bossrank.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.ai.boss.valakas.HeartOfVolcano", "script.ai.boss.valakas.Valakas"}, "call Rnd.nextBoolean", "number 1000519", `string "3"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
