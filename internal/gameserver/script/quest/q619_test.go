package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ619MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ619MatchesReferenceFingerprints(t *testing.T) {
	problems, err := // 79 and 45 are the reference's initial sizes of its two drop maps, which
		// Go switches have no use for.
		scriptfp.Check("q619", []string{"quest.Q619_RelicsOfTheOldEmpire"}, "number 79", "number 45")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ619PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ619PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q619", "quest/Q619_RelicsOfTheOldEmpire")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
