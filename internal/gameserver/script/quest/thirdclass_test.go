package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestThirdclassMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The reference sizes the
// map of sagas for its 31 sagas; the family's cell needs no size. The
// party's members (Party.getMembers) are the Player handle's PartyMembers,
// whose name the check also matches to the quest helper
// Quest.getPartyMembers, which the class does not call.
func TestThirdclassMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("thirdclass", []string{"quest.ThirdClassQuest"}, "number 31", "call Quest.getPartyMembers")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
