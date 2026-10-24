package quest

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ086MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class and the
// NpcStrings are named constants in the reference; the saga's name keys
// its family entry (see q070).
func TestQ086MatchesReferenceFingerprints(t *testing.T) {
	exempt := []string{"number 98", `string "Q086_SagaOfTheHierophant"`}
	for id := 8650; id <= 8667; id++ {
		exempt = append(exempt, "number "+strconv.Itoa(id))
	}
	problems, err := scriptfp.Check("q086", []string{"quest.Q086_SagaOfTheHierophant"}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ086PagesAreInTheDatapack requires every page the saga core names to be
// a page of the saga's datapack directory.
func TestQ086PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "thirdclass", "quest/Q086_SagaOfTheHierophant")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
