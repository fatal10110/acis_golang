package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ416MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class ids the quest
// checks are named constants in the reference.
func TestQ416MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q416", []string{"quest.Q416_PathToAnOrcShaman"}, "number 49", "number 50")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ416PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ416PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q416", "quest/Q416_PathToAnOrcShaman")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
