package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ605MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference's 15 and 21
// are the initial sizes of its two chance maps, which Go has no use for.
func TestQ605MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q605", []string{"quest.Q605_AllianceWithKetraOrcs"}, "number 15", "number 21")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ605PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ605PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q605", "quest/Q605_AllianceWithKetraOrcs")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
