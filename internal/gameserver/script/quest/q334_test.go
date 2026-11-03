package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ334MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids of the
// NPCs' lines are named constants in the reference.
func TestQ334MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q334", []string{"quest.Q334_TheWishingPotion"},
		"number 33409", "number 33410", "number 33411", "number 33412", "number 33413", "number 33414", "number 33415", "number 33416", "number 33417", "number 33418", "number 33420", "number 33421", "number 33422", "number 33423", "number 33424", "number 33425", "number 33426")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ334PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ334PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q334", "quest/Q334_TheWishingPotion")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
