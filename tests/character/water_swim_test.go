package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func encodeValidatePosition(at location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeValidatePosition)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(0) // heading
	w.WriteInt32(0) // boat id
	return w.Bytes()
}

// validatePositionReplies sends one ValidatePosition report and returns
// every frame it produced, fenced by an ItemList barrier.
func validatePositionReplies(t *testing.T, c *testsupport.ScriptedClient, reported location.Location) [][]byte {
	t.Helper()
	c.Send(encodeValidatePosition(reported))
	return testsupport.SyncBarrierFrames(t, c, func() { c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList)) }, serverpackets.OpcodeItemList)
}

// assertValidateLocation requires frames to be exactly one ValidateLocation
// sending the server-held position back.
func assertValidateLocation(t *testing.T, frames [][]byte, objID int32, want location.Location) {
	t.Helper()
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeValidateLocation {
		t.Fatalf("replies = %x, want one ValidateLocation", frames)
	}
	r := wire.NewReader(frames[0][1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("ValidateLocation object id = %d, want %d", got, objID)
	}
	got := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	if got != want {
		t.Fatalf("ValidateLocation position = %+v, want the server position %+v", got, want)
	}
}

func waterZones(t *testing.T, minX, maxX int) *zone.Index {
	t.Helper()
	form, err := zone.NewCuboid(minX, maxX, -1_000, 1_000, -1_000, 150)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, form))
	return zones
}

// TestValidatePositionGroundMeasuresDriftIn2D pins ValidatePosition.java:76
// for MoveType.GROUND: the desync is distance2D, so a report off only in
// height is accepted silently, while the same offset along X is corrected.
func TestValidatePositionGroundMeasuresDriftIn2D(t *testing.T) {
	srv, _, objID := bootInZones(t, zone.NewIndex())
	x, y, z := srv.PlayerPosition(t, objID)
	server := location.Location{X: x, Y: y, Z: z}

	if frames := validatePositionReplies(t, srv.Client, location.Location{X: x, Y: y, Z: z + 1_000}); len(frames) != 0 {
		t.Fatalf("ground height drift replies = %x, want none", frames)
	}
	assertValidateLocation(t, validatePositionReplies(t, srv.Client, location.Location{X: x + 1_000, Y: y, Z: z}), objID, server)
}

// TestValidatePositionSwimmingMeasuresDriftIn3D pins ValidatePosition.java:76
// for MoveType.SWIM (WaterZone.onEnter adds it): the desync is distance3D,
// so a report off only in height beyond the swim speed is corrected.
func TestValidatePositionSwimmingMeasuresDriftIn3D(t *testing.T) {
	srv, character, objID := bootInZones(t, waterZones(t, -1_000, 1_000))
	x, y, z := srv.PlayerPosition(t, objID)
	hp := character.HP()

	assertValidateLocation(t, validatePositionReplies(t, srv.Client, location.Location{X: x, Y: y, Z: z - 1_000}), objID, location.Location{X: x, Y: y, Z: z})
	if got := character.HP(); got != hp {
		t.Fatalf("swimming height report HP = %v, want %v", got, hp)
	}
}

// TestValidatePositionFlyingMeasuresDriftIn3D pins ValidatePosition.java:76
// for MoveType.FLY (a flying mount adds it, Player.java:4904): the desync
// is distance3D.
func TestValidatePositionFlyingMeasuresDriftIn3D(t *testing.T) {
	srv, character, objID := bootInZones(t, zone.NewIndex())
	x, y, z := srv.PlayerPosition(t, objID)
	hp := character.HP()
	srv.SetPlayerFlying(t, objID, true)

	assertValidateLocation(t, validatePositionReplies(t, srv.Client, location.Location{X: x, Y: y, Z: z - 1_000}), objID, location.Location{X: x, Y: y, Z: z})
	if got := character.HP(); got != hp {
		t.Fatalf("flying height report HP = %v, want %v", got, hp)
	}
}

// teleportIntoWater teleports the player from its dry spawn to (x, y, z) and
// returns every frame that follows Appearing.
func teleportIntoWater(t *testing.T, srv *gameservertest.Server, character *player.Character, x, y, z int) [][]byte {
	t.Helper()
	character.TeleportTo(x, y, z, 0)
	readUntilQuiet(srv.Client)
	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	return readUntilQuiet(srv.Client)
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

// TestWaterZoneSharedByTwoServersSwimsEachServersPlayer: a water zone is
// shared data, so building a second game server over the same zone index
// must not take over the first server's swim reaction. The first server's
// player entering the water still gets its entry UserInfo and breath gauge.
func TestWaterZoneSharedByTwoServersSwimsEachServersPlayer(t *testing.T) {
	zones := waterZones(t, 2_000, 4_000)
	clock := &waterClock{}
	clock.nanos.Store(time.Now().UnixNano())
	srv, character, objID := bootInZones(t, zones, gameservertest.WithWater(clock.now))
	t.Run("second server over the same zones", func(t *testing.T) {
		bootInZones(t, zones, gameservertest.WithWater(clock.now))
	})

	_, y, z := srv.PlayerPosition(t, objID)
	frames := teleportIntoWater(t, srv, character, 3_000, y, min(z, 100))
	if got := countOpcode(frames, serverpackets.OpcodeUserInfo); got != 2 {
		t.Fatalf("Appearing in water sent %d UserInfo, want the water-entry one and the Appearing answer", got)
	}
	gauge := firstOpcode(frames, serverpackets.OpcodeSetupGauge)
	if gauge < 0 || gaugeTime(t, frames[gauge]) <= 0 {
		t.Fatal("no breath gauge on entering the water")
	}
}

// TestWaterEntryWithoutAllowWaterStillSwims pins WaterZone.onEnter
// (WaterZone.java:23-40): the entry UserInfo broadcast is unconditional;
// only the breath countdown in Player.revalidateZone (Player.java:847)
// depends on AllowWater.
func TestWaterEntryWithoutAllowWaterStillSwims(t *testing.T) {
	clock := &waterClock{}
	clock.nanos.Store(time.Now().UnixNano())
	srv, character, objID := bootInZones(t, waterZones(t, 2_000, 4_000), gameservertest.WithWater(clock.now), gameservertest.WithAllowWater(false))

	_, y, z := srv.PlayerPosition(t, objID)
	frames := teleportIntoWater(t, srv, character, 3_000, y, min(z, 100))
	if got := countOpcode(frames, serverpackets.OpcodeUserInfo); got != 2 {
		t.Fatalf("Appearing in water sent %d UserInfo, want the water-entry one and the Appearing answer", got)
	}
	if i := firstOpcode(frames, serverpackets.OpcodeSetupGauge); i >= 0 {
		t.Fatalf("breath gauge sent at frame %d with AllowWater off, want none", i)
	}
}
