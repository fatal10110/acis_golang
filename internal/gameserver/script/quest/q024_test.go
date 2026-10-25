package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ024MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids of the
// NPCs' lines are named constants in the reference.
func TestQ024MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q024", []string{"quest.Q024_InhabitantsOfTheForestOfTheDead"},
		"number 2450")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ024PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ024PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q024", "quest/Q024_InhabitantsOfTheForestOfTheDead")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
