package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

func TestGameInboundServesLoginAndCharacterSelectOpcodes(t *testing.T) {
	for _, op := range []byte{
		clientpackets.OpcodeProtocolVersion,
		clientpackets.OpcodeAuthLogin,
		clientpackets.OpcodeRequestNewCharacter,
		clientpackets.OpcodeRequestCharacterCreate,
		clientpackets.OpcodeRequestCharacterDelete,
		clientpackets.OpcodeCharacterRestore,
		clientpackets.OpcodeRequestGameStart,
		clientpackets.OpcodeEnterWorld,
		clientpackets.OpcodeRequestPledgeCrest,
		clientpackets.OpcodeRequestAllyCrest,
	} {
		if _, ok := gameInbound[op]; !ok {
			t.Errorf("gameInbound has no handler for opcode %#x", op)
		}
	}
}

func TestNoPayloadIgnoresTrailingBytes(t *testing.T) {
	for _, payload := range [][]byte{{clientpackets.OpcodeEnterWorld}, {clientpackets.OpcodeEnterWorld, 1, 2, 3}} {
		if _, err := noPayload(payload); err != nil {
			t.Errorf("noPayload(% x) = %v, want nil", payload, err)
		}
	}
}
