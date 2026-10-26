package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ410MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class ids the quest
// checks are named constants in the reference.
func TestQ410MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q410", []string{"quest.Q410_PathToAPalusKnight"}, "number 31", "number 32")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ410PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ410PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q410", "quest/Q410_PathToAPalusKnight")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
