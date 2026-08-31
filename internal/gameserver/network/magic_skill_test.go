package network

import (
	"testing"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestTargetCastRejectionsSendMessageBeforeActionFailed(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)

	for _, tt := range []struct {
		name      string
		rejection skilltarget.CastRejection
		message   int
	}{
		{"aura", skilltarget.CastRejectCantAttackPeaceZone, serverpackets.SystemMessageCantAtkPeacezone},
		{"front aura", skilltarget.CastRejectCantAttackPeaceZone, serverpackets.SystemMessageCantAtkPeacezone},
		{"behind aura", skilltarget.CastRejectCantAttackPeaceZone, serverpackets.SystemMessageCantAtkPeacezone},
		{"one invalid", skilltarget.CastRejectInvalidTarget, serverpackets.SystemMessageInvalidTarget},
		{"one target in peace", skilltarget.CastRejectTargetInPeaceZone, serverpackets.SystemMessageTargetInPeacezone},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.ResetCapture(frames)
			sendTargetCastRejection(live, tt.rejection)
			sendMagicActionFailed(live)

			got := frames.Frames()
			if opcodes := testsupport.FrameOpcodes(got); len(opcodes) != 2 || opcodes[0] != serverpackets.OpcodeSystemMessage || opcodes[1] != serverpackets.OpcodeActionFailed {
				t.Fatalf("opcodes = %x, want [SystemMessage, ActionFailed]", opcodes)
			}
			assertStaticSystemMessageFrame(t, got[0], tt.message)
		})
	}
}
