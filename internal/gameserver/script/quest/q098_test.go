package quest

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ098MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class and the
// NpcStrings are named constants in the reference; the saga's name keys
// its family entry (see q070).
func TestQ098MatchesReferenceFingerprints(t *testing.T) {
	exempt := []string{"number 112", `string "Q098_SagaOfTheShillienSaint"`}
	for id := 9850; id <= 9867; id++ {
		exempt = append(exempt, "number "+strconv.Itoa(id))
	}
	problems, err := scriptfp.Check("q098", []string{"quest.Q098_SagaOfTheShillienSaint"}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ098PagesAreInTheDatapack requires every page the saga core names to be
// a page of the saga's datapack directory.
func TestQ098PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "thirdclass", "quest/Q098_SagaOfTheShillienSaint")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
