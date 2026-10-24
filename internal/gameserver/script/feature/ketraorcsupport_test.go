package feature

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestKetraorcsupportMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The base and bit size of
// the event's int32 parse are the Go call's own. The party's members are
// the Player handle's PartyMembers, which shares its name with the
// Quest helper's.
func TestKetraorcsupportMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("ketraorcsupport", []string{"script.feature.KetraOrcSupport"},
		"number 10", "number 32", "call Quest.getPartyMembers")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestKetraorcsupportPagesAreInTheDatapack requires every page the feature names to be a
// page of its datapack directory.
func TestKetraorcsupportPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "ketraorcsupport", "feature/KetraOrcSupport")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
