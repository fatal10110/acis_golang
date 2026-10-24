package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ004MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference's talk on
// an NPC outside its gift table aborts on a null gift, the port on a named
// panic.
func TestQ004MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q004", []string{"quest.Q004_LongliveThePaagrioLord"},
		`string "q004: no gift of npc "`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ004PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ004PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q004", "quest/Q004_LongliveThePaagrioLord")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
