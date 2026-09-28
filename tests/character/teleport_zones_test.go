package character

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// countedPeace is a peace zone over the given box that counts its enters
// and exits.
type countedPeace struct {
	*zone.Peace
	enters, exits atomic.Int32
}

func newCountedPeace(t *testing.T, minX, maxX, minY, maxY int) *countedPeace {
	t.Helper()
	form, err := zone.NewCuboid(minX, maxX, minY, maxY, -10_000, 10_000)
	if err != nil {
		t.Fatalf("build zone form: %v", err)
	}
	z := &countedPeace{Peace: zone.NewPeace(1, form)}
	z.OnEnter(func(zone.Actor) { z.enters.Add(1) })
	z.OnExit(func(zone.Actor) { z.exits.Add(1) })
	return z
}

func (z *countedPeace) assert(t *testing.T, when string, enters, exits int32, inside bool) {
	t.Helper()
	if got := z.enters.Load(); got != enters {
		t.Fatalf("%s: zone enters = %d, want %d", when, got, enters)
	}
	if got := z.exits.Load(); got != exits {
		t.Fatalf("%s: zone exits = %d, want %d", when, got, exits)
	}
	if got := len(z.Occupants()) == 1; got != inside {
		t.Fatalf("%s: player inside the zone = %v, want %v", when, got, inside)
	}
}

func bootInZones(t *testing.T, zones *zone.Index, opts ...gameservertest.Option) (*gameservertest.Server, *player.Character, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
	}, opts...)...)
	objID := srv.SoleObjectID(t)
	// A water zone at the spawn point adds its entry UserInfo and breath
	// gauge to the login burst, so drain it instead of matching it.
	srv.Client.Send(encodeRequestGameStart(0))
	srv.Client.Read() // SSQInfo
	srv.Client.Read() // CharSelected
	srv.Client.Send(encodeEnterWorld())
	drainQuiet(t, srv.Client)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	character, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	return srv, character, objID
}

// appear sends Appearing and returns every frame up to and including its
// UserInfo answer.
func appear(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	var frames [][]byte
	for range 100 {
		f := c.Read()
		frames = append(frames, f)
		if f[0] == serverpackets.OpcodeUserInfo {
			return frames
		}
	}
	t.Fatal("no UserInfo answer to Appearing")
	return nil
}

func readUntilQuiet(c *testsupport.ScriptedClient) [][]byte {
	var frames [][]byte
	for f := c.ReadWithTimeout(rejectSilenceWindow); f != nil; f = c.ReadWithTimeout(rejectSilenceWindow) {
		frames = append(frames, f)
	}
	return frames
}

func firstOpcode(frames [][]byte, op byte) int {
	for i, f := range frames {
		if f[0] == op {
			return i
		}
	}
	return -1
}

// TestTeleportInsidePeaceZoneExitsUntilAppearing pins the zone half of a
// player teleport. Creature.teleportTo takes the player off the grid
// (setRegion(null), Creature.java:415), and Creature.setRegion removes it
// from every zone of its old region (WorldRegion.removeFromZones). While off
// the grid, revalidateZone returns early (Creature.java:1313-1316), so a
// position update enters nothing. Appearing (Player.onTeleported →
// Creature.onTeleported, Creature.java:221-227) sets the region again and
// revalidates the zones. A teleport that stays inside one peace zone
// therefore exits it at the teleport and enters it again at Appearing, and
// the peace flag is down in between.
func TestTeleportInsidePeaceZoneExitsUntilAppearing(t *testing.T) {
	peace := newCountedPeace(t, -10_000, 10_000, -10_000, 10_000)
	zones := zone.NewIndex()
	zones.Add(peace)
	srv, character, objID := bootInZones(t, zones)
	if !character.InPeaceZone() {
		t.Fatal("player not in the peace zone after entering the world")
	}
	peace.assert(t, "after EnterWorld", 1, 0, true)

	x, y, z := srv.PlayerPosition(t, objID)
	character.TeleportTo(x+300, y, z, 0)
	if character.InPeaceZone() {
		t.Fatal("peace flag still set after the teleport, before Appearing")
	}
	peace.assert(t, "after the teleport", 1, 1, false)

	// A position update before Appearing does not put the player back in a
	// zone.
	character.SyncPosition(location.Location{X: x + 300, Y: y, Z: z})
	if character.InPeaceZone() {
		t.Fatal("a position update before Appearing set the peace flag")
	}
	peace.assert(t, "after a position update before Appearing", 1, 1, false)

	readUntilQuiet(srv.Client)
	appear(t, srv.Client)
	if !character.InPeaceZone() {
		t.Fatal("peace flag not restored at Appearing")
	}
	peace.assert(t, "after Appearing", 2, 1, true)
}

// TestTeleportIntoPeaceZoneEntersAtAppearing is the arrival side: the
// destination's zones are entered when the client appears, not when the
// teleport is issued.
func TestTeleportIntoPeaceZoneEntersAtAppearing(t *testing.T) {
	peace := newCountedPeace(t, 5_000, 7_000, -10_000, 10_000)
	zones := zone.NewIndex()
	zones.Add(peace)
	srv, character, objID := bootInZones(t, zones)
	peace.assert(t, "after EnterWorld", 0, 0, false)

	_, y, z := srv.PlayerPosition(t, objID)
	character.TeleportTo(6_000, y, z, 0)
	if character.InPeaceZone() {
		t.Fatal("destination peace zone entered before Appearing")
	}
	peace.assert(t, "after the teleport", 0, 0, false)

	readUntilQuiet(srv.Client)
	appear(t, srv.Client)
	if !character.InPeaceZone() {
		t.Fatal("destination peace zone not entered at Appearing")
	}
	peace.assert(t, "after Appearing", 1, 0, true)
}

