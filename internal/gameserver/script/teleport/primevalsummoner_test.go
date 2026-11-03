package teleport

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestPrimevalsummonerMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. A party's members are read
// on the player handle (Party.getMembers), whose method shares its name
// with the quest helper Quest.getPartyMembers, which the class never calls.
func TestPrimevalsummonerMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("primevalsummoner", []string{"script.teleport.PrimevalSummoner"}, "call Quest.getPartyMembers")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestPrimevalsummonerPagesAreInTheDatapack requires every page the teleporter names to be a
// page of its datapack directory.
func TestPrimevalsummonerPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "primevalsummoner", "teleport/PrimevalSummoner")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
