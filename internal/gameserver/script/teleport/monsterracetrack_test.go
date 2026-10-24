package teleport

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMonsterracetrackMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The player memo's write
// and removal are the Player handle's SetMemo and UnsetMemo; the census
// rows of MemoSet are shared with the quest state's variables. The 12 is
// the reference map's initial capacity, which nothing observes; the 10 and
// 32 are the base and bit size of the memo's parse, which rejects a value
// past int32 as the reference's parseInt does. A memo
// naming a gatekeeper outside the table aborts the reference's talk on a
// null location, the port's on a named panic. The teleporter names no
// page, so there is no page check.
func TestMonsterracetrackMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check("monsterracetrack", []string{"script.teleport.MonsterRaceTrack"},
		"call MemoSet.set", "call MemoSet.unset", "number 12", "number 10", "number 32",
		`string "monsterracetrack: no return point for gatekeeper "`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
