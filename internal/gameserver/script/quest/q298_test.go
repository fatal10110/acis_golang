package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ298MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ298MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q298", []string{"quest.Q298_LizardmensConspiracy"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ298PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ298PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q298", "quest/Q298_LizardmensConspiracy")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
