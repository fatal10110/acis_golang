package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ406MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class ids the quest
// checks are named constants in the reference.
func TestQ406MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q406", []string{"quest.Q406_PathToAnElvenKnight"}, "number 18")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ406PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ406PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q406", "quest/Q406_PathToAnElvenKnight")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
