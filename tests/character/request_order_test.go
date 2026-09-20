package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestInWorldRequestsAnswerInSendOrder pipelines requests handled on the
// player's queue (SkillList, ItemList) between requests the connection
// answers itself (ManorList) without reading in between, then logs out. Every
// reply must come back in send order, ending with LeaveWorld: the connection
// hands each in-world frame to the player's queue and waits for it before
// reading the next one.
func TestInWorldRequestsAnswerInSendOrder(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	drainQuiet(t, c)

	const rounds = 5
	for range rounds {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestSkillList))
		c.Send(encodeRequestManorList())
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}
	c.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))

	for round := range rounds {
		if reply := c.Read(); reply[0] != serverpackets.OpcodeSkillList {
			t.Fatalf("round %d reply 1 opcode = %#x, want SkillList (%#x)", round, reply[0], serverpackets.OpcodeSkillList)
		}
		reply := c.Read()
		if reply[0] != serverpackets.OpcodeExtended || wire.NewReader(reply[1:]).ReadUint16() != serverpackets.OpcodeExSendManorList {
			t.Fatalf("round %d reply 2 = % x, want ExSendManorList", round, reply[:min(len(reply), 3)])
		}
		if reply := c.Read(); reply[0] != serverpackets.OpcodeItemList {
			t.Fatalf("round %d reply 3 opcode = %#x, want ItemList (%#x)", round, reply[0], serverpackets.OpcodeItemList)
		}
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeLeaveWorld {
		t.Fatalf("logout reply opcode = %#x, want LeaveWorld (%#x)", reply[0], serverpackets.OpcodeLeaveWorld)
	}
}
