package admin

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// reloadZoneLair is the boss zone id both zone sets of the reload tests
// share.
const reloadZoneLair = 110_000

func reloadZoneCuboid(t *testing.T, x1, x2, y1, y2 int) zone.Form {
	t.Helper()
	form, err := zone.NewCuboid(x1, x2, y1, y2, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	return form
}

// occupantIDs returns the object ids k holds, sorted.
func occupantIDs(k zone.Kind) []int32 {
	var ids []int32
	for _, a := range k.Core().Occupants() {
		ids = append(ids, a.ObjectID())
	}
	slices.Sort(ids)
	return ids
}

// compassFrame reports whether frame is ExSetCompassZoneCode with code.
func compassFrame(frame []byte, code int32) bool {
	return bytes.Equal(frame, []byte{0xfe, 0x32, 0, byte(code), 0, 0, 0})
}

// exchangeZones sends payload, when there is one, and returns every frame
// it drew, read up to the manor-list barrier sent after it: a compass zone
// is an extended packet too, which exchange would take for the barrier.
func exchangeZones(t *testing.T, c *testsupport.ScriptedClient, payload []byte) [][]byte {
	t.Helper()
	if payload != nil {
		c.Send(payload)
	}
	c.Send(encodeManorBarrier())
	var frames [][]byte
	for range 2000 {
		frame := c.Read()
		if len(frame) >= 3 && frame[0] == serverpackets.OpcodeExtended && frame[1] == 0x1b && frame[2] == 0 {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("no manor-list barrier within 2000 frames")
	return nil
}

// encodeWalk is MoveBackwardToLocation from from to to.
func encodeWalk(to, from location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	for _, v := range []int{to.X, to.Y, to.Z, from.X, from.Y, from.Z} {
		w.WriteInt32(int32(v))
	}
	w.WriteInt32(1)
	return w.Bytes()
}

// TestAdminReloadZone pins //reload zone (ZoneManager.reload): the boss
// zones' entry permissions are saved to grandboss_list and the reloaded
// boss zones get them back; every creature leaves the zones it stood in,
// without their exit rules, and enters the reloaded ones around it, the
// game master seeing its new compass zone before the reload's answer.
func TestAdminReloadZone(t *testing.T) {
	t.Parallel()
	before := zone.NewIndex()
	oldPeace := zone.NewPeace(1, reloadZoneCuboid(t, -100, 100, -100, 100))
	oldLair := zone.NewBoss(reloadZoneLair, reloadZoneCuboid(t, 5000, 6000, 5000, 6000))
	oldLair.AllowEntry(777, 0)
	before.Add(oldPeace)
	before.Add(oldLair)

	after := zone.NewIndex()
	arena := zone.NewArena(2, reloadZoneCuboid(t, -100, 100, -100, 100))
	lair := zone.NewBoss(reloadZoneLair, reloadZoneCuboid(t, 5000, 6000, 5000, 6000))
	after.Add(arena)
	after.Add(lair)

	again := zone.NewIndex()
	peace := zone.NewPeace(1, reloadZoneCuboid(t, -100, 100, -100, 100))
	again.Add(peace)
	reads := []*zone.Index{after, again}
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithZones(before),
		gameservertest.WithDataReloads(network.DataReloads{Zones: func() (*zone.Index, error) {
			next := reads[0]
			reads = reads[1:]
			return next, nil
		}}),
	)
	gm := srv.Client
	enterWorld(t, gm)
	wolf := srv.SpawnHostileNPCAt(t, location.Location{X: 40, Y: 20, Z: 30})
	drain(t, gm)
	if got := occupantIDs(oldPeace); !slices.Equal(got, []int32{gmID, wolf.ObjectID()}) || !wolf.InPeaceZone() {
		t.Fatalf("before the reload the peace zone holds %v (wolf in peace: %v), want the game master and the wolf", got, wolf.InPeaceZone())
	}

	frames := exchangeZones(t, gm, encodeBuildCmd("reload zone"))
	if len(frames) != 2 || !compassFrame(frames[0], serverpackets.CompassPvPZone) {
		t.Fatalf("//reload zone frames = %x, want the PvP compass zone then the answer", testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, frames[1:], "Zones have been reloaded.")

	if got := occupantIDs(oldPeace); len(got) != 0 {
		t.Fatalf("the replaced peace zone still holds %v", got)
	}
	if got := occupantIDs(arena); !slices.Equal(got, []int32{gmID, wolf.ObjectID()}) {
		t.Fatalf("the reloaded arena holds %v, want the game master and the wolf", got)
	}
	if wolf.InPeaceZone() || !wolf.InsideZone(zone.FlagPvP) {
		t.Fatalf("wolf after the reload: peace %v, pvp %v; want only the arena's pvp", wolf.InPeaceZone(), wolf.InsideZone(zone.FlagPvP))
	}
	if got := lair.AllowedPlayers(); !slices.Equal(got, []int32{777}) {
		t.Fatalf("reloaded lair allows %v, want the saved 777", got)
	}
	var player, zoneID int
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT player_id, zone FROM grandboss_list").Scan(&player, &zoneID); err != nil || player != 777 || zoneID != reloadZoneLair {
		t.Fatalf("grandboss_list = (%d, %d) err %v, want (777, %d)", player, zoneID, err, reloadZoneLair)
	}

	// Zones read anew once more put both back in peace.
	frames = exchangeZones(t, gm, encodeBuildCmd("reload zone"))
	if len(frames) != 2 || !compassFrame(frames[0], serverpackets.CompassPeaceZone) {
		t.Fatalf("second //reload zone frames = %x, want the peace compass zone then the answer", testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, frames[1:], "Zones have been reloaded.")
	if got := occupantIDs(arena); len(got) != 0 || !wolf.InPeaceZone() || wolf.InsideZone(zone.FlagPvP) {
		t.Fatalf("after the second reload the arena holds %v, wolf peace %v pvp %v", got, wolf.InPeaceZone(), wolf.InsideZone(zone.FlagPvP))
	}
	if got := occupantIDs(peace); !slices.Equal(got, []int32{gmID, wolf.ObjectID()}) {
		t.Fatalf("the peace zone read last holds %v, want the game master and the wolf", got)
	}
}

// TestAdminReloadZoneRaceFree reloads the zones over and over while a
// player walks across a peace zone's border: under -race nothing races,
// and once the walk ends the player stands in the last zones read alone,
// its peace state matching where it stopped.
func TestAdminReloadZoneRaceFree(t *testing.T) {
	t.Parallel()
	var (
		mu   sync.Mutex
		sets []*zone.Peace
	)
	read := func() *zone.Index {
		ix := zone.NewIndex()
		p := zone.NewPeace(1, reloadZoneCuboid(t, -100, 100, -100, 100))
		ix.Add(p)
		mu.Lock()
		sets = append(sets, p)
		mu.Unlock()
		return ix
	}
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithZones(read()),
		gameservertest.WithDataReloads(network.DataReloads{Zones: func() (*zone.Index, error) { return read(), nil }}),
	)
	gm := srv.Client
	enterWorld(t, gm)
	walker, walkerID := addPlayer(t, srv, "walker", "Walker", 0)
	drain(t, gm)

	const reloads, maxReloads = 8, 24
	for _, leg := range []struct {
		from, to location.Location
		inPeace  bool
	}{
		{location.Location{X: spawnX, Y: spawnY, Z: spawnZ}, location.Location{X: 400, Y: spawnY, Z: spawnZ}, false},
		{location.Location{X: 400, Y: spawnY, Z: spawnZ}, location.Location{X: spawnX, Y: spawnY, Z: spawnZ}, true},
	} {
		walker.Send(encodeWalk(leg.to, leg.from))
		// A burst of reloads races the first steps, the game master's link
		// running them while the clock drives the walk; one more goes out
		// between each advance of the clock, so reloads keep landing while
		// the walk crosses the border.
		for range reloads {
			gm.Send(encodeBuildCmd("reload zone"))
		}
		sent := reloads
		srv.AdvanceUntil(t, "walk arrival", func() bool {
			x, y, z := srv.PlayerPosition(t, walkerID)
			if x == leg.to.X && y == leg.to.Y && z == leg.to.Z {
				return true
			}
			if sent < maxReloads {
				gm.Send(encodeBuildCmd("reload zone"))
				sent++
			}
			return false
		})
		frames := exchangeZones(t, gm, nil)
		if got := len(textsIn(t, frames)); got != sent {
			t.Fatalf("reload answers = %d, want %d", got, sent)
		}
		drain(t, walker)

		mu.Lock()
		last := sets[len(sets)-1]
		for _, p := range sets[:len(sets)-1] {
			if got := occupantIDs(p); len(got) != 0 {
				t.Fatalf("a replaced peace zone still holds %v", got)
			}
		}
		mu.Unlock()
		character := onlineCharacter(t, srv, walkerID)
		if inside := slices.Contains(occupantIDs(last), walkerID); inside != leg.inPeace || character.InPeaceZone() != leg.inPeace {
			t.Fatalf("walker at %+v: in the last peace zone %v, peace flag %v; want %v", leg.to, inside, character.InPeaceZone(), leg.inPeace)
		}
	}
}
