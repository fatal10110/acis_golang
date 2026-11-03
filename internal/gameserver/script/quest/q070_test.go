package quest

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ070MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class and the
// NpcStrings are named constants in the reference. The saga's name is the
// reference's class name: the family keeps it so the core finds a party
// member's state in the saga, where the reference keeps the saga object.
func TestQ070MatchesReferenceFingerprints(t *testing.T) {
	exempt := []string{"number 90", `string "Q070_SagaOfThePhoenixKnight"`}
	for id := 7050; id <= 7067; id++ {
		exempt = append(exempt, "number "+strconv.Itoa(id))
	}
	problems, err := scriptfp.Check("q070", []string{"quest.Q070_SagaOfThePhoenixKnight"}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ070PagesAreInTheDatapack requires every page the saga core names to be
// a page of the saga's datapack directory.
func TestQ070PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "thirdclass", "quest/Q070_SagaOfThePhoenixKnight")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
