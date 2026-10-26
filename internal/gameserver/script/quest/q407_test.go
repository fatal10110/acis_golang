package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ407MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class ids the quest
// checks are named constants in the reference.
func TestQ407MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q407", []string{"quest.Q407_PathToAnElvenScout"}, "number 18", "number 22")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ407PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ407PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q407", "quest/Q407_PathToAnElvenScout")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
