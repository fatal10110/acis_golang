package ai

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// teleportFrame reads a TeleportToLocation frame: its destination and its
// fast flag.
func teleportFrame(f []byte) (location.Location, bool) {
	v := func(i int) int32 { return int32(binary.LittleEndian.Uint32(f[5+4*i:])) }
	return location.Location{X: int(v(0)), Y: int(v(1)), Z: int(v(2))}, v(3) == 1
}

// TestScriptInstantTeleport moves a player and a hostile NPC the way the
// Warrior base swaps two players and pulls a stuck NPC to its attacker: each
// lands at the point in one step, the character sees one fast
// TeleportToLocation for each, and both stay in the world.
func TestScriptInstantTeleport(t *testing.T) {
	t.Parallel()
	w := bootWarriors(t, warriorTemplate(warriorID, nil))
	h := w.spawn(t, warriorID, 150, 0)

	to := location.Location{X: w.at.X + 300, Y: w.at.Y + 40, Z: w.at.Z}
	done := make(chan struct{})
	w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		script.PlayerOf(w.player).InstantTeleportTo(script.Loc{X: to.X, Y: to.Y, Z: to.Z})
	})
	<-done
	w.srv.Settle(t)
	frames := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeTeleportToLocation, w.player.ObjectID())
	if len(frames) != 1 {
		t.Fatalf("player TeleportToLocation frames = %d, want 1", len(frames))
	}
	if at, fast := teleportFrame(frames[0]); at != to || !fast {
		t.Fatalf("player TeleportToLocation = %v fast=%v, want %v fast", at, fast, to)
	}
	if x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID()); (location.Location{X: x, Y: y, Z: z}) != to {
		t.Fatalf("player stands at %d,%d,%d, want %v", x, y, z, to)
	}

	npcTo := location.Location{X: to.X + 50, Y: to.Y, Z: to.Z}
	done = make(chan struct{})
	h.Queue().Post(func() {
		defer close(done)
		script.NPCOf(h).InstantTeleportTo(script.Loc{X: npcTo.X, Y: npcTo.Y, Z: npcTo.Z})
	})
	<-done
	w.srv.Settle(t)
	frames = framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeTeleportToLocation, h.ObjectID())
	if len(frames) != 1 {
		t.Fatalf("npc TeleportToLocation frames = %d, want 1", len(frames))
	}
	if at, fast := teleportFrame(frames[0]); at != npcTo || !fast {
		t.Fatalf("npc TeleportToLocation = %v fast=%v, want %v fast", at, fast, npcTo)
	}
	if x, y, z := h.Position(); (location.Location{X: x, Y: y, Z: z}) != npcTo {
		t.Fatalf("npc stands at %d,%d,%d, want %v", x, y, z, npcTo)
	}
	if _, ok := w.srv.State.Object(h.ObjectID()); !ok {
		t.Fatal("the npc left the world")
	}
}
