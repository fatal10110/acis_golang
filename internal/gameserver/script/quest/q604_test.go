package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ604MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The ids of the raid's chat lines are named
// constants in the reference.
func TestQ604MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q604", []string{"quest.Q604_DaimonTheWhiteEyed_Part2"}, "number 60403", "number 60404")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ604PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ604PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q604", "quest/Q604_DaimonTheWhiteEyed_Part2")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
