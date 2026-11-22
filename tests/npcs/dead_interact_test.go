package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestDeadPlayerDialogTeleportRefused pins #4996: a gatekeeper teleport
// link on a page left open at death is refused at the interaction gate
// with the bypass's closing ActionFailed alone. Nothing is paid, kept or
// persisted, and the corpse does not move.
func TestDeadPlayerDialogTeleportRefused(t *testing.T) {
	t.Parallel()
	w, gk := bootGatekeeperWorld(t, 1500, 0, false, newTeleportClock())
	w.openAnyNpcPage(t)
	assertAnswer(t, w.tripFrames(t, npcCommand(gk, "teleport_request")), pageAnswer, gk, teleportWindow(gk, tripPrice, 0))
	w.srv.MarkPlayerDead(t, w.player)
	drainFrames(t, w.c)

	frames := w.tripFrames(t, npcCommand(gk, "teleport 0"))
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("dead teleport = %x, want ActionFailed alone", opcodes(frames))
	}
	if got := w.character(t).CurrentLocation(); got != w.at {
		t.Fatalf("dead player lies at %+v, want %+v", got, w.at)
	}
	if got := w.held(t, item.AdenaID); got != 1500 {
		t.Fatalf("adena held = %d, want 1500", got)
	}
	if got := w.savedCount(t, item.AdenaID); got != 1500 {
		t.Fatalf("adena saved = %d, want 1500", got)
	}
}

// TestDeadPlayerTrainerRequestsRefused pins #4996 on the trainer requests
// made at the NPC the player last selected: from a corpse, every skill
// learn and enchant request, and their info requests, is refused with
// ActionFailed alone.
func TestDeadPlayerTrainerRequestsRefused(t *testing.T) {
	t.Parallel()
	w, gk := bootGatekeeperWorld(t, 0, 0, false, newTeleportClock())
	w.selectFolk(t, gk)
	w.srv.MarkPlayerDead(t, w.player)
	drainFrames(t, w.c)

	for name, packet := range map[string][]byte{
		"RequestAcquireSkillInfo":   encodeTrainerRequest(clientpackets.OpcodeRequestAcquireSkillInfo, 0, 294, 1, 0),
		"RequestAcquireSkill":       encodeTrainerRequest(clientpackets.OpcodeRequestAcquireSkill, 0, 294, 1, 0),
		"RequestExEnchantSkillInfo": encodeTrainerRequest(clientpackets.OpcodeExtended, clientpackets.OpcodeRequestExEnchantSkillInfo, 294, 101),
		"RequestExEnchantSkill":     encodeTrainerRequest(clientpackets.OpcodeExtended, clientpackets.OpcodeRequestExEnchantSkill, 294, 101),
	} {
		w.c.Send(packet)
		if frames := drainFrames(t, w.c); len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("dead %s = %x, want ActionFailed alone", name, opcodes(frames))
		}
	}
}

// encodeTrainerRequest writes a trainer request: opcode, the extended
// sub-opcode when sub is not 0, then the int32 fields.
func encodeTrainerRequest(opcode byte, sub uint16, fields ...int32) []byte {
	w := wire.NewPacketWriter(opcode)
	if sub != 0 {
		w.WriteUint16(sub)
	}
	for _, f := range fields {
		w.WriteInt32(f)
	}
	return w.Bytes()
}
