package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ119MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ119MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q119", []string{"quest.Q119_LastImperialPrince"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ119PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory, except the page Devorin answers with to a
// character without the brooch, which the datapack does not ship: the
// dialog shows its missing-page notice.
func TestQ119PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q119", "quest/Q119_LastImperialPrince", "31453-02a.htm")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
