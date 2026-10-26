package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ659MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ659MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q659", []string{"quest.Q659_IdRatherBeCollectingFairyBreath"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ659PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ659PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q659", "quest/Q659_IdRatherBeCollectingFairyBreath")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
