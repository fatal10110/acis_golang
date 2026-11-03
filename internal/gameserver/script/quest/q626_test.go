package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ626MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ626MatchesReferenceFingerprints(t *testing.T) {
	problems, err := // 14 is the reference's initial size of its drop map, which a Go switch has no use for.
		scriptfp.Check("q626", []string{"quest.Q626_ADarkTwilight"}, "number 14")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ626PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ626PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q626", "quest/Q626_ADarkTwilight")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
