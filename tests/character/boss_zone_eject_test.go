package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func bossEjectZone(t *testing.T, oust location.Location, minX int) (*zone.Index, *zone.Boss) {
	t.Helper()
	form, err := zone.NewCuboid(minX, 1000, -100, 100, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	set := commons.NewStatSet()
	set.Set("InvadeTime", "600000")
	set.Set("oustX", oust.X)
	set.Set("oustY", oust.Y)
	set.Set("oustZ", oust.Z)
	boss, err := zone.NewBoss(1, form, set)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(boss)
	return zones, boss
}

func assertEjectFrame(t *testing.T, frames [][]byte, objID int32, xMin, xMax int) {
	t.Helper()
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeTeleportToLocation {
			continue
		}
		r := wire.NewReader(frame[1:])
		id := r.ReadInt32()
		x := int(r.ReadInt32())
		if id == objID && x >= xMin && x <= xMax {
			return
		}
	}
	t.Fatalf("no ejection TeleportToLocation for %d at x=%d..%d", objID, xMin, xMax)
}

func TestBossZoneWalkInEjectsToOustLocation(t *testing.T) {
	t.Parallel()
	oust := location.Location{X: -500, Y: 20, Z: 30}
	zones, _ := bossEjectZone(t, oust, 100)
	srv, _, objID := bootInZones(t, zones)
	spawn := location.Location{X: 10, Y: 20, Z: 30}
	srv.Client.Send(encodeMoveBackwardToLocation(location.Location{X: 300, Y: 20, Z: 30}, spawn, 1))
	if frame := srv.Client.Read(); frame[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation", frame[0])
	}
	waitForWorldPosition(t, srv, objID, oust)
	assertEjectFrame(t, readUntilQuiet(srv.Client), objID, oust.X, oust.X)
}

func TestBossZoneLoginEjectsAfterExpiredPermission(t *testing.T) {
	t.Parallel()
	oust := location.Location{X: -5000, Y: 20, Z: 30}
	zones, boss := bossEjectZone(t, oust, -100)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
	)
	objID := srv.SoleObjectID(t)
	boss.AllowEntry(objID, -time.Second)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	frames := append(readEnterWorldBurst(t, c), readUntilQuiet(c)...)
	waitForWorldPosition(t, srv, objID, oust)
	assertEjectFrame(t, frames, objID, oust.X, oust.X)
}

func TestBossZoneTeleportRejoinEjectsToTown(t *testing.T) {
	t.Parallel()
	// One zero coordinate selects the town fallback in BossZone.onEnter.
	zones, boss := bossEjectZone(t, location.Location{X: 0, Y: 900, Z: 30}, 100)
	area, err := restart.NewArea([]location.Point{{X: 0, Y: -200}, {X: 1100, Y: -200}, {X: 1100, Y: 200}, {X: 0, Y: 200}}, -10_000, 10_000, map[player.Race]string{player.RaceHuman: "town"})
	if err != nil {
		t.Fatal(err)
	}
	town := &restart.Table{Areas: []restart.Area{area}, Points: []restart.Point{{Name: "town", Points: []location.Location{{X: -500, Y: 20, Z: 30}}}}}
	srv, character, objID := bootInZones(t, zones, gameservertest.WithRestartPoints(town))
	boss.AllowEntry(objID, time.Minute)
	character.TeleportTo(300, 20, 30, 0)
	readUntilQuiet(srv.Client)
	appear(t, srv.Client)
	if len(boss.Occupants()) != 1 {
		t.Fatal("permitted player did not enter the boss zone")
	}

	// Teleporting inside the zone exits it, revokes the used permission,
	// then re-enters at Appearing. The ejection must run after that handler
	// releases both teleportMu and the zone actor's mu.
	character.TeleportTo(400, 20, 30, 0)
	readUntilQuiet(srv.Client)
	appear(t, srv.Client)
	srv.AdvanceUntil(t, "boss-zone town ejection", func() bool {
		x, _, _ := srv.PlayerPosition(t, objID)
		return x >= -520 && x <= -480
	})
	assertEjectFrame(t, readUntilQuiet(srv.Client), objID, -520, -480)
}
