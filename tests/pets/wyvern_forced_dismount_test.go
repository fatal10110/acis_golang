package pets

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// System message ids a rider over ground closed to wyverns sees.
const (
	msgEnteredCombatZone = 283
	msgLeftCombatZone    = 284
	msgNoWyvernArea      = 1687
)

// wyvernGraceDelay is how long a wyvern rider may stay over ground closed
// to wyverns before it is taken off its mount.
const wyvernGraceDelay = 5 * time.Second

// systemMessageIDs returns the ids of the system messages among frames, in
// order.
func systemMessageIDs(t *testing.T, frames [][]byte) []int {
	t.Helper()
	var ids []int
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			ids = append(ids, systemMessageID(t, f))
		}
	}
	return ids
}

// requireStillRiding fails if frames show a dismount or the rider is off
// its wyvern.
func (h *petWorld) requireStillRiding(t *testing.T, what string, frames [][]byte) {
	t.Helper()
	if slices.ContainsFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeRide }) {
		t.Fatalf("%s = %x, want no Ride", what, frameOpcodes(frames))
	}
	if !h.rider(t).Mounted() {
		t.Fatalf("%s took the rider off its wyvern", what)
	}
}

// flyTo flies the rider to (x, y, z) and waits for it to land there; it
// returns what the rider saw on the way.
func (h *petWorld) flyTo(t *testing.T, x, y, z int) [][]byte {
	t.Helper()
	h.client.Send(encodeMoveBackwardToLocation(int32(x), int32(y), int32(z)))
	h.handled(t)
	for range 50 {
		if ox, _, _ := h.srv.PlayerPosition(t, h.ownerID); ox == x {
			return drainFrames(t, h.client)
		}
		h.srv.Advance(t, 100*time.Millisecond)
	}
	t.Fatalf("rider did not reach x=%d", x)
	return nil
}

