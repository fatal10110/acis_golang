package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ377MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ377MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q377", []string{"quest.Q377_ExplorationOfTheGiantsCave_Part2"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ377PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ377PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q377", "quest/Q377_ExplorationOfTheGiantsCave_Part2")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
