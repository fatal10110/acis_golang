package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestFolkSummonFriendCastStepChecksTheWorldFirst follows
// AbstractAI.isTargetLost(WorldObject, L2Skill) for a civilian NPC's cast
// step: a SUMMON_FRIEND cast keeps a target the NPC does not know, but not
// one that is no longer registered in the world. The step is taken
// directly, as the swing-end and bow-reuse paths take the current
// intention's step without the selection's desire pruning first.
func TestFolkSummonFriendCastStepChecksTheWorldFirst(t *testing.T) {
	for _, tc := range []struct {
		name     string
		despawn  bool
		wantCast bool
	}{
		{"off the grid, still in the world", false, true},
		{"out of the world", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newFolkCastRig(t, 50)
			r.castAI.skillType = "SUMMON_FRIEND"
			if tc.despawn {
				r.f.world.Despawn(r.target)
			} else {
				r.f.world.Leave(r.target)
			}
			if r.f.Knows(r.target) {
				t.Fatal("NPC still knows its target")
			}

			r.f.cast.brain.Lock()
			r.f.castOn(&ai.Desire{Kind: ai.IntentionCast, FinalTarget: r.target, Skill: modelskill.Ref{ID: 1403, Level: 1}})
			r.f.cast.brain.Unlock()

			if got := len(r.castAI.casts) == 1; got != tc.wantCast {
				t.Fatalf("cast started = %v, want %v", got, tc.wantCast)
			}
			if got := r.events.moveToPawns(); !tc.wantCast && len(got) != 0 {
				t.Fatalf("MoveToPawn %+v toward a target out of the world, want none", got)
			}
		})
	}
}
