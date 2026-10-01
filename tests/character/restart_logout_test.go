package character

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestRestartReturnsToCharacterSelect walks away from the spawn point,
// restarts back to character selection, and requires the walk destination
// to survive in the characters row so the next selection starts there.
func TestRestartReturnsToCharacterSelect(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1), gameservertest.WithReuseDelays(0, 0))
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	objID := srv.SoleObjectID(t)

	target := location.Location{X: 80, Y: 70, Z: 30}
	spawn := location.Location{X: 10, Y: 20, Z: 30}
	c.Send(encodeMoveBackwardToLocation(target, spawn, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	waitForWorldPosition(t, srv, objID, target)
	walkHeading := spawn.HeadingTo(target)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeRestartResponse {
		t.Fatalf("restart opcode = %#x, want RestartResponse (%#x)", reply[0], serverpackets.OpcodeRestartResponse)
	}
	if ok := wire.NewReader(reply[1:]).ReadInt32(); ok != 1 {
		t.Fatalf("RestartResponse result = %d, want 1", ok)
	}
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("post-restart opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}
	if _, ok := srv.State.Player(objID); ok {
		t.Fatalf("world.Player(%d) still present after restart", objID)
	}
	assertPersistedPosition(t, srv, objID, target, walkHeading)

	c.Send(encodeRequestGameStart(0))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("second select opcode = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
}

// TestLogoutPersistsAndLeavesWorld walks away from the spawn point, logs
// out, and requires the live actor to leave world state while the
// characters row keeps the walked position, heading, and level.
func TestLogoutPersistsAndLeavesWorld(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	objID := srv.SoleObjectID(t)

	target := location.Location{X: 80, Y: 70, Z: 30}
	spawn := location.Location{X: 10, Y: 20, Z: 30}
	c.Send(encodeMoveBackwardToLocation(target, spawn, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	waitForWorldPosition(t, srv, objID, target)
	walkHeading := spawn.HeadingTo(target)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeLeaveWorld {
		t.Fatalf("logout opcode = %#x, want LeaveWorld (%#x)", reply[0], serverpackets.OpcodeLeaveWorld)
	}
	// LeaveWorld is the last frame: detach's cast-stop ack is dropped.
	c.ExpectClosed()

	if _, ok := srv.State.Player(objID); ok {
		t.Fatalf("world.Player(%d) still present after logout", objID)
	}
	ch := persistedCharacter(t, srv, objID)
	if ch.CharLevel != 1 {
		t.Fatalf("persisted level after logout = %d, want 1", ch.CharLevel)
	}
	if ch.Location != target || ch.LastHeading != walkHeading {
		t.Fatalf("persisted position after logout = %+v/%d, want %+v/%d", ch.Location, ch.LastHeading, target, walkHeading)
	}
}

// TestLogoutAtCharacterSelectKeepsConnectionOpen sends Logout before any
// character is in the world. The reference Logout handler returns at once
// when the client has no player: it sends nothing and leaves the socket
// open. The next request must therefore be answered, and its reply must be
// the first frame read, proving Logout itself produced none.
func TestLogoutAtCharacterSelectKeepsConnectionOpen(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t)
	c := srv.Client

	c.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeCharCreateOk {
		t.Fatalf("reply after char-select logout = %#x, want CharCreateOk (%#x)", reply[0], serverpackets.OpcodeCharCreateOk)
	}
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("post-create opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}
}

// TestLogoutAfterRestartKeepsConnectionOpen restarts back to character
// select, where the client again has no player, and requires Logout there
// to be ignored the same way: no frame, and the following character
// selection is still answered.
func TestLogoutAfterRestartKeepsConnectionOpen(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1), gameservertest.WithReuseDelays(0, 0))
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	objID := srv.SoleObjectID(t)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeRestartResponse {
		t.Fatalf("restart opcode = %#x, want RestartResponse (%#x)", reply[0], serverpackets.OpcodeRestartResponse)
	}
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("post-restart opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))

	c.Send(encodeRequestGameStart(0))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("reply after char-select logout = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("select opcode = %#x, want CharSelected (%#x)", reply[0], serverpackets.OpcodeCharSelected)
	}
	// Selected, the character is registered but not spawned until
	// EnterWorld.
	srv.AwaitHandled(t)
	if _, ok := srv.State.Player(objID); !ok {
		t.Fatalf("world.Player(%d) missing after the selection", objID)
	}
	if _, ok := srv.State.Object(objID); ok {
		t.Fatalf("world.Object(%d) spawned before EnterWorld", objID)
	}
}

func assertPersistedPosition(t *testing.T, srv *gameservertest.Server, objID int32, want location.Location, wantHeading int) {
	t.Helper()
	ch := persistedCharacter(t, srv, objID)
	if ch.Location != want || ch.LastHeading != wantHeading {
		t.Fatalf("persisted position = %+v/%d, want %+v/%d", ch.Location, ch.LastHeading, want, wantHeading)
	}
}

func persistedCharacter(t *testing.T, srv *gameservertest.Server, objID int32) *player.Character {
	t.Helper()
	ch, err := srv.Chars.Get(context.Background(), objID)
	if err != nil {
		t.Fatalf("load character: %v", err)
	}
	return ch
}
