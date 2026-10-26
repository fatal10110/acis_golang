package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ409MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The two class ids of the
// elven mystic and oracle and the NpcString ids of the monsters' lines are
// named constants in the reference.
func TestQ409MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q409", []string{"quest.Q409_PathToAnElvenOracle"},
		"number 25", "number 29", "number 40909", "number 40910", "number 40911", "number 40912", "number 40913")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ409PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ409PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q409", "quest/Q409_PathToAnElvenOracle")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
