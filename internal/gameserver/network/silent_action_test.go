package network

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestGameClientLinkNeverGoesSilentOnActionRequests is the guardrail against
// the bug class behind #828/#829/#873: an accepted client action packet that
// a handler quietly drops, leaving the client's pending action unresolved —
// which presented as a character that walks up to a target and freezes, a
// picked-up item that never leaves the ground, or an item-window click that
// does nothing. Every case here sends a request built to be rejected (a
// nonexistent object id, an unclaimed action id, a command with no target to
// act on) and asserts the exact rejection frames that come back, read up to a
// manor-list barrier so nothing is left in flight for the next case. A new
// rejection reason or message updates the case's want list.
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
		want    []byte // reply opcodes, in wire order
	}{
		{"UseItem on an object the player doesn't hold", encodeUseItem(missingObjectID, false), []byte{serverpackets.OpcodeActionFailed}},
		{"RequestUnEquipItem for an empty body slot", encodeRequestUnEquipItem(0), []byte{serverpackets.OpcodeActionFailed}},
		{"RequestActionUse with an action id no handler claims", encodeRequestActionUse(9999, false, false), []byte{serverpackets.OpcodeActionFailed}},
		{"RequestActionUse pet command with no active summon", encodeRequestActionUse(16, false, false), []byte{serverpackets.OpcodeActionFailed}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			frames := testsupport.SyncBarrierFrames(t, c, func() {
				c.Send(tc.payload)
				c.Send(encodeRequestManorList())
			}, serverpackets.OpcodeExtended)
			if len(frames) == 0 {
				t.Fatalf("%s: no reply at all — the request was silently dropped, leaving the client's action unresolved", tc.name)
			}
			got := make([]byte, len(frames))
			for i, f := range frames {
				got[i] = f[0]
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("%s: reply opcodes = %x, want %x", tc.name, got, tc.want)
			}
		})
	}
}

func encodeRequestUnEquipItem(bodySlot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUnEquipItem)
	w.WriteInt32(bodySlot)
	return w.Bytes()
}
