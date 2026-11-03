package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ633MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ633MatchesReferenceFingerprints(t *testing.T) {
	problems, err := // 10 and 16 are the reference's initial sizes of its two drop maps, which Go switches have no use for.
		scriptfp.Check("q633", []string{"quest.Q633_InTheForgottenVillage"}, "number 10", "number 16")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ633PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ633PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q633", "quest/Q633_InTheForgottenVillage")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
