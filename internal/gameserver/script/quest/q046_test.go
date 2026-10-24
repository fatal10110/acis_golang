package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ046MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ046MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q046", []string{"quest.Q046_OnceMoreInTheArmsOfTheMotherTree"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ046PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ046PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q046", "quest/Q046_OnceMoreInTheArmsOfTheMotherTree")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
