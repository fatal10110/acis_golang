package drchaos

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The NpcString ids Doctor
// Chaos says are named constants in the reference, numbers in Go. The
// party lookup is the player handle's PartyMembers, which the census
// also maps from Quest.getPartyMembers.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.ai.boss.drchaos.DoctorChaosNpc"}, "number 1010578", "number 1010579", "number 1010580", "call Quest.getPartyMembers")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
