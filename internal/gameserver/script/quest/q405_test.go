package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ405MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class ids the quest
// checks are named constants in the reference.
func TestQ405MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q405", []string{"quest.Q405_PathToACleric"}, "number 10", "number 15")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ405PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ405PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q405", "quest/Q405_PathToACleric")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
