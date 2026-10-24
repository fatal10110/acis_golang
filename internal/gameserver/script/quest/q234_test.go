package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ234MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference sizes its
// two maps (13), names the NpcString of Baium's cry, and parses the chosen
// A-grade id as a 32-bit integer without a literal; its unknown weapon
// aborts on a null name, the port on a named panic.
func TestQ234MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q234", []string{"quest.Q234_FatesWhisper"},
		"number 13", "number 23434", "number 32", `string "q234: no B-grade weapon "`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ234PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ234PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q234", "quest/Q234_FatesWhisper")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
