package character

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func encodeFinishRotating(degree, side int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeFinishRotating)
	w.WriteInt32(degree)
	w.WriteInt32(side)
	return w.Bytes()
}

func encodeActionUse(actionID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestActionUse)
	w.WriteInt32(actionID)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

// framesBeforeItemList sends req, then RequestItemList as a barrier, and
// returns every frame the client read before the ItemList.
func framesBeforeItemList(t *testing.T, c *testsupport.ScriptedClient, req []byte) [][]byte {
	t.Helper()
	return testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(req)
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList)
}

// assertOnlyActionFailed fails unless frames is exactly one ActionFailed.
func assertOnlyActionFailed(t *testing.T, frames [][]byte, what string) {
	t.Helper()
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
		ops := make([]byte, len(frames))
		for i, f := range frames {
			ops[i] = f[0]
		}
		t.Fatalf("%s answered opcodes %x, want only ActionFailed (%#x)", what, ops, serverpackets.OpcodeActionFailed)
	}
}

// TestFinishRotatingKeepsServerHeading pins FinishRotating.java:17-25: the
// request only broadcasts StopRotation with the client's degree; the
// server-side heading, which every behind/front check reads, stays where it
// was, so an attacker standing behind the player is still behind it.
func TestFinishRotatingKeepsServerHeading(t *testing.T) {
	t.Parallel()
	for _, degree := range []int32{12345, 2_000_000_000} {
		t.Run(strconv.Itoa(int(degree)), func(t *testing.T) {
			t.Parallel()
			finishRotatingKeepsHeading(t, degree)
		})
	}
}

func finishRotatingKeepsHeading(t *testing.T, degree int32) {
	t.Helper()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	objID := srv.SoleObjectID(t)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	// Facing east (+X): a point to the west is behind the player.
	const facing = 0
	player.Presence.SetHeading(facing)
	x, y, z := player.Position()
	attacker := location.Location{X: x - 100, Y: y, Z: z}

	c.Send(encodeFinishRotating(degree, 0))
	frame := mustReadOpcode(t, c, serverpackets.OpcodeStopRotation, "StopRotation")
	r := wire.NewReader(frame[1:])
	if id, deg, speed := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != objID || deg != degree || speed != 0 {
		t.Fatalf("StopRotation = (%d,%d,%d), want (%d,%d,0)", id, deg, speed, objID, degree)
	}
	if got := player.Heading(); got != facing {
		t.Fatalf("degree %d: Heading() = %d after FinishRotating, want unchanged %d", degree, got, facing)
	}
	self := location.OrientedLocation{Location: location.Location{X: x, Y: y, Z: z}, Heading: player.Heading()}
	if !self.IsBehind(attacker) {
		t.Fatalf("degree %d: attacker at %+v no longer behind the player after FinishRotating", degree, attacker)
	}
}

// TestUnableActionUseOnlyFails pins RequestActionUse.java:84-96: a dead
// player, or a fake-dead one using any action but sit/stand, is answered
// ActionFailed and nothing else, whatever the action; walk/run keeps its
// stance.
func TestUnableActionUseOnlyFails(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	obj, ok := srv.State.Player(srv.SoleObjectID(t))
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}

	// Fake death first: only sit/stand passes the gate.
	if !player.StartFakeDeath() {
		t.Fatal("StartFakeDeath reported no change")
	}
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "fake death")
	running := player.Running()
	for _, action := range []int32{1, 10, 28, 38, 61, 19, 9999} {
		assertOnlyActionFailed(t, framesBeforeItemList(t, c, encodeActionUse(action)), "fake-dead action")
	}
	if player.Running() != running {
		t.Fatal("fake-dead walk/run changed the stance")
	}
	if !player.FakeDead() {
		t.Fatal("a fake-dead action other than sit/stand ended the fake death")
	}
	player.StopFakeDeath()
	srv.AdvanceUntil(t, "fake-death get-up ended", func() bool { return !player.StandingNow() && player.Standing() })
	drainQuiet(t, c)

	if !player.MarkDead() {
		t.Fatal("MarkDead reported no change")
	}
	for _, action := range []int32{0, 1, 10, 28, 38, 61, 19, 9999} {
		assertOnlyActionFailed(t, framesBeforeItemList(t, c, encodeActionUse(action)), "dead action")
	}
	if player.Running() != running {
		t.Fatal("dead walk/run changed the stance")
	}
}

// TestMoveBackwardToLocationRejectsExactly9900Units pins the strict radius
// of MoveBackwardToLocation.java:109-114 (Location.isIn3DRadius uses <): a
// target exactly 9900 units from the packet origin, after the floor-to-head
// Z conversion, is refused; one just inside is walked to.
func TestMoveBackwardToLocationRejectsExactly9900Units(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	obj, ok := srv.State.Player(srv.SoleObjectID(t))
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	collision := int(player.CollisionHeight())
	origin := location.Location{}

	frames := framesBeforeItemList(t, c, encodeMoveBackwardToLocation(location.Location{X: 9900, Z: -collision}, origin, 1))
	assertOnlyActionFailed(t, frames, "walk of exactly 9900 units")

	c.Send(encodeMoveBackwardToLocation(location.Location{X: 9899, Z: -collision}, origin, 1))
	expectGroundClickAck(t, c)
	if reply := c.Read(); reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk just inside 9900 units opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
}

// TestMoveToLocationBroadcastsRequestedZ pins PlayerMove.moveToLocation
// (PlayerMove.java:111-201): the walk broadcasts the requested height (the
// click Z plus the collision height), not the floor under the target; only
// each step lands on the floor (PlayerMove.java:237-238), so the walk still
// ends on it.
func TestMoveToLocationBroadcastsRequestedZ(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithGeo(gameservertest.FlatGeo{Z: gameservertest.SpawnZ}),
	)
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	objID := srv.SoleObjectID(t)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}

	const clickZ = 50
	spawn := location.Location{X: 10, Y: 20, Z: gameservertest.SpawnZ}
	c.Send(encodeMoveBackwardToLocation(location.Location{X: 80, Y: 70, Z: clickZ}, spawn, 1))
	expectGroundClickAck(t, c)
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	r := wire.NewReader(reply[1:])
	r.ReadInt32() // object ID
	dx, dy, dz := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if want := int32(clickZ + int(player.CollisionHeight())); dx != 80 || dy != 70 || dz != want {
		t.Fatalf("MoveToLocation destination = (%d,%d,%d), want (80,70,%d)", dx, dy, dz, want)
	}
	waitForWorldPosition(t, srv, objID, location.Location{X: 80, Y: 70, Z: gameservertest.SpawnZ})
}
