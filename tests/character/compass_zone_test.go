package character

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func compassCodes(t *testing.T, frames [][]byte) []byte {
	t.Helper()
	var codes []byte
	for _, frame := range frames {
		if len(frame) < 3 || frame[0] != 0xfe || frame[1] != 0x32 || frame[2] != 0 {
			continue
		}
		if len(frame) != 7 || !bytes.Equal(frame[4:], []byte{0, 0, 0}) {
			t.Fatalf("compass frame = %x, want FE 32 00 code 00 00 00", frame)
		}
		codes = append(codes, frame[3])
	}
	return codes
}

func assertCompassCodes(t *testing.T, frames [][]byte, want ...byte) {
	t.Helper()
	got := compassCodes(t, frames)
	if !bytes.Equal(got, want) {
		t.Fatalf("compass codes = %x, want %x", got, want)
	}
}

func TestCompassZoneOnEnterWorldAndWalk(t *testing.T) {
	form, err := zone.NewCuboid(-100, 100, -100, 100, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewPeace(1, form))
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
	)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	burst := readEnterWorldBurst(t, c)
	assertCompassCodes(t, burst, 0x0c) // PEACEZONE
	drainQuiet(t, c)

	objID := srv.SoleObjectID(t)
	spawn := location.Location{X: 10, Y: 20, Z: 30}
	target := location.Location{X: 300, Y: 20, Z: 30}
	c.Send(encodeMoveBackwardToLocation(target, spawn, 1))
	if frame := c.Read(); frame[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation", frame[0])
	}
	waitForWorldPosition(t, srv, objID, target)
	assertCompassCodes(t, readUntilQuiet(c), 0x0f) // GENERALZONE

	next := location.Location{X: 500, Y: 20, Z: 30}
	c.Send(encodeMoveBackwardToLocation(next, target, 1))
	if frame := c.Read(); frame[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("second walk opcode = %#x, want MoveToLocation", frame[0])
	}
	waitForWorldPosition(t, srv, objID, next)
	assertCompassCodes(t, readUntilQuiet(c))
}

func TestCompassZoneTownToTownTeleport(t *testing.T) {
	peace := newCountedPeace(t, -10_000, 10_000, -10_000, 10_000)
	zones := zone.NewIndex()
	zones.Add(peace)
	srv, character, objID := bootInZones(t, zones)
	x, y, z := srv.PlayerPosition(t, objID)
	character.TeleportTo(x+300, y, z, 0)
	assertCompassCodes(t, readUntilQuiet(srv.Client), 0x0f) // off-grid GENERALZONE
	assertCompassCodes(t, appear(t, srv.Client), 0x0c)      // destination PEACEZONE
}

func TestCompassZoneTeleportFollowsKnownListClear(t *testing.T) {
	zones := zone.NewIndex()
	zones.Add(newCountedPeace(t, -10_000, 10_000, -10_000, 10_000))
	srv, c, observer, objID := bootObserverPair(t, gameservertest.WithZones(zones))
	srv.SpawnHostileNPCAt(t, location.Location{X: 40, Y: 20, Z: 30})
	drainQuiet(t, c)
	drainQuiet(t, observer)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	character, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatal("player has no live character")
	}
	x, y, z := srv.PlayerPosition(t, objID)
	character.TeleportTo(x+300, y, z, 0)
	frames := readUntilQuiet(c)
	deleted := firstOpcode(frames, serverpackets.OpcodeDeleteObject)
	compass := -1
	for i, frame := range frames {
		if len(frame) >= 3 && bytes.Equal(frame[:3], []byte{0xfe, 0x32, 0}) {
			compass = i
			break
		}
	}
	if deleted < 0 || compass <= deleted {
		t.Fatalf("teleport DeleteObject index = %d, compass index = %d; want known-list clear before compass", deleted, compass)
	}
}

func TestCompassZoneWalkIntoPvPArena(t *testing.T) {
	form, err := zone.NewCuboid(100, 500, -100, 100, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewArena(1, form))
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
	)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read()
	c.Read()
	c.Send(encodeEnterWorld())
	assertCompassCodes(t, readEnterWorldBurst(t, c), 0x0f)
	drainQuiet(t, c)

	spawn := location.Location{X: 10, Y: 20, Z: 30}
	target := location.Location{X: 300, Y: 20, Z: 30}
	c.Send(encodeMoveBackwardToLocation(target, spawn, 1))
	if frame := c.Read(); frame[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation", frame[0])
	}
	waitForWorldPosition(t, srv, srv.SoleObjectID(t), target)
	assertCompassCodes(t, readUntilQuiet(c), 0x0e)
}

func TestCompassZonePvPAndSiegePriority(t *testing.T) {
	form, err := zone.NewCuboid(-100, 100, -100, 100, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewPeace(1, form))
	zones.Add(zone.NewArena(2, form))
	siege, err := zone.NewSiege(3, form, commons.NewStatSet())
	if err != nil {
		t.Fatal(err)
	}
	siege.SetActive(true)
	zones.Add(siege)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
	)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read()
	c.Read()
	c.Send(encodeEnterWorld())
	assertCompassCodes(t, readEnterWorldBurst(t, c), 0x0b)
	drainQuiet(t, c)
	objID := srv.SoleObjectID(t)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	character, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatal("player has no live character")
	}
	if !character.InSiegeZone() {
		t.Fatal("player not in the active siege zone after EnterWorld")
	}
	x, y, z := srv.PlayerPosition(t, objID)
	// Siege wins over both PvP and peace. Leaving for general ground starts
	// the normal PvP flag window before sending the compass update.
	character.TeleportTo(x+300, y, z, 0)
	frames := readUntilQuiet(c)
	assertCompassCodes(t, frames, 0x0f)
	userInfo := firstOpcode(frames, serverpackets.OpcodeUserInfo)
	compass := -1
	for i, frame := range frames {
		if len(frame) >= 3 && frame[0] == 0xfe && frame[1] == 0x32 && frame[2] == 0 {
			compass = i
			break
		}
	}
	if userInfo < 0 || compass <= userInfo {
		t.Fatalf("siege exit UserInfo index = %d, compass index = %d; want PvP update before compass", userInfo, compass)
	}
	if character.PvPFlagState() == 0 {
		t.Fatal("leaving a siege zone did not flag the player for PvP")
	}
}
