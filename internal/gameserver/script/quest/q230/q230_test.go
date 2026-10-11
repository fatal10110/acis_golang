package q230

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The class ids the trial is
// open to are named constants in the reference, and so are the NpcStrings
// the duel monsters say.
func TestMatchesReferenceFingerprints(t *testing.T) {
	exempt := []string{"number 11", "number 26"}
	for id := 23060; id <= 23077; id++ {
		exempt = append(exempt, "number "+strconv.Itoa(id))
	}
	problems, err := scriptfp.Check(".", []string{"quest.Q230_TestOfTheSummoner"}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestPagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, ".", "quest/Q230_TestOfTheSummoner")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
