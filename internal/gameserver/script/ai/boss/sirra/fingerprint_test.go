package sirra

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// class it ports: literals, engine calls and hook shapes. The deacon names
// its chat lines and the overweight refusal by id, where the reference
// names them by constant. It fills its Silver Hemocyte refusal page in
// itself and shows it through Player.ShowHTML, so the page file it reads
// is no Player.ShowPage call. It walks the party through
// Player.PartyMembers, the Go name it shares with the quest helper
// Quest.getPartyMembers, which the class does not call.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.boss.sirra.FreyaDeacon",
	},
		"number 1118", "number 1121000", "number 1121001", "number 1121003", "number 1121004", "number 1121005", "number 1121007",
		"call NpcHtmlMessage.setFile", "call Quest.getPartyMembers")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
