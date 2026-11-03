package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ217MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference reads the
// class tier through the class itself; ClassLevel reads both.
func TestQ217MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q217", []string{"quest.Q217_TestimonyOfTrust"},
		"call Player.getClassId")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ217PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ217PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q217", "quest/Q217_TestimonyOfTrust")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
