package admin

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestAdminReloadZoneAfterAJumpKeepsTheHeldRegion pins //reload zone
// (ZoneManager.reload revalidating each creature against its region) for a
// game master an instant teleport moved into another region: the jump
// leaves it held in the region it left, so the reload enters none of the
// zones at its landing point and sends no compass zone; its first step
// then enters the landing point's peace zone.
func TestAdminReloadZoneAfterAJumpKeepsTheHeldRegion(t *testing.T) {
	t.Parallel()
	landing := location.Location{X: spawnX + 6_000, Y: spawnY, Z: spawnZ}
	peaceAt := func() (*zone.Index, *zone.Peace) {
		ix := zone.NewIndex()
		p := zone.NewPeace(1, reloadZoneCuboid(t, landing.X-500, landing.X+500, landing.Y-1_500, landing.Y+1_500))
		ix.Add(p)
		return ix, p
	}
	before, _ := peaceAt()
	after, peace := peaceAt()
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithZones(before),
		gameservertest.WithDataReloads(network.DataReloads{Zones: func() (*zone.Index, error) { return after, nil }}),
	)
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)

	obj, ok := srv.State.Object(gmID)
	if !ok {
		t.Fatal("the game master is not in the world")
	}
	p := script.PlayerOf(obj.(attackable.Combatant))
	done := make(chan struct{})
	if !srv.PlayerQueue(t, gmID).Post(func() {
		defer close(done)
		p.InstantTeleportTo(script.Loc{X: landing.X, Y: landing.Y, Z: landing.Z})
	}) {
		t.Fatal("the game master's queue is closed")
	}
	<-done
	srv.Settle(t)
	drain(t, gm)
	character := onlineCharacter(t, srv, gmID)
	if character.InPeaceZone() {
		t.Fatal("the jump entered the landing point's peace zone")
	}

	frames := exchangeZones(t, gm, encodeBuildCmd("reload zone"))
	if len(frames) != 1 {
		t.Fatalf("//reload zone after the jump frames = %x, want only the answer", testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, frames, "Zones have been reloaded.")
	if character.InPeaceZone() || len(occupantIDs(peace)) != 0 {
		t.Fatalf("the reload after the jump entered the landing point's peace zone (holds %v)", occupantIDs(peace))
	}

	gm.Send(encodeWalk(location.Location{X: landing.X, Y: landing.Y + 1_000, Z: landing.Z}, landing))
	srv.Settle(t)
	srv.TickPositions()
	frames = exchangeZones(t, gm, nil)
	if !slices.ContainsFunc(frames, func(f []byte) bool { return compassFrame(f, serverpackets.CompassPeaceZone) }) {
		t.Fatalf("first step after the reload frames = %x, want the peace compass zone", testsupport.FrameOpcodes(frames))
	}
	if !character.InPeaceZone() || !slices.Equal(occupantIDs(peace), []int32{gmID}) {
		t.Fatal("the first step after the reload did not enter the landing point's peace zone")
	}
}
