package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ426MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference lists the
// fishing-spot monster 21263 in two tables and only the first is ever read,
// so its second chance, 795, is not in the Go package.
func TestQ426MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q426", []string{"quest.Q426_QuestForFishingShot"}, "number 795")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ426PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ426PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q426", "quest/Q426_QuestForFishingShot")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
