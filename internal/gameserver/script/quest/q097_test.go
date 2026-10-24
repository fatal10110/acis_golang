package quest

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ097MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class and the
// NpcStrings are named constants in the reference; the saga's name keys
// its family entry (see q070).
func TestQ097MatchesReferenceFingerprints(t *testing.T) {
	exempt := []string{"number 106", `string "Q097_SagaOfTheShillienTemplar"`}
	for id := 9750; id <= 9767; id++ {
		exempt = append(exempt, "number "+strconv.Itoa(id))
	}
	problems, err := scriptfp.Check("q097", []string{"quest.Q097_SagaOfTheShillienTemplar"}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ097PagesAreInTheDatapack requires every page the saga core names to be
// a page of the saga's datapack directory.
func TestQ097PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "thirdclass", "quest/Q097_SagaOfTheShillienTemplar")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
