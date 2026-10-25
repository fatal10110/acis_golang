package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ022MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids of the
// NPCs' lines are named constants in the reference.
func TestQ022MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q022", []string{"quest.Q022_TragedyInVonHellmannForest"},
		"number 2250", "number 2251")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ022PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ022PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q022", "quest/Q022_TragedyInVonHellmannForest")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
