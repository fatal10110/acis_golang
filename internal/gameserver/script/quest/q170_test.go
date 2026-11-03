package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ170MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids of
// Merkenis's two lines are named constants in the reference.
func TestQ170MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q170", []string{"quest.Q170_DangerousSeduction"}, "number 17004", "number 17005")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ170PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ170PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q170", "quest/Q170_DangerousSeduction")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
