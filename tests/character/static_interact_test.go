package character

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Static object interact pins StaticObject.onAction (StaticObject.java:
// 37-44) → PlayableAI.tryToInteract → PlayerAI.thinkInteract
// (PlayerAI.java:413-461): out of 100 + the player's collision radius
// (3D), the player walks to the object (PlayerMove.maybeMoveToPawn,
// PlayerMove.java:335-352, MoveToPawn at offset 100), unless shift is held;
// in range and within Npc.INTERACTION_DISTANCE (150), it faces the object
// with MoveToPawn(actor, target, 150) before StaticObject.onInteract
// (StaticObject.java:24-35) answers. The approach ends on the object's
// point: only a Creature pawn stops the walk at its offset
// (CreatureMove.isOnLastPawnMoveGeoPath, CreatureMove.java:468-471).

// bootBodied boots the one character with the datapack human fighter body
// (radius 9 male), which the shared class template leaves at zero: the
// approach range is then 100 + 9, the static object adding no radius.
func bootBodied(t *testing.T) *gameservertest.Server {
	t.Helper()
	tmpl := gameservertest.ClassTemplate()
	tmpl.CollisionRadius, tmpl.CollisionHeight = 9, 23
	tmpl.CollisionRadiusFemale, tmpl.CollisionHeightFemale = 8, 23.5
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(tmpl))
	enterWorld(t, srv.Client)
	drainQuiet(t, srv.Client)
	return srv
}

// spawnTownMap spawns a town map dx units east of spawnOrigin and reads its
// StaticObjectInfo.
func spawnTownMap(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, dx int) *staticobject.Object {
	t.Helper()
	at := location.Location{X: spawnOrigin.X + dx, Y: spawnOrigin.Y, Z: spawnOrigin.Z}
	obj, err := staticobject.NewObject(srv.NewObjectID(), &staticobject.Template{
		ID: 24180018, Location: at, Type: staticobject.MapType, Texture: "testmap", MapX: 17, MapY: 21,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.State.Spawn(obj, at.X, at.Y, at.Z, 0)
	mustReadOpcode(t, c, serverpackets.OpcodeStaticObjectInfo, "town map StaticObjectInfo")
	drainQuiet(t, c)
	return obj
}

// clickStatic sends a click on obj and returns every frame it answered
// with, in wire order.
func clickStatic(t *testing.T, c *testsupport.ScriptedClient, obj *staticobject.Object, shift bool) [][]byte {
	t.Helper()
	c.Send(encodeAction(obj.ObjectID(), int32(spawnOrigin.X), int32(spawnOrigin.Y), int32(spawnOrigin.Z), shift))
	return syncFrames(t, c)
}

func syncFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	return testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList)
}

// staticInteractOrder keeps the frames a static interact answers with.
func staticInteractOrder(frames [][]byte) []byte {
	var order []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeShowTownMap,
			serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeStopMove, serverpackets.OpcodeChairSit,
			serverpackets.OpcodeChangeWaitType:
			order = append(order, f[0])
		}
	}
	return order
}

func frameWithOpcode(t *testing.T, frames [][]byte, opcode byte) []byte {
	t.Helper()
	for _, f := range frames {
		if f[0] == opcode {
			return f
		}
	}
	t.Fatalf("no opcode %#x in %x", opcode, frames)
	return nil
}

func assertMoveToPawn(t *testing.T, frame []byte, mover, target int32, distance int32) {
	t.Helper()
	r := wire.NewReader(frame[1:])
	gotMover, gotTarget, gotDistance := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if gotMover != mover || gotTarget != target || gotDistance != distance {
		t.Fatalf("MoveToPawn = %d->%d at %d, want %d->%d at %d", gotMover, gotTarget, gotDistance, mover, target, distance)
	}
}

func assertTownMap(t *testing.T, frame []byte) {
	t.Helper()
	r := wire.NewReader(frame[1:])
	if texture, x, y := r.ReadString(), r.ReadInt32(), r.ReadInt32(); texture != "town_map.testmap" || x != 17 || y != 21 {
		t.Fatalf("ShowTownMap = %q (%d,%d), want town_map.testmap (17,21)", texture, x, y)
	}
}

func selectStatic(t *testing.T, c *testsupport.ScriptedClient, obj *staticobject.Object) {
	t.Helper()
	c.Send(encodeAction(obj.ObjectID(), int32(spawnOrigin.X), int32(spawnOrigin.Y), int32(spawnOrigin.Z), false))
	mustReadOpcode(t, c, serverpackets.OpcodeMyTargetSelected, "select static object")
	drainQuiet(t, c)
}

func TestTownMapClickInRangeFacesThenShowsMap(t *testing.T) {
	t.Parallel()
	srv := bootBodied(t)
	c := srv.Client
	mapObject := spawnTownMap(t, srv, c, 0)
	selectStatic(t, c, mapObject)

	frames := clickStatic(t, c, mapObject, false)
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeShowTownMap}
	if got := staticInteractOrder(frames); string(got) != string(want) {
		t.Fatalf("in-range town map click = %x, want %x", got, want)
	}
	assertMoveToPawn(t, frameWithOpcode(t, frames, serverpackets.OpcodeMoveToPawn), srv.SoleObjectID(t), mapObject.ObjectID(), 150)
	assertTownMap(t, frameWithOpcode(t, frames, serverpackets.OpcodeShowTownMap))
}

