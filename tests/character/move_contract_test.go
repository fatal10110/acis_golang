package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func encodeStartRotating(degree, side int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeStartRotating)
	w.WriteInt32(degree)
	w.WriteInt32(side)
	return w.Bytes()
}

func soleOnlineCharacter(t *testing.T, srv *gameservertest.Server) (*player.Character, int32) {
	t.Helper()
	objID := srv.SoleObjectID(t)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	character, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	return character, objID
}

// TestStartRotatingBroadcastsClientTurn pins StartRotating.runImpl: the
// request only broadcasts StartRotation with the client's degree and side and
// a zero speed to the player and everyone around. The server-side heading
// stays where it was, as with FinishRotating.
func TestStartRotatingBroadcastsClientTurn(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	character, objID := soleOnlineCharacter(t, srv)
	const facing = 0
	character.Presence.SetHeading(facing)

	for _, req := range []struct{ degree, side int32 }{{12345, 1}, {-7, -1}, {2_000_000_000, 0}} {
		c.Send(encodeStartRotating(req.degree, req.side))
		frame := mustReadOpcode(t, c, serverpackets.OpcodeStartRotation, "StartRotation")
		r := wire.NewReader(frame[1:])
		if id, deg, side, speed := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != objID || deg != req.degree || side != req.side || speed != 0 {
			t.Fatalf("StartRotation = (%d,%d,%d,%d), want (%d,%d,%d,0)", id, deg, side, speed, objID, req.degree, req.side)
		}
		if err := r.Err(); err != nil {
			t.Fatalf("StartRotation decode: %v", err)
		}
	}
	if got := character.Heading(); got != facing {
		t.Fatalf("Heading() = %d after StartRotating, want unchanged %d", got, facing)
	}
}

// TestMoveBackwardToLocationZeroSpeedRejected pins the zero-move-speed gate
// of MoveBackwardToLocation.runImpl: a player whose move speed is 0 (here a
// swamp that takes the whole speed) is answered ActionFailed then
// CANT_MOVE_TOO_ENCUMBERED, and no walk starts. The gate sits before the
// 9900-unit cap, so a far target gets the same answer.
func TestMoveBackwardToLocationZeroSpeedRejected(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(2_000, 4_000, -1_000, 1_000, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	swamp := zone.NewSwamp(1, form)
	swamp.MoveBonus = -100
	zones := zone.NewIndex()
	zones.Add(swamp)
	srv, character, objID := bootInZones(t, zones)
	_, y, z := srv.PlayerPosition(t, objID)
	teleportIntoWater(t, srv, character, 3_000, y, z)
	assertLiveMoveSpeed(t, "in the stopping swamp", character, 0)

	here := location.Location{X: 3_000, Y: y, Z: z}
	collision := int(character.CollisionHeight())
	for _, target := range []location.Location{
		{X: 3_100, Y: y, Z: z - collision},
		{X: 3_000 + 9_900, Y: y, Z: z - collision},
	} {
		frames := framesBeforeItemList(t, srv.Client, encodeMoveBackwardToLocation(target, here, 1))
		if len(frames) != 2 || frames[0][0] != serverpackets.OpcodeActionFailed || frames[1][0] != serverpackets.OpcodeSystemMessage {
			ops := make([]byte, len(frames))
			for i, f := range frames {
				ops[i] = f[0]
			}
			t.Fatalf("zero-speed walk to %+v answered opcodes %x, want ActionFailed then SystemMessage", target, ops)
		}
		assertStaticSystemMessage(t, frames[1], serverpackets.SystemMessageCantMoveTooEncumbered)
	}
	if got := character.Move().Moving(); got {
		t.Fatal("a zero-speed walk request started a move")
	}
}

// TestOutOfControlActionUseOnlyFails pins the isOutOfControl branch of
// RequestActionUse.runImpl: a player who is paralyzed or mid-teleport is
// answered ActionFailed and nothing else for any action, sit/stand included.
func TestOutOfControlActionUseOnlyFails(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	character, _ := soleOnlineCharacter(t, srv)

	for _, state := range []struct {
		name  string
		set   func(bool) bool
		unset func(bool) bool
	}{
		{"paralyzed", character.SetParalyzed, character.SetParalyzed},
		{"teleporting", character.SetTeleporting, character.SetTeleporting},
	} {
		if !state.set(true) {
			t.Fatalf("%s: state reported no change", state.name)
		}
		running := character.Running()
		for _, action := range []int32{0, 1, 10, 28, 38, 61, 9999} {
			assertOnlyActionFailed(t, framesBeforeItemList(t, c, encodeActionUse(action)), state.name+" action")
		}
		if character.Running() != running || !character.Standing() {
			t.Fatalf("%s: an action changed the stance", state.name)
		}
		state.unset(false)
	}
}
