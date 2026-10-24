package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ014MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ014MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q014", []string{"quest.Q014_WhereaboutsOfTheArchaeologist"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ014PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ014PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q014", "quest/Q014_WhereaboutsOfTheArchaeologist")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
