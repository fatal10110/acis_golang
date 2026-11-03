package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ386MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference reads the lines of the board from a shared
// constant, which the port holds inline, and sizes its chance map; the port
// names the variable a pick finds missing.
func TestQ386MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q386", []string{"quest.Q386_StolenDignity"}, "number 26", "number 5", "number 6", "number 7", "number 8", `string "q386: no variable "`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ386PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ386PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q386", "quest/Q386_StolenDignity")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
