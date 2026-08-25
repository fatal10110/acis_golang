package network

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestGameClientLinkNeverGoesSilentOnActionRequests is the guardrail against
// the bug class behind #828/#829/#873: an accepted client action packet that
// a handler quietly drops, leaving the client's pending action unresolved —
// which presented as a character that walks up to a target and freezes, a
// picked-up item that never leaves the ground, or an item-window click that
// does nothing. Every case here sends a request built to be rejected (a
// nonexistent object id, an unclaimed action id, a command with no target to
// act on) and asserts at least one frame comes back. It intentionally does
// not assert which frame — the point is that the handler answered at all,
// not what it said — so this test keeps working (and keeps catching
// regressions) as new rejection reasons and messages are added.
//
// Scope limit: every case here is a *rejected* request, so this guardrail
// cannot catch a flow that answers rejections correctly but leaves the
// client's pending action outstanding when the action succeeds. Flows whose
// success path must also release the click (pickup, interact, follow — see
// docs/agents/action-response-contract.md) assert that release in their own
// success-path tests instead.
//
// RequestAcquireSkillInfo is deliberately absent: the reference returns
// without an alternative packet when no offer matches (#1638), and the
// opcode registers no pending client action, so its silence is documented
// reference parity rather than a silent drop.
func TestGameClientLinkNeverGoesSilentOnActionRequests(t *testing.T) {
	c, chars, _, _ := newLinkedGameClient(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	chars.soleObjectID(t)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	const missingObjectID = 999999

	cases := []struct {
		name    string
		payload []byte
	}{
		{"UseItem on an object the player doesn't hold", encodeUseItem(missingObjectID, false)},
		{"RequestUnEquipItem for an empty body slot", encodeRequestUnEquipItem(0)},
		{"RequestActionUse with an action id no handler claims", encodeRequestActionUse(9999, false, false)},
		{"RequestActionUse pet command with no active summon", encodeRequestActionUse(16, false, false)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c.Send(tc.payload)
			// The first read is the actual assertion: it must prove the
			// server answered at all, so it gets a generous timeout. Any
			// further frames from the same rejection (e.g. a system
			// message followed by ActionFailed) are already in flight by
			// the time the first one arrives, so a short timeout is
			// enough to drain them before the next case's send.
			first := c.ReadWithTimeout(2 * time.Second)
			if first == nil {
				t.Fatalf("%s: no reply at all — the request was silently dropped, leaving the client's action unresolved", tc.name)
			}
			if first[0] != serverpackets.OpcodeSystemMessage && first[0] != serverpackets.OpcodeActionFailed {
				t.Fatalf("%s: reply opcode = %#x, want a rejection frame", tc.name, first[0])
			}
			for c.ReadWithTimeout(100*time.Millisecond) != nil {
			}
		})
	}
}

func encodeRequestUnEquipItem(bodySlot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUnEquipItem)
	w.WriteInt32(bodySlot)
	return w.Bytes()
}