func TestTownMapClickFromAfarWalksThenShowsMap(t *testing.T) {
	t.Parallel()
	srv := bootBodied(t)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	mapObject := spawnTownMap(t, srv, c, 300)
	selectStatic(t, c, mapObject)

	frames := clickStatic(t, c, mapObject, false)
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn}
	if got := staticInteractOrder(frames); string(got) != string(want) {
		t.Fatalf("town map click from 300 = %x, want %x", got, want)
	}
	assertMoveToPawn(t, frameWithOpcode(t, frames, serverpackets.OpcodeMoveToPawn), objID, mapObject.ObjectID(), 100)

	mover := srv.PlayerMove(t, objID)
	srv.AdvanceUntil(t, "town map approach arrival", func() bool { return !mover.Moving() })
	frames = syncFrames(t, c)
	want = []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeShowTownMap}
	if got := staticInteractOrder(frames); string(got) != string(want) {
		t.Fatalf("town map arrival = %x, want %x", got, want)
	}
	assertMoveToPawn(t, frameWithOpcode(t, frames, serverpackets.OpcodeMoveToPawn), objID, mapObject.ObjectID(), 150)
	assertTownMap(t, frameWithOpcode(t, frames, serverpackets.OpcodeShowTownMap))
	// A static object is no creature: the approach walks onto its point
	// rather than stopping 100 short of it (isOnLastPawnMoveGeoPath).
	at := mover.Position()
	if x, y, _ := mapObject.Position(); at.X != x || at.Y != y {
		t.Fatalf("arrived at %v, want the town map's point (%d,%d)", at, x, y)
	}
}

func TestTownMapShiftClickFromAfarDoesNotWalk(t *testing.T) {
	t.Parallel()
	srv := bootBodied(t)
	c := srv.Client
	mapObject := spawnTownMap(t, srv, c, 300)
	selectStatic(t, c, mapObject)

	frames := clickStatic(t, c, mapObject, true)
	if got := staticInteractOrder(frames); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("shift town map click from 300 = %x, want ActionFailed alone", got)
	}
	if srv.PlayerMove(t, srv.SoleObjectID(t)).Moving() {
		t.Fatal("a shift-click from afar walked")
	}
}

// TestTownMapApproachRangeCountsNoStaticRadius pins the approach boundary:
// a static object has no collision radius, so a click 108 units away
// interacts in place and one 109 units away walks.
func TestTownMapApproachRangeCountsNoStaticRadius(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		dx   int
		want []byte
	}{
		{108, []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeShowTownMap}},
		{109, []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn}},
	} {
		t.Run(fmt.Sprint(tc.dx), func(t *testing.T) {
			srv := bootBodied(t)
			c := srv.Client
			mapObject := spawnTownMap(t, srv, c, tc.dx)
			selectStatic(t, c, mapObject)
			frames := clickStatic(t, c, mapObject, false)
			if got := staticInteractOrder(frames); string(got) != string(tc.want) {
				t.Fatalf("town map click from %d = %x, want %x", tc.dx, got, tc.want)
			}
			distance := int32(150)
			if tc.dx == 109 {
				distance = 100
			}
			assertMoveToPawn(t, frameWithOpcode(t, frames, serverpackets.OpcodeMoveToPawn), srv.SoleObjectID(t), mapObject.ObjectID(), distance)
		})
	}
}

// TestArenaSignClickInRangeShowsSignboard pins the arena sign's interact:
// facing MoveToPawn, then the signboard page tagged with the sign's id.
func TestArenaSignClickInRangeShowsSignboard(t *testing.T) {
	t.Parallel()
	srv := bootBodied(t)
	c := srv.Client
	sign, err := staticobject.NewObject(srv.NewObjectID(), &staticobject.Template{
		ID: 24180019, Location: spawnOrigin, Type: staticobject.ArenaSignType,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.State.Spawn(sign, spawnOrigin.X, spawnOrigin.Y, spawnOrigin.Z, 0)
	mustReadOpcode(t, c, serverpackets.OpcodeStaticObjectInfo, "arena sign StaticObjectInfo")
	drainQuiet(t, c)
	selectStatic(t, c, sign)

	frames := clickStatic(t, c, sign, false)
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeNpcHtmlMessage}
	if got := staticInteractOrder(frames); string(got) != string(want) {
		t.Fatalf("arena sign click = %x, want %x", got, want)
	}
	assertMoveToPawn(t, frameWithOpcode(t, frames, serverpackets.OpcodeMoveToPawn), srv.SoleObjectID(t), sign.ObjectID(), 150)
	r := wire.NewReader(frameWithOpcode(t, frames, serverpackets.OpcodeNpcHtmlMessage)[1:])
	if id := r.ReadInt32(); id != sign.ObjectID() {
		t.Fatalf("signboard NpcHtmlMessage object id = %d, want %d", id, sign.ObjectID())
	}
}
