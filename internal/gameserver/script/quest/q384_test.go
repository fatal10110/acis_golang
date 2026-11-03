package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ384MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference reads the
// lines of the board from a shared constant, which the port holds inline, and
// sizes its chance map; the port names the variable a pick finds missing.
func TestQ384MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q384", []string{"quest.Q384_WarehouseKeepersPastime"},
		"number 26", "number 4", "number 5", "number 6", "number 7", "number 8", `string "q384: no variable "`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ384PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ384PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q384", "quest/Q384_WarehouseKeepersPastime")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
