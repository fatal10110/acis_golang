package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Restart destinations of observerRestartTable: the colosseum's own map
// region restarts at colosseumTown, every other region at homeTown.
var (
	homeTown      = location.Location{X: 200, Y: 400, Z: gameservertest.SpawnZ}
	colosseumTown = location.Location{X: 147000, Y: 46000, Z: -3000}
)

func observerRestartTable() *restart.Table {
	seat := location.Point{
		X: (colosseumSeat.X-world.MinX)/world.TileSize + world.TileXMin,
		Y: (colosseumSeat.Y-world.MinY)/world.TileSize + world.TileYMin,
	}
	var elsewhere []location.Point
	for x := geo.TileXMin; x <= geo.TileXMax; x++ {
		for y := geo.TileYMin; y <= geo.TileYMax; y++ {
			if p := (location.Point{X: x, Y: y}); p != seat {
				elsewhere = append(elsewhere, p)
			}
		}
	}
	return &restart.Table{Points: []restart.Point{
		{Name: "Colosseum", Points: []location.Location{colosseumTown}, ChaoPoints: []location.Location{colosseumTown}, MapRegions: []location.Point{seat}},
		{Name: "Home", Points: []location.Location{homeTown}, ChaoPoints: []location.Location{homeTown}, MapRegions: elsewhere},
	}}
}

func encodeRestartPoint(requestType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestRestartPoint)
	w.WriteInt32(requestType)
	return w.Bytes()
}

// TestDeadPlayerCannotObserve pins #4953: a dead player following a
// tower's viewpoint link, from a page opened before it died, is refused
// with the bypass's closing ActionFailed alone; it pays nothing, stays where it lies and
// watches nothing.
func TestDeadPlayerCannotObserve(t *testing.T) {
	t.Parallel()
	w, tower := bootTowerWorld(t, 1000)
	w.openAnyNpcPage(t)
	w.srv.MarkPlayerDead(t, w.player)
	drainFrames(t, w.c)

	frames := w.observeFrames(t, npcCommand(tower, "observe 634"))
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("dead observe = %x, want ActionFailed alone", opcodes(frames))
	}
	c := w.character(t)
	if c.ObserverMode() || c.Invisible() || c.Invul() || c.Paralyzed() {
		t.Fatalf("after dead observe: observer=%v invisible=%v invul=%v paralyzed=%v, want all off",
			c.ObserverMode(), c.Invisible(), c.Invul(), c.Paralyzed())
	}
	if _, ok := c.SavedLocation(); ok {
		t.Fatal("dead observe kept a saved location")
	}
	if got := c.CurrentLocation(); got != w.at {
		t.Fatalf("dead player stands at %+v, want %+v", got, w.at)
	}
	if got := w.held(t, item.AdenaID); got != 1000 {
		t.Fatalf("adena held = %d, want 1000", got)
	}
}

// TestObserverRestartEndsObservationFirst pins #4953: an observer that dies
// and asks for a restart point stops watching and goes back where it left
// to watch, then restarts from there, not from the viewpoint. A second
// request while the jump back lands is dropped instead of reviving it in
// place.
func TestObserverRestartEndsObservationFirst(t *testing.T) {
	t.Parallel()
	w, tower := bootTowerWorld(t, 1000, gameservertest.WithRestartPoints(observerRestartTable()))
	left := w.at
	w.openAnyNpcPage(t)
	w.observeFrames(t, npcCommand(tower, "observe 634"))
	w.c.Send(encodeAppearing())
	drainFrames(t, w.c)
	w.srv.MarkPlayerDead(t, w.player)
	drainFrames(t, w.c)

	w.c.Send(encodeRestartPoint(0))
	w.c.Send(encodeRestartPoint(0))
	frames := drainFrames(t, w.c)
	if _, ok := firstOpcode(frames, serverpackets.OpcodeRevive); ok {
		t.Fatalf("restart before landing back = %x, want no revive yet", opcodes(frames))
	}
	kept := w.observerKept(frames)
	if end, ok := firstOpcode(kept, serverpackets.OpcodeObserverEnd); !ok || string(end) != string(observerEnd(left)) {
		t.Fatalf("observer restart = %x, want ObserverEnd at %v", opcodes(kept), left)
	}
	if n := countOpcode(kept, serverpackets.OpcodeTeleportToLocation); n != 1 {
		t.Fatalf("observer restart sent %d own TeleportToLocation, want 1 (back where it left)", n)
	}
	assertLandedNear(t, landing(t, kept), left)
	c := w.character(t)
	// Invul stays on only for the jump in flight.
	if c.ObserverMode() || c.Invisible() || c.Paralyzed() || !c.Dead() {
		t.Fatalf("jumping back: observer=%v invisible=%v paralyzed=%v dead=%v, want only dead",
			c.ObserverMode(), c.Invisible(), c.Paralyzed(), c.Dead())
	}
	if _, ok := c.SavedLocation(); ok {
		t.Fatal("observer restart kept the saved location")
	}

	w.c.Send(encodeAppearing())
	frames = w.observerKept(drainFrames(t, w.c))
	assertLandedNear(t, landing(t, frames), homeTown)
	w.c.Send(encodeAppearing())
	drainFrames(t, w.c)
	c = w.character(t)
	if c.Dead() || c.ObserverMode() || c.Invisible() || c.Invul() || c.Paralyzed() {
		t.Fatalf("after the restart: dead=%v observer=%v invisible=%v invul=%v paralyzed=%v, want all off",
			c.Dead(), c.ObserverMode(), c.Invisible(), c.Invul(), c.Paralyzed())
	}
	assertStandsNear(t, c.CurrentLocation(), homeTown)

	// The dropped second request left nothing waiting.
	w.c.Send(encodeAppearing())
	if frames := w.observerKept(drainFrames(t, w.c)); countOpcode(frames, serverpackets.OpcodeTeleportToLocation) != 0 {
		t.Fatalf("after landing in town = %x, want no further jump", opcodes(frames))
	}
}

func countOpcode(frames [][]byte, op byte) int {
	n := 0
	for _, f := range frames {
		if f[0] == op {
			n++
		}
	}
	return n
}
