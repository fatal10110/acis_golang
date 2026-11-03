package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ162MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference sizes its
// drop-chance map with a capacity hint of 6, which is not a game value.
func TestQ162MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q162", []string{"quest.Q162_CurseOfTheUndergroundFortress"}, "number 6")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ162PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ162PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q162", "quest/Q162_CurseOfTheUndergroundFortress")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
