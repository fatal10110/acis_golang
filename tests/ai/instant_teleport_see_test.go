package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// instantTeleport jumps the character to to through the script handle, on
// its queue as a hook would, and returns the frames it was sent.
func (w *warriorWorld) instantTeleport(t *testing.T, to location.Location) [][]byte {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		script.PlayerOf(w.player).InstantTeleportTo(script.Loc{X: to.X, Y: to.Y, Z: to.Z})
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// TestInstantTeleportTellsTheMonstersItLandsBy shows the see-creature
// re-fire at the end of a player's instant teleport (Creature.java
// instantTeleportTo, forceSeeCreature; golden aggro.teleport_see) on the
// real WarriorAggressive chain, with no AI tick run: a character that jumps
// next to a WarriorAggressive monster out of its sight is seen once, so the
// monster wants to attack it with the see-creature hook's 200, and the
// character gets one fast TeleportToLocation for the jump. A jump from
// three regions away tells a monster at the landing point nothing, as the
// reference scans the NPCs around the region the jumper left.
func TestInstantTeleportTellsTheMonstersItLandsBy(t *testing.T) {
	t.Parallel()
	w := bootWarriors(t, warriorTemplate(aggressiveID, nil), warriorTemplate(aggressiveTwoID, nil))
	near := w.spawn(t, aggressiveID, 1000, 0)
	if got := near.AggroHate(w.player); got != 0 {
		t.Fatalf("hate before the jump = %v, want none: the monster stands out of sight", got)
	}

	to := location.Location{X: w.at.X + 850, Y: w.at.Y, Z: w.at.Z}
	frames := framesOf(w.instantTeleport(t, to), serverpackets.OpcodeTeleportToLocation, w.player.ObjectID())
	if len(frames) != 1 {
		t.Fatalf("TeleportToLocation frames = %d, want 1", len(frames))
	}
	if at, fast := teleportFrame(frames[0]); at != to || !fast {
		t.Fatalf("TeleportToLocation = %v fast=%v, want %v fast", at, fast, to)
	}
	if got := near.AggroHate(w.player); got != 200 {
		t.Fatalf("hate after the jump = %v, want 200: the see-creature hook runs once", got)
	}

	far := w.spawnAt(t, aggressiveTwoID, location.Location{X: to.X + 6150, Y: to.Y, Z: to.Z})
	farTo := location.Location{X: to.X + 6000, Y: to.Y, Z: to.Z}
	frames = framesOf(w.instantTeleport(t, farTo), serverpackets.OpcodeTeleportToLocation, w.player.ObjectID())
	if len(frames) != 1 {
		t.Fatalf("far TeleportToLocation frames = %d, want 1", len(frames))
	}
	if got := far.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the monster three regions from the jump's start = %v, want none", got)
	}
}

// spawnAt places an NPC of id at at through the live population, so its
// created hooks run.
func (w *warriorWorld) spawnAt(t *testing.T, id int32, at location.Location) *npc.Hostile {
	t.Helper()
	return w.spawn(t, id, at.X-w.at.X, at.Y-w.at.Y)
}
