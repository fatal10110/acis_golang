package trade

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestActionRefusedWhileHoldingRequest pins Action.runImpl
// (Action.java:39-43): a player holding a request it has not answered has
// every plain click refused with ActionFailed alone, before the clicked
// object is resolved, so nothing is selected. The requester holds no
// request of its own and clicks as usual, and so does the target once it
// has answered.
func TestActionRefusedWhileHoldingRequest(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	h.enterAll(t)
	monster := h.srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)
	h.sendRequest(t)

	if got := clickOpcodes(t, h.second, monster.ObjectID()); !slices.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("click while holding the request answered %#x, want ActionFailed alone", got)
	}
	if got := clickOpcodes(t, h.first, monster.ObjectID()); !slices.Contains(got, serverpackets.OpcodeMyTargetSelected) {
		t.Fatalf("requester's click answered %#x, want the monster selected", got)
	}

	h.second.Send(encodeAnswerTradeRequest(0))
	syncTrader(t, h.second)
	drainUntilQuiet(t, h.first)
	if got := clickOpcodes(t, h.second, monster.ObjectID()); !slices.Contains(got, serverpackets.OpcodeMyTargetSelected) || slices.Contains(got, serverpackets.OpcodeActionFailed) {
		t.Fatalf("click after answering answered %#x, want the monster selected", got)
	}
}

// clickOpcodes sends a plain click on objectID and returns the opcodes of
// every frame answering it.
func clickOpcodes(t *testing.T, c *testsupport.ScriptedClient, objectID int32) []byte {
	t.Helper()
	frames := testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(encodeAction(objectID))
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList)
	got := make([]byte, 0, len(frames))
	for _, frame := range frames {
		got = append(got, frame[0])
	}
	return got
}

// syncTrader waits until the server has handled everything c sent and
// drops what it answered.
func syncTrader(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList)
}
