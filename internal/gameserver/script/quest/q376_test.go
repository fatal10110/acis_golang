package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ376MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ376MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q376", []string{"quest.Q376_ExplorationOfTheGiantsCave_Part1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ376PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ376PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q376", "quest/Q376_ExplorationOfTheGiantsCave_Part1")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
