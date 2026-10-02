package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// fixedIntention is an IntentionSource whose current intention acts on id.
type fixedIntention int32

func (f fixedIntention) IntentionFinalTarget() int32 { return int32(f) }

// TestSummonCastMainTargetReadsOwnerIntention judges a summon's single-target
// cast as its acting player's (ownCast false): a CTRL damage skill on a
// party mate or an unflagged stranger passes only while the player's own
// current intention acts on that target. Other branches ignore the main
// target.
func TestSummonCastMainTargetReadsOwnerIntention(t *testing.T) {
	cases := []struct {
		name    string
		g       policyGraph
		ts      policySide
		aimed   bool
		allowed bool
	}{
		{name: "party mate the player aims at", g: samePartyGraph(), aimed: true, allowed: true},
		{name: "party mate the player does not aim at", g: samePartyGraph()},
		{name: "white stranger the player aims at", aimed: true, allowed: true},
		{name: "white stranger the player does not aim at"},
		{name: "flagged stranger needs no main target", ts: policySide{flag: task.PvPFlagOn}, allowed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, tgt := policyPair(t, tc.g, policySide{}, tc.ts)
			aim := fixedIntention(0)
			if tc.aimed {
				aim = fixedIntention(tgt.ObjectID())
			}
			c.SetIntentionSource(aim)
			if got := c.CanCastOnPlayable(tgt, policyDamage, true, true, false); got != tc.allowed {
				t.Fatalf("summon's CTRL damage cast allowed = %v, want %v", got, tc.allowed)
			}
			// The player's own cast always aims at its final target.
			if !c.CanCastOnPlayable(tgt, policyDamage, true, true, true) {
				t.Fatal("player's own CTRL damage cast refused, want allowed")
			}
		})
	}
}

// TestIntentionAimsAtWithoutSource reports no main target for a player
// whose intention source is not wired yet.
func TestIntentionAimsAtWithoutSource(t *testing.T) {
	c, tgt := policyPair(t, policyGraph{}, policySide{}, policySide{})
	if c.IntentionAimsAt(tgt) {
		t.Fatal("IntentionAimsAt with no source = true, want false")
	}
	if c.CanCastOnPlayable(tgt, policyDamage, true, true, false) {
		t.Fatal("summon's CTRL damage cast allowed with no owner intention, want refused")
	}
}