// bootWyvernOverNoLanding boots a wyvern rider standing 100 units west of a
// no-landing zone and returns it with its position.
func bootWyvernOverNoLanding(t *testing.T) (h *petWorld, x, y, z int) {
	t.Helper()
	// The fixture spawns its character at (10, 20, 30).
	form, err := zone.NewCuboid(110, 10_000, -10_000, 10_000, -10_000, 10_000)
	if err != nil {
		t.Fatalf("build zone form: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewNoLanding(1, form))
	h, _ = bootWyvernRiderOpts(t, []gameservertest.Option{gameservertest.WithZones(zones)})
	drainFrames(t, h.client)
	x, y, z = h.srv.PlayerPosition(t, h.ownerID)
	if x >= 110 {
		t.Fatalf("rider spawned at x=%d, inside the no-landing zone", x)
	}
	return h, x, y, z
}

// TestWyvernDismountedOverNoLandingZone pins Player.enterOnNoLandingZone: a
// wyvern rider flying into a no-landing zone is told the area is closed to
// wyverns (1687) and is taken off its mount 5 seconds later.
func TestWyvernDismountedOverNoLandingZone(t *testing.T) {
	t.Parallel()
	h, x, y, z := bootWyvernOverNoLanding(t)
	entered := h.flyTo(t, x+200, y, z)
	if got := systemMessageIDs(t, entered); !slices.Contains(got, msgNoWyvernArea) {
		t.Fatalf("entering the no-landing zone: system messages %v, want %d", got, msgNoWyvernArea)
	}
	// The zone was entered on the last moves of a 200-unit flight, well
	// under a second before it landed.
	h.srv.Advance(t, wyvernGraceDelay-time.Second)
	h.requireStillRiding(t, "before the grace delay", drainFrames(t, h.client))
	h.srv.Advance(t, 1500*time.Millisecond)
	h.requireDismounted(t, "after the grace delay", drainFrames(t, h.client))
}

// TestWyvernLeavingNoLandingZoneKeepsFlying pins
// Player.exitOnNoLandingZone: a rider that leaves the zone before the
// grace delay keeps its wyvern.
func TestWyvernLeavingNoLandingZoneKeepsFlying(t *testing.T) {
	t.Parallel()
	h, x, y, z := bootWyvernOverNoLanding(t)
	if got := systemMessageIDs(t, h.flyTo(t, x+200, y, z)); !slices.Contains(got, msgNoWyvernArea) {
		t.Fatalf("entering the no-landing zone: system messages %v, want %d", got, msgNoWyvernArea)
	}
	h.flyTo(t, x, y, z)
	h.srv.Advance(t, 2*wyvernGraceDelay)
	h.requireStillRiding(t, "after leaving the zone", drainFrames(t, h.client))
}

// bootWyvernOnBattlefield boots a wyvern rider standing on a battlefield
// whose siege has not started.
func bootWyvernOnBattlefield(t *testing.T) (*petWorld, *zone.Siege) {
	t.Helper()
	form, err := zone.NewCuboid(-10_000, 10_000, -10_000, 10_000, -10_000, 10_000)
	if err != nil {
		t.Fatalf("build zone form: %v", err)
	}
	zones := zone.NewIndex()
	field := zone.NewSiege(1, form)
	zones.Add(field)
	h, _ := bootWyvernRiderOpts(t, []gameservertest.Option{gameservertest.WithZones(zones)})
	drainFrames(t, h.client)
	return h, field
}

// mountWyvern puts the rider back on its wyvern through its collar.
func (h *petWorld) mountWyvern(t *testing.T) {
	t.Helper()
	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeRide, "mount Ride")
	readUntilOpcode(t, h.client, serverpackets.OpcodeUserInfo, "mounted UserInfo")
	h.handled(t)
	drainFrames(t, h.client)
}

// TestWyvernDismountedWhenSiegeStarts pins SiegeZone.setActive(true): a
// wyvern rider on the battlefield as its siege starts hears it entered a
// combat zone, then that the area is closed to wyverns, and is taken off
// its mount 5 seconds later.
func TestWyvernDismountedWhenSiegeStarts(t *testing.T) {
	t.Parallel()
	h, field := bootWyvernOnBattlefield(t)
	field.SetActive(true)
	h.handled(t)
	if got, want := systemMessageIDs(t, drainFrames(t, h.client)), []int{msgEnteredCombatZone, msgNoWyvernArea}; !slices.Equal(got, want) {
		t.Fatalf("siege start: system messages %v, want %v", got, want)
	}
	// Reading the client lets the driven clock run on a little, so the
	// delay is checked a second either side.
	h.srv.Advance(t, wyvernGraceDelay-time.Second)
	h.requireStillRiding(t, "before the grace delay", drainFrames(t, h.client))
	h.srv.Advance(t, 1500*time.Millisecond)
	h.requireDismounted(t, "after the grace delay", drainFrames(t, h.client))
}

// TestWyvernKeepsFlyingWhenSiegeEnds pins SiegeZone.setActive(false): a
// rider still on the battlefield as the siege ends hears it left the
// combat zone and keeps its wyvern.
func TestWyvernKeepsFlyingWhenSiegeEnds(t *testing.T) {
	t.Parallel()
	h, field := bootWyvernOnBattlefield(t)
	field.SetActive(true)
	h.handled(t)
	drainFrames(t, h.client)
	h.srv.Advance(t, 2*time.Second)
	field.SetActive(false)
	h.handled(t)
	if got, want := systemMessageIDs(t, drainFrames(t, h.client)), []int{msgLeftCombatZone}; !slices.Equal(got, want) {
		t.Fatalf("siege end: system messages %v, want %v", got, want)
	}
	h.srv.Advance(t, 2*wyvernGraceDelay)
	h.requireStillRiding(t, "after the siege ended", drainFrames(t, h.client))
}

// TestWyvernDismountedAgainOnNextSiege: a rider forced down once, who
// mounts again while the battlefield is quiet, is forced down again when
// the next siege starts. The countdown is spent once it has run, so the
// first forced dismount does not leave the rider immune to the next.
func TestWyvernDismountedAgainOnNextSiege(t *testing.T) {
	t.Parallel()
	h, field := bootWyvernOnBattlefield(t)
	field.SetActive(true)
	h.handled(t)
	drainFrames(t, h.client)
	h.srv.Advance(t, wyvernGraceDelay+100*time.Millisecond)
	h.requireDismounted(t, "first siege", drainFrames(t, h.client))
	field.SetActive(false)
	h.handled(t)
	drainFrames(t, h.client)

	h.mountWyvern(t)
	field.SetActive(true)
	h.handled(t)
	if got := systemMessageIDs(t, drainFrames(t, h.client)); !slices.Contains(got, msgNoWyvernArea) {
		t.Fatalf("second siege start: system messages %v, want %d", got, msgNoWyvernArea)
	}
	h.srv.Advance(t, wyvernGraceDelay+100*time.Millisecond)
	h.requireDismounted(t, "second siege", drainFrames(t, h.client))
}

// TestWyvernBanishedAsSiegeStartsKeepsFlying pins Siege.startSiege's order
// (banishForeigners, then SiegeZone.setActive(true)): a foreign wyvern
// rider on the battlefield as its siege starts is teleported out before the
// battlefield turns on, so it hears neither the combat notice nor that the
// area is closed to wyverns, and no countdown takes it off its mount.
func TestWyvernBanishedAsSiegeStartsKeepsFlying(t *testing.T) {
	t.Parallel()
	h, field := bootWyvernOnBattlefield(t)
	// The clanless rider is foreign to every owning clan. Its queue is
	// parked so the siege turns on before the teleport out runs, as it does
	// when the siege steps faster than the rider's queue drains.
	const ownerClanID = 1
	release := make(chan struct{})
	if !h.srv.PlayerQueue(t, h.ownerID).Post(func() { <-release }) {
		t.Fatal("rider queue closed")
	}
	field.BanishForeigners(ownerClanID)
	field.SetActive(true)
	close(release)
	h.handled(t)
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeTeleportToLocation, "banish teleport")
	h.srv.Advance(t, 2*wyvernGraceDelay)
	frames = append(frames, drainFrames(t, h.client)...)
	if got := systemMessageIDs(t, frames); slices.Contains(got, msgEnteredCombatZone) || slices.Contains(got, msgNoWyvernArea) {
		t.Fatalf("banished as the siege started: system messages %v, want neither %d nor %d", got, msgEnteredCombatZone, msgNoWyvernArea)
	}
	h.requireStillRiding(t, "banished as the siege started", frames)
}
