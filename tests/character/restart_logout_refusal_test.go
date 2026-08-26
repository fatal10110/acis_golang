package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func encodeUseItemForEnchant(objectID int32, ctrl bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	return w.Bytes()
}

func enterWorld(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
}

func assertStaticSystemMessage(t *testing.T, frame []byte, messageID int) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage (%#x)", frame[0], serverpackets.OpcodeSystemMessage)
	}
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != int32(messageID) {
		t.Fatalf("system message id = %d, want %d", got, messageID)
	}
}

// TestEnchantSelectionBlocksRestartAndLogout pins that a selected enchant
// scroll refuses both restart (RestartResponse(false), no message) and
// logout (ActionFailed) while leaving the session and world state intact.
func TestEnchantSelectionBlocksRestartAndLogout(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, 955, 1)

	enterWorld(t, c)

	c.Send(encodeUseItemForEnchant(scroll, false))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSelectItemToEnchant)
	if reply := c.Read(); reply[0] != serverpackets.OpcodeChooseInventoryItem {
		t.Fatalf("opcode = %#x, want ChooseInventoryItem (%#x)", reply[0], serverpackets.OpcodeChooseInventoryItem)
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeRestartResponse {
		t.Fatalf("opcode = %#x, want RestartResponse (%#x)", reply[0], serverpackets.OpcodeRestartResponse)
	} else if ok := wire.NewReader(reply[1:]).ReadInt32(); ok != 0 {
		t.Fatalf("RestartResponse result = %d, want 0", ok)
	}
	if _, ok := srv.State.Player(objID); !ok {
		t.Fatal("player left the world despite refused restart")
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
	if _, ok := srv.State.Player(objID); !ok {
		t.Fatal("player left the world despite refused logout")
	}
}

// TestNoRestartZoneBlocksRestartAndLogout pins the zone refusal: inside a
// NO_RESTART zone restart answers NO_RESTART_HERE + RestartResponse(false)
// and logout answers NO_LOGOUT_HERE + ActionFailed.
func TestNoRestartZoneBlocksRestartAndLogout(t *testing.T) {
	form, err := zone.NewCuboid(-100000, 100000, -100000, 100000, -100000, 100000)
	if err != nil {
		t.Fatalf("build zone form: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewNoRestart(1, form))

	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithZones(zones),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)

	enterWorld(t, c)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageNoRestartHere)
	if reply := c.Read(); reply[0] != serverpackets.OpcodeRestartResponse {
		t.Fatalf("opcode = %#x, want RestartResponse (%#x)", reply[0], serverpackets.OpcodeRestartResponse)
	} else if ok := wire.NewReader(reply[1:]).ReadInt32(); ok != 0 {
		t.Fatalf("RestartResponse result = %d, want 0", ok)
	}
	if _, ok := srv.State.Player(objID); !ok {
		t.Fatal("player left the world despite refused restart")
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageNoLogoutHere)
	if reply := c.Read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
	if _, ok := srv.State.Player(objID); !ok {
		t.Fatal("player left the world despite refused logout")
	}
}
