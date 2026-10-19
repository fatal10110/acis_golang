package q093

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class and the
// NpcStrings are named constants in the reference; the saga's name keys
// its family entry (see q070).
func TestMatchesReferenceFingerprints(t *testing.T) {
	exempt := []string{"number 111", `string "Q093_SagaOfTheSpectralMaster"`}
	for id := 9350; id <= 9367; id++ {
		exempt = append(exempt, "number "+strconv.Itoa(id))
	}
	problems, err := scriptfp.Check(".", []string{"quest.Q093_SagaOfTheSpectralMaster"}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestPagesAreInTheDatapack requires every page the saga core names to be
// a page of the saga's datapack directory.
func TestPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "../thirdclass", "quest/Q093_SagaOfTheSpectralMaster")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
