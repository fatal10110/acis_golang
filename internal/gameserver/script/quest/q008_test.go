package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ008MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
func TestQ008MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q008", []string{"quest.Q008_AnAdventureBegins"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ008PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory, except Harne's last page, which the
// datapack does not ship: the dialog shows its missing-page notice.
func TestQ008PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q008", "quest/Q008_AnAdventureBegins", "30144-03.htm")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
