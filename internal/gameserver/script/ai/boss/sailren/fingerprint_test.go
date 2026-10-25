package sailren

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes it ports: literals, engine calls and hook shapes. The dummy's
// two timer-name patterns, whole-name matches of digit alternatives, are
// case-insensitive comparisons with each alternative in Go. The statue
// walks the party through Player.PartyMembers, the Go name it shares with
// the quest helper Quest.getPartyMembers, which neither class calls.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.boss.sailren.SailrenDummy",
		"script.ai.boss.sailren.StatueOfShilen",
	}, `string "(?i)1001|1002|1003|1004"`, `string "(?i)1005|1006"`, "call Quest.getPartyMembers")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
