package quest

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ090MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class and the
// NpcStrings are named constants in the reference; the saga's name keys
// its family entry (see q070).
func TestQ090MatchesReferenceFingerprints(t *testing.T) {
	exempt := []string{"number 110", `string "Q090_SagaOfTheStormScreamer"`}
	for id := 9050; id <= 9067; id++ {
		exempt = append(exempt, "number "+strconv.Itoa(id))
	}
	problems, err := scriptfp.Check("q090", []string{"quest.Q090_SagaOfTheStormScreamer"}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ090PagesAreInTheDatapack requires every page the saga core names to be
// a page of the saga's datapack directory.
func TestQ090PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "thirdclass", "quest/Q090_SagaOfTheStormScreamer")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
