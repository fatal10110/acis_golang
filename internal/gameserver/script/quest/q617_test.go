package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ617MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference parses the
// chosen item id as a 32-bit integer without a literal (base 10, 32 bits).
func TestQ617MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q617", []string{"quest.Q617_GatherTheFlames"}, "number 10", "number 32")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ617PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ617PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q617", "quest/Q617_GatherTheFlames")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
