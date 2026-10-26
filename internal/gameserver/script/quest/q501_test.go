package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestQ501MatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The id of the chat line is a
// named constant in the reference. It walks the party through
// Player.PartyMembers, the Go name it shares with the quest helper
// Quest.getPartyMembers, which the class does not call.
func TestQ501MatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("q501", []string{"quest.Q501_ProofOfClanAlliance"}, "number 50110", "call Quest.getPartyMembers")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestQ501PagesAreInTheDatapack requires every page the quest names to be a
// page of its datapack directory.
func TestQ501PagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, "q501", "quest/Q501_ProofOfClanAlliance")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