// waterClock is the drowning tracker's clock, advanced by hand.
type waterClock struct{ nanos atomic.Int64 }

func (c *waterClock) now() time.Time          { return time.Unix(0, c.nanos.Load()) }
func (c *waterClock) advance(d time.Duration) { c.nanos.Add(int64(d)) }

func bootInWater(t *testing.T) (*gameservertest.Server, *player.Character, int32, *waterClock) {
	t.Helper()
	form, err := zone.NewCuboid(-1_000, 1_000, -1_000, 1_000, -1_000, 150)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, form))
	clock := &waterClock{}
	clock.nanos.Store(time.Now().UnixNano())
	srv, character, objID := bootInZones(t, zones, gameservertest.WithWater(clock.now))
	return srv, character, objID, clock
}

func gaugeTime(t *testing.T, frame []byte) int32 {
	t.Helper()
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // color
	return r.ReadInt32()
}

func drownIndex(frames [][]byte) int {
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && wire.NewReader(f[1:]).ReadInt32() == int32(serverpackets.SystemMessageDrownDamage) {
			return i
		}
	}
	return -1
}

// TestTeleportOutOfWaterStopsBreathAtTeleport pins where the breath
// countdown stops for a teleport out of the water. Creature.teleportTo →
// setRegion(null) (Creature.java:415) → Creature.setRegion
// (Creature.java:1773-1790) runs WaterZone.onExit, which drops the swim
// move type and broadcasts UserInfo (WaterZone.java:44-49), then clears the
// known list, then calls revalidateZone(true). Player.revalidateZone
// (Player.java:843-852) goes on to the water block after the base method
// returns early off the grid: isInWater() is now false, so
// WaterTaskManager.remove sends SetupGauge(0) at the teleport. A player
// already drowning takes no drowning damage while off the grid, and
// Appearing at a dry destination sends no gauge.
func TestTeleportOutOfWaterStopsBreathAtTeleport(t *testing.T) {
	srv, character, objID, clock := bootInWater(t)
	x, y, z := srv.PlayerPosition(t, objID)

	// Past the breath limit: every tick drowns.
	clock.advance(time.Hour)
	srv.Water.Tick()
	if drownIndex(readUntilQuiet(srv.Client)) < 0 {
		t.Fatal("no drowning damage past the breath limit")
	}

	character.TeleportTo(x+5_000, y, z, 0)
	frames := readUntilQuiet(srv.Client)
	user := firstOpcode(frames, serverpackets.OpcodeUserInfo)
	gauge := firstOpcode(frames, serverpackets.OpcodeSetupGauge)
	if user < 0 || gauge < user {
		t.Fatalf("teleport frames: UserInfo at %d, SetupGauge at %d, want the water-exit UserInfo then SetupGauge", user, gauge)
	}
	if got := gaugeTime(t, frames[gauge]); got != 0 {
		t.Fatalf("SetupGauge time at the teleport = %d, want 0", got)
	}

	// Off the grid, before Appearing: no drowning.
	srv.Water.Tick()
	if i := drownIndex(readUntilQuiet(srv.Client)); i >= 0 {
		t.Fatal("drowning damage while off the grid after a teleport out of the water")
	}

	frames = appear(t, srv.Client)
	if i := firstOpcode(frames, serverpackets.OpcodeSetupGauge); i >= 0 {
		t.Fatalf("breath gauge sent at Appearing (frame %d), want none at a dry destination", i)
	}
}

// TestTeleportWithinWaterRestartsBreath: the teleport's water exit stops the
// countdown (SetupGauge 0). At Appearing, Creature.onTeleported →
// setRegion(region) → revalidateZone(true) enters the water again
// (WaterZone.onEnter broadcasts UserInfo), and WaterTaskManager.add finds
// the player untracked, so a fresh countdown starts with a full gauge
// before Appearing's own UserInfo answer.
func TestTeleportWithinWaterRestartsBreath(t *testing.T) {
	srv, character, objID, _ := bootInWater(t)
	x, y, z := srv.PlayerPosition(t, objID)

	character.TeleportTo(x+300, y, z, 0)
	frames := readUntilQuiet(srv.Client)
	gauge := firstOpcode(frames, serverpackets.OpcodeSetupGauge)
	if gauge < 0 || gaugeTime(t, frames[gauge]) != 0 {
		t.Fatalf("teleport sent no SetupGauge(0) among %d frames", len(frames))
	}

	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	frames = readUntilQuiet(srv.Client)
	enter := firstOpcode(frames, serverpackets.OpcodeUserInfo)
	gauge = firstOpcode(frames, serverpackets.OpcodeSetupGauge)
	if enter < 0 || gauge < enter {
		t.Fatalf("Appearing frames: UserInfo at %d, SetupGauge at %d, want the water-enter UserInfo then SetupGauge", enter, gauge)
	}
	if got := gaugeTime(t, frames[gauge]); got <= 0 {
		t.Fatalf("SetupGauge time at Appearing = %d, want a fresh full breath", got)
	}
	answered := false
	for _, f := range frames[gauge+1:] {
		answered = answered || f[0] == serverpackets.OpcodeUserInfo
	}
	if !answered {
		t.Fatal("no UserInfo answer to Appearing after the fresh breath gauge")
	}
}
