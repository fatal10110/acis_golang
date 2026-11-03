package frintezza

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes it ports: literals, engine calls and hook shapes.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.boss.frintezza.AlarmDevice",
		"script.ai.boss.frintezza.EvilateA",
		"script.ai.boss.frintezza.EvilateB",
		"script.ai.boss.frintezza.FollowerDummy",
		"script.ai.boss.frintezza.FollowerOfFrintezza",
		"script.ai.boss.frintezza.FollowerOfFrintezzaTransform",
		"script.ai.boss.frintezza.Frintezza",
		"script.ai.boss.frintezza.FrintezzaSeeker",
		"script.ai.boss.frintezza.FrintezzaTeleporter",
		"script.ai.boss.frintezza.OrganDummy",
		"script.ai.boss.frintezza.PortraitSpirit",
		"script.ai.boss.frintezza.PortraitSpiritWinged",
		"script.ai.boss.frintezza.UndeadBandmaster",
		// Frintezza's on-screen song names, the alarm's shouts and the
		// teleporter's countdown and overweight refusal are named constants
		// in the reference, numbers in Go. The teleporter reads the channel
		// leader through Player.ChannelLeader where the census records the
		// group getter AbstractGroup.getLeader, and walks each entering
		// party's members in the lists Player.ChannelParties returns where
		// the census records Party.getMembers.
	}, "number 1010643", "number 1118",
		"call AbstractGroup.getLeader", "call Party.getMembers",
		"number 1010630", "number 1010631", "number 1000522", "number 1000523", "number 1000524",
		"number 1000525", "number 1000526", "number 1000527",
		"number 1010644", "number 1010645", "number 1010646")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
