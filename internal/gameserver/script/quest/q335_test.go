package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ335MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids of the
// monsters' lines are named constants in the reference, and 42 is only the
// initial capacity of one of its maps.
func TestQ335MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q335", []string{"quest.Q335_SongOfTheHunter"},
		"number 33511", "number 33512", "number 33513", "number 42")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ335PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ335PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q335", "quest/Q335_SongOfTheHunter")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
