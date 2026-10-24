package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ104MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ104MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q104", []string{"quest.Q104_SpiritOfMirrors"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ104PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ104PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q104", "quest/Q104_SpiritOfMirrors")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
