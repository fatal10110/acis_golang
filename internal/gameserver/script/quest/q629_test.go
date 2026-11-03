package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ629MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ629MatchesReferenceFingerprints(t *testing.T) {
	problems, err := // 10 is the reference's initial size of its drop map, which a Go switch has no use for.
		scriptfp.Check("q629", []string{"quest.Q629_CleanUpTheSwampOfScreams"}, "number 10")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ629PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ629PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q629", "quest/Q629_CleanUpTheSwampOfScreams")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
