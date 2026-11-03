package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ627MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ627MatchesReferenceFingerprints(t *testing.T) {
	problems, err := // 14 and 5 are the reference's initial sizes of its drop map and reward map, which Go switches have no use for.
		scriptfp.Check("q627", []string{"quest.Q627_HeartInSearchOfPower"}, "number 14", "number 5")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ627PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory. Its page 31518-7.htm, which answers a reward
// asked for without the gem, is none: the datapack ships 31518-07.htm only.
func TestQ627PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q627", "quest/Q627_HeartInSearchOfPower", "31518-7.htm")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
