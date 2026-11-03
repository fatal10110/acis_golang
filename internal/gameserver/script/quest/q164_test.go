package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ164MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids of
// Kirunak's two lines are named constants in the reference.
func TestQ164MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q164", []string{"quest.Q164_BloodFiend"}, "number 16404", "number 16405")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ164PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ164PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q164", "quest/Q164_BloodFiend")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
