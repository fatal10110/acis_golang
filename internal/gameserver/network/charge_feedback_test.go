package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func assertForceChargeMessage(t *testing.T, frame []byte, messageID int, charges int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("message opcode = %#x, want SystemMessage (%#x)", frame[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != int32(messageID) {
		t.Fatalf("message id = %d, want %d", got, messageID)
	}
	params := r.ReadInt32()
	if messageID == serverpackets.SystemMessageForceIncreasedToS1 {
		if params != 1 || r.ReadInt32() != serverpackets.SystemMessageParamNumber || r.ReadInt32() != charges {
			t.Fatalf("force-increased message params = %d, want one number %d", params, charges)
		}
		return
	}
	if params != 0 {
		t.Fatalf("force-max message params = %d, want 0", params)
	}
}
