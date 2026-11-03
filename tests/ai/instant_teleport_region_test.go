package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// farJump is far enough to land outside the 3x3 block of regions around
// the start.
const farJump = 6000

// walkStep sends the character walking from from to to and runs one
// position update.
func (w *warriorWorld) walkStep(t *testing.T, from, to location.Location) [][]byte {
	t.Helper()
	p := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	for _, v := range []int{to.X, to.Y, to.Z, from.X, from.Y, from.Z} {
		p.WriteInt32(int32(v))
	}
	p.WriteInt32(1)
	w.srv.Client.Send(p.Bytes())
	w.srv.Settle(t)
	w.srv.TickPositions()
	return w.srv.ReadQueued(t, w.srv.Client)
}

// TestPlayerInstantTeleportKeepsItsRegion (Creature.instantTeleportTo:
// the position is set with no region edit): a character that jumps across
// region borders stays in its old region until it next moves. The monster
// that saw it before the jump keeps knowing it and keeps its hate, and the
// character learns nothing of the monster at its landing point. Its first
// step after the jump puts it in its new region: it then meets the monster
// there, and the one it left forgets it.
func TestPlayerInstantTeleportKeepsItsRegion(t *testing.T) {
	t.Parallel()
	w := bootWarriors(t, warriorTemplate(aggressiveID, nil), warriorTemplate(aggressiveTwoID, nil))
	near := w.spawn(t, aggressiveID, 1000, 0)
	to := location.Location{X: w.at.X + 850, Y: w.at.Y, Z: w.at.Z}
	w.instantTeleport(t, to)
	if got := near.AggroHate(w.player); got != 200 {
		t.Fatalf("hate after the near jump = %v, want 200", got)
	}

	farTo := location.Location{X: to.X + farJump, Y: to.Y, Z: to.Z}
	far := w.spawnAt(t, aggressiveTwoID, location.Location{X: farTo.X + 150, Y: farTo.Y, Z: farTo.Z})
	frames := w.instantTeleport(t, farTo)
	if got := len(framesOf(frames, serverpackets.OpcodeTeleportToLocation, w.player.ObjectID())); got != 1 {
		t.Fatalf("TeleportToLocation frames = %d, want 1", got)
	}
	if got := len(framesOf(frames, serverpackets.OpcodeNPCInfo, far.ObjectID())); got != 0 {
		t.Fatalf("NpcInfo frames for the monster at the landing point = %d, want none before the next move", got)
	}
	if got := len(framesOf(frames, serverpackets.OpcodeDeleteObject, near.ObjectID())); got != 0 {
		t.Fatalf("DeleteObject frames for the monster left behind = %d, want none before the next move", got)
	}
	live := w.player.(world.Tracked)
	if !world.Knows(near, live) || world.Knows(live, far) {
		t.Fatalf("knows: left-behind monster -> character %v, character -> landing monster %v; want true, false",
			world.Knows(near, live), world.Knows(live, far))
	}
	if got := near.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the monster left behind = %v, want its 200 kept", got)
	}

	frames = w.walkStep(t, farTo, location.Location{X: farTo.X, Y: farTo.Y + 400, Z: farTo.Z})
	if got := len(framesOf(frames, serverpackets.OpcodeNPCInfo, far.ObjectID())); got != 1 {
		t.Fatalf("NpcInfo frames for the landing monster after the step = %d, want 1", got)
	}
	if got := len(framesOf(frames, serverpackets.OpcodeDeleteObject, near.ObjectID())); got != 1 {
		t.Fatalf("DeleteObject frames for the monster left behind after the step = %d, want 1", got)
	}
}

// TestNPCInstantTeleportKeepsItsRegion (Creature.instantTeleportTo): a
// monster that jumps across region borders stays in its old region until
// it next moves, so the character beside its start keeps seeing it: it
// gets the fast TeleportToLocation and no DeleteObject. The monster's
// first movement step puts it in its new region, and the character then
// forgets it.
func TestNPCInstantTeleportKeepsItsRegion(t *testing.T) {
	t.Parallel()
	w := bootWarriors(t, warriorTemplate(warriorID, nil))
	h := w.spawn(t, warriorID, 150, 0)

	to := location.Location{X: w.at.X + farJump, Y: w.at.Y, Z: w.at.Z}
	done := make(chan struct{})
	h.Queue().Post(func() {
		defer close(done)
		script.NPCOf(h).InstantTeleportTo(script.Loc{X: to.X, Y: to.Y, Z: to.Z})
	})
	<-done
	w.srv.Settle(t)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := len(framesOf(frames, serverpackets.OpcodeTeleportToLocation, h.ObjectID())); got != 1 {
		t.Fatalf("TeleportToLocation frames = %d, want 1", got)
	}
	if got := len(framesOf(frames, serverpackets.OpcodeDeleteObject, h.ObjectID())); got != 0 {
		t.Fatalf("DeleteObject frames = %d, want none before the monster moves", got)
	}
	if x, y, z := h.Position(); (location.Location{X: x, Y: y, Z: z}) != to {
		t.Fatalf("monster stands at %d,%d,%d, want %v", x, y, z, to)
	}
	if !world.Knows(w.player.(world.Tracked), h) {
		t.Fatal("the character forgot the monster at its jump")
	}

	done = make(chan struct{})
	h.Queue().Post(func() {
		defer close(done)
		h.SyncPosition(location.Location{X: to.X + 10, Y: to.Y, Z: to.Z})
	})
	<-done
	w.srv.Settle(t)
	if got := len(framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeDeleteObject, h.ObjectID())); got != 1 {
		t.Fatalf("DeleteObject frames after the monster's step = %d, want 1", got)
	}
}
