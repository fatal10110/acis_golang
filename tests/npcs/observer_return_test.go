package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestObserverReturnWaitsForTheEntryJump pins the Go-only guard on an
// ObserverReturn sent before the client reported the jump to the viewpoint
// landed: it is ignored, so the player stays a hidden, invulnerable,
// paralyzed observer instead of being left at the viewpoint as a plain
// player. Once it has appeared, ObserverReturn brings it back where it
// left, and the fee stays paid once.
func TestObserverReturnWaitsForTheEntryJump(t *testing.T) {
	t.Parallel()
	w, tower := bootTowerWorld(t, 1000)
	left := w.at
	w.openAnyNpcPage(t)
	w.observeFrames(t, npcCommand(tower, "observe 634"))

	w.c.Send(encodeObserverReturn())
	if frames := drainFrames(t, w.c); len(frames) != 0 {
		t.Fatalf("ObserverReturn before Appearing = %x, want silence", opcodes(frames))
	}
	c := w.character(t)
	if !c.ObserverMode() || !c.Invisible() || !c.Invul() || !c.Paralyzed() {
		t.Fatalf("after early ObserverReturn: observer=%v invisible=%v invul=%v paralyzed=%v, want all on",
			c.ObserverMode(), c.Invisible(), c.Invul(), c.Paralyzed())
	}
	if saved, ok := c.SavedLocation(); !ok || saved != left {
		t.Fatalf("saved location = %v %v, want %v", saved, ok, left)
	}

	w.c.Send(encodeAppearing())
	drainFrames(t, w.c)
	assertStandsNear(t, w.character(t).CurrentLocation(), colosseumSeat)

	w.c.Send(encodeObserverReturn())
	frames := w.observerKept(drainFrames(t, w.c))
	if end, ok := firstOpcode(frames, serverpackets.OpcodeObserverEnd); !ok || string(end) != string(observerEnd(left)) {
		t.Fatalf("ObserverReturn after Appearing = %x, want ObserverEnd at %v", opcodes(frames), left)
	}
	assertLandedNear(t, landing(t, frames), left)
	w.c.Send(encodeAppearing())
	drainFrames(t, w.c)
	c = w.character(t)
	if c.ObserverMode() || c.Invisible() || c.Invul() || c.Paralyzed() {
		t.Fatalf("after the return: observer=%v invisible=%v invul=%v paralyzed=%v, want all off",
			c.ObserverMode(), c.Invisible(), c.Invul(), c.Paralyzed())
	}
	assertStandsNear(t, c.CurrentLocation(), left)
	if got := w.held(t, item.AdenaID); got != 1000-colosseumFee {
		t.Fatalf("adena held = %d, want %d", got, 1000-colosseumFee)
	}
}

// assertStandsNear checks at is within the 20-unit teleport scatter of
// spot.
func assertStandsNear(t *testing.T, at, spot location.Location) {
	t.Helper()
	if at.X < spot.X-20 || at.X > spot.X+20 || at.Y < spot.Y-20 || at.Y > spot.Y+20 || at.Z != spot.Z {
		t.Fatalf("stands at %+v, want within 20 of %+v", at, spot)
	}
}
