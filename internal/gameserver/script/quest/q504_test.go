package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ504MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference sizes its
// drop chance map for 5 entries, and its date pattern is Go's layout here.
func TestQ504MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q504", []string{"quest.Q504_CompetitionForTheBanditStronghold"},
		"number 5", `string "yyyy-MM-dd HH:mm:ss"`, `string "2006-01-02 15:04:05"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ504PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ504PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q504", "quest/Q504_CompetitionForTheBanditStronghold")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
