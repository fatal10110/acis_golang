package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ221MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference reads the
// class tier through the class itself; ClassLevel reads both.
func TestQ221MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q221", []string{"quest.Q221_TestimonyOfProsperity"}, "call Player.getClassId")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ221PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ221PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q221", "quest/Q221_TestimonyOfProsperity")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
