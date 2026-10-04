package network

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/fence"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

type fenceTestIDs struct{ next int32 }

func (f *fenceTestIDs) NextID() (int32, error) {
	f.next++
	return f.next, nil
}

// TestFenceActionsNeverGoSilent extends the guardrail of
// TestGameClientLinkNeverGoesSilentOnActionRequests to fences: a plain,
// shifted or forced click on a fence or one of its layers answers
// ActionFailed, as WorldObject.onAction does for a fence.
func TestFenceActionsNeverGoSilent(t *testing.T) {
	c, chars, _, state := newLinkedGameClient(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	chars.soleObjectID(t)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)
	testsupport.SyncBarrierFrames(t, c, func() { c.Send(encodeRequestManorList()) }, serverpackets.OpcodeExtended)

	// Placed far from the player, so its info does not interleave with
	// the barrier replies.
	f, err := fence.NewManager(engine.New(), state, &fenceTestIDs{next: 900000}).Add(geo.WorldXMax-5000, geo.WorldYMax-5000, 0, 2, 100, 100, 2)
	if err != nil {
		t.Fatalf("add fence: %v", err)
	}
	attack := func(id int32) []byte {
		w := wire.NewPacketWriter(clientpackets.OpcodeAttackRequest)
		w.WriteInt32(id)
		w.WriteInt32(0)
		w.WriteInt32(0)
		w.WriteInt32(0)
		w.WriteUint8(0)
		return w.Bytes()
	}
	for _, id := range []int32{f.ObjectID(), f.ObjectID() + 1} {
		for name, payload := range map[string][]byte{
			"Action":        encodeActionOn(id, false),
			"shift Action":  encodeActionOn(id, true),
			"AttackRequest": attack(id),
		} {
			frames := testsupport.SyncBarrierFrames(t, c, func() {
				c.Send(payload)
				c.Send(encodeRequestManorList())
			}, serverpackets.OpcodeExtended)
			got := make([]byte, len(frames))
			for i, frame := range frames {
				got[i] = frame[0]
			}
			if !bytes.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
				t.Fatalf("%s on %d: reply opcodes = %x, want ActionFailed", name, id, got)
			}
		}
	}
}
