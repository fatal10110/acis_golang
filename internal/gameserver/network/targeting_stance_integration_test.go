//go:build integration

package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestGameClientLinkActionBarStanceCommandsToggleStance covers the
// action-bar sit/stand and walk/run buttons, which arrive as action-use
// requests rather than the dedicated wait/move-type packets, and the
// release path for an action-bar command no handler claims yet: the client
// must get ActionFailed back, never silence.
func TestGameClientLinkActionBarStanceCommandsToggleStance(t *testing.T) {
	c, chars, _, _, _, _ := newLinkedSQLGameClient(t, nil, nil, 0)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := sqlCharacterID(t, chars)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	// Walk/run button: a fresh character runs, so the first press walks and
	// the second runs again.
	c.Send(encodeRequestActionUse(1, false, false))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeChangeMoveType {
		t.Fatalf("walk/run toggle opcode = %#x, want ChangeMoveType (%#x)", reply[0], serverpackets.OpcodeChangeMoveType)
	}
	r := wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("ChangeMoveType object id = %d, want %d", got, objID)
	}
	if running := r.ReadInt32(); running != 0 {
		t.Fatalf("ChangeMoveType running = %d, want 0 after first toggle", running)
	}
	c.Send(encodeRequestActionUse(1, false, false))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeChangeMoveType {
		t.Fatalf("run toggle opcode = %#x, want ChangeMoveType (%#x)", reply[0], serverpackets.OpcodeChangeMoveType)
	}
	r = wire.NewReader(reply[1:])
	r.ReadInt32()
	if running := r.ReadInt32(); running != 1 {
		t.Fatalf("ChangeMoveType running = %d, want 1 after second toggle", running)
	}

	// Sit/stand button: a fresh character stands, so the first press sits
	// and the second stands back up.
	c.Send(encodeRequestActionUse(0, false, false))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeChangeWaitType {
		t.Fatalf("sit toggle opcode = %#x, want ChangeWaitType (%#x)", reply[0], serverpackets.OpcodeChangeWaitType)
	}
	r = wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("ChangeWaitType object id = %d, want %d", got, objID)
	}
	if waitType := r.ReadInt32(); waitType != int32(serverpackets.WaitSitting) {
		t.Fatalf("ChangeWaitType type = %d, want sitting", waitType)
	}
	c.Send(encodeRequestActionUse(0, false, false))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeChangeWaitType {
		t.Fatalf("stand toggle opcode = %#x, want ChangeWaitType (%#x)", reply[0], serverpackets.OpcodeChangeWaitType)
	}
	r = wire.NewReader(reply[1:])
	r.ReadInt32()
	if waitType := r.ReadInt32(); waitType != int32(serverpackets.WaitStanding) {
		t.Fatalf("ChangeWaitType type = %d, want standing", waitType)
	}

	// An action-bar command nothing claims (private store sell) must
	// release the client with ActionFailed instead of silence.
	c.Send(encodeRequestActionUse(10, false, false))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("unclaimed action opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
}

func TestGameClientLinkStanceAndSocialPacketsInGame(t *testing.T) {
	c, chars, _, _, _, _ := newLinkedSQLGameClient(t, nil, nil, 0)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := sqlCharacterID(t, chars)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.Send(encodeRequestChangeMoveType(false))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeChangeMoveType {
		t.Fatalf("walk opcode = %#x, want ChangeMoveType (%#x)", reply[0], serverpackets.OpcodeChangeMoveType)
	}
	r := wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("ChangeMoveType object id = %d, want %d", got, objID)
	}
	if running, swimming := r.ReadInt32(), r.ReadInt32(); running != 0 || swimming != 0 {
		t.Fatalf("ChangeMoveType flags = (%d,%d), want (0,0)", running, swimming)
	}

	c.Send(encodeRequestChangeMoveType(true))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeChangeMoveType {
		t.Fatalf("run opcode = %#x, want ChangeMoveType (%#x)", reply[0], serverpackets.OpcodeChangeMoveType)
	}
	r = wire.NewReader(reply[1:])
	r.ReadInt32()
	if running := r.ReadInt32(); running != 1 {
		t.Fatalf("ChangeMoveType running = %d, want 1", running)
	}

	c.Send(encodeRequestChangeWaitType(false))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeChangeWaitType {
		t.Fatalf("sit opcode = %#x, want ChangeWaitType (%#x)", reply[0], serverpackets.OpcodeChangeWaitType)
	}
	r = wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("ChangeWaitType object id = %d, want %d", got, objID)
	}
	if waitType := r.ReadInt32(); waitType != int32(serverpackets.WaitSitting) {
		t.Fatalf("ChangeWaitType type = %d, want sitting", waitType)
	}

	c.Send(encodeRequestChangeWaitType(true))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeChangeWaitType {
		t.Fatalf("stand opcode = %#x, want ChangeWaitType (%#x)", reply[0], serverpackets.OpcodeChangeWaitType)
	}
	r = wire.NewReader(reply[1:])
	r.ReadInt32()
	if waitType := r.ReadInt32(); waitType != int32(serverpackets.WaitStanding) {
		t.Fatalf("ChangeWaitType type = %d, want standing", waitType)
	}

	c.Send(encodeRequestSocialAction(13))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeSocialAction {
		t.Fatalf("social opcode = %#x, want SocialAction (%#x)", reply[0], serverpackets.OpcodeSocialAction)
	}
	r = wire.NewReader(reply[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("SocialAction object id = %d, want %d", got, objID)
	}
	if actionID := r.ReadInt32(); actionID != 13 {
		t.Fatalf("SocialAction action id = %d, want 13", actionID)
	}
}
