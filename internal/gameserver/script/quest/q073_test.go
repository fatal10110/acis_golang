package quest

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ073MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class and the
// NpcStrings are named constants in the reference; the saga's name keys
// its family entry (see q070).
func TestQ073MatchesReferenceFingerprints(t *testing.T) {
	exempt := []string{"number 88", `string "Q073_SagaOfTheDuelist"`}
	for id := 7350; id <= 7367; id++ {
		// 7364 is also an amulet's item id, a literal on both sides.
		if id != 7364 {
			exempt = append(exempt, "number "+strconv.Itoa(id))
		}
	}
	problems, err := scriptfp.Check("q073", []string{"quest.Q073_SagaOfTheDuelist"}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ073PagesAreInTheDatapack requires every page the saga core names to be
// a page of the saga's datapack directory.
func TestQ073PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "thirdclass", "quest/Q073_SagaOfTheDuelist")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
