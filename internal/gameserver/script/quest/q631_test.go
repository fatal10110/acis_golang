package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ631MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ631MatchesReferenceFingerprints(t *testing.T) {
	problems, err := // 24 is the reference's initial size of its drop map, which a Go switch has no use for; 32 is the bit size of the ParseInt of the reward index.
		scriptfp.Check("q631", []string{"quest.Q631_DeliciousTopChoiceMeat"}, "number 24", "number 32")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ631PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ631PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q631", "quest/Q631_DeliciousTopChoiceMeat")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
