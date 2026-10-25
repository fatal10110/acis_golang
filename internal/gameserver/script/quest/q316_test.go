package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ316MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString id of
// Varool's cry is a named constant in the reference.
func TestQ316MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q316", []string{"quest.Q316_DestroyPlagueCarriers"}, "number 31603")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ316PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ316PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q316", "quest/Q316_DestroyPlagueCarriers")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
