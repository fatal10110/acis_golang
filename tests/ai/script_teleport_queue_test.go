package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestNPCHookTeleportsAWalkingPlayerOnItsQueue pins where a script's
// teleport of a player asked from an NPC's queue runs: on the player's own
// queue, once the hook's task is done, so it never touches the player's
// move, cast or selection while that queue runs them. The rest of the hook
// still reads the player's old position; the player then gets one
// TeleportToLocation, stands at the destination, and its walk is over.
func TestNPCHookTeleportsAWalkingPlayerOnItsQueue(t *testing.T) {
	t.Parallel()
	w := bootWarriors(t, warriorTemplate(warriorID, nil))
	h := w.spawn(t, warriorID, 1000, 0)
	walkTo := location.Location{X: w.at.X, Y: w.at.Y + 2000, Z: w.at.Z}
	w.walkStep(t, w.at, walkTo)
	x, y, z := w.player.Position()
	before := location.Location{X: x, Y: y, Z: z}
	if before == w.at {
		t.Fatal("test setup: the character did not start walking")
	}

	dest := location.Location{X: w.at.X + 300, Y: w.at.Y - 300, Z: w.at.Z}
	var during location.Location
	done := make(chan struct{})
	if !h.Queue().Post(func() {
		defer close(done)
		script.PlayerOf(w.player).TeleportTo(script.Loc{X: dest.X, Y: dest.Y, Z: dest.Z}, 0)
		x, y, z := w.player.Position()
		during = location.Location{X: x, Y: y, Z: z}
	}) {
		t.Fatal("the monster's queue is closed")
	}
	<-done
	if during != before {
		t.Fatalf("the hook read the character at %v after asking the teleport, want its old %v", during, before)
	}
	w.srv.TickPositions()
	w.srv.Settle(t)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if got := len(framesOf(frames, serverpackets.OpcodeTeleportToLocation, w.player.ObjectID())); got != 1 {
		t.Fatalf("TeleportToLocation frames = %d, want 1", got)
	}
	if x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID()); x != dest.X || y != dest.Y {
		t.Fatalf("character at %d,%d, want the destination %d,%d", x, y, dest.X, dest.Y)
	}
	w.srv.TickPositions()
	if x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID()); x != dest.X || y != dest.Y {
		t.Fatalf("character walked on to %d,%d after the teleport, want it kept at %d,%d", x, y, dest.X, dest.Y)
	}
}
