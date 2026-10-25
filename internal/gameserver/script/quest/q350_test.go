package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ350MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes.
//
// Exempt: the absorb lookups key on the player handle and read the
// crystal's object id as a field, so no object id call is made; the five
// soul crystal system messages and the three absorb types are named
// constants in the reference.
func TestQ350MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q350", []string{"quest.Q350_EnhanceYourWeapon"},
		"call WorldObject.getObjectId",
		"number 974", "number 975", "number 976", "number 977", "number 978",
		`string "FULL_PARTY"`, `string "LAST_HIT"`, `string "PARTY_ONE_RANDOM"`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ350PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ350PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q350", "quest/Q350_EnhanceYourWeapon")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
