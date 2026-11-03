package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ508MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The system message id is a
// named constant in the reference, and the reward columns its indices 2
// and 3 read are struct fields here.
func TestQ508MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q508", []string{"quest.Q508_AClansReputation"},
		"number 1777", "number 2", "number 3")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ508PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory. The raid pages are built from the raid
// number, as in the reference.
func TestQ508PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q508", "quest/Q508_AClansReputation", "a.htm", "b.htm")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
