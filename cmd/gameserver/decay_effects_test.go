package main

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestCorpseDecayClaimsTheRespawnOnlyOnWinningTheDecay: the corpse-decay
// task resolves a spawned NPC's respawn hook only once it has won the NPC's
// decay, so a decay that lost to a removal already under way arms nothing.
func TestCorpseDecayClaimsTheRespawnOnlyOnWinningTheDecay(t *testing.T) {
	newFolk := func(id int32) *npc.Folk {
		f, err := npc.NewFolk(&npc.Instance{ObjectID: id, Kind: "Folk", Template: &npc.Template{ID: int(id), Type: "Folk"}})
		if err != nil {
			t.Fatalf("new folk: %v", err)
		}
		return f
	}
	state := world.New()
	effects := &worldDecayEffects{state: state}
	var calls []int32
	var decayedAtCall []bool
	byID := map[int32]*npc.Folk{}
	effects.SetRespawnHook(func(id int32) func() {
		calls = append(calls, id)
		decayedAtCall = append(decayedAtCall, byID[id].Decayed())
		return nil
	})

	live := newFolk(1)
	byID[1] = live
	state.AddObject(live)
	effects.Decay(live)
	if len(calls) != 1 || calls[0] != 1 || !decayedAtCall[0] {
		t.Fatalf("respawn hook calls = %v, decayed at the call = %v; want one call, after the decay was won", calls, decayedAtCall)
	}

	// A removal that won the decay but has not left the world yet.
	lost := newFolk(2)
	byID[2] = lost
	state.AddObject(lost)
	lost.DecayWithRespawn(nil, nil)
	effects.Decay(lost)
	if len(calls) != 1 {
		t.Fatalf("respawn hook calls = %v; a decay that lost claimed the respawn", calls)
	}
}
