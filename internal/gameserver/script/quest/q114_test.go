package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ114MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids of the
// golem's lines and of the stone's screen message are named constants in
// the reference.
func TestQ114MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q114", []string{"quest.Q114_ResurrectionOfAnOldManager"},
		"number 11450", "number 11451", "number 11452", "number 11453")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ114PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ114PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q114", "quest/Q114_ResurrectionOfAnOldManager")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
