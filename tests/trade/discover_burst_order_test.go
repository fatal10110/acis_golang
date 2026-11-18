package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestEnteringPlayerSeesStoreOwnerInDiscoveryOrder pins the burst a player
// is sent when it comes to know a player with an open sell store, as the
// reference's Player.sendInfo orders it: the owner's CharInfo, then the
// owner's RelationChanged to the viewer, then the store message, with
// nothing between them for an owner without a summon, seat or boat. The
// owner's own client is sent the viewer's relation to the owner in the
// same step.
func TestEnteringPlayerSeesStoreOwnerInDiscoveryOrder(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	potions := h.srv.GiveItem(t, h.firstID, potionID, 10)
	startInWorld(t, h.first)
	drainUntilQuiet(t, h.first)

	h.first.Send(encodeRequestActionUse(actionStoreSell))
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodePrivateStoreManageListSell, "PrivateStoreManageListSell")
	h.first.Send(encodeSetPrivateStoreListSell(false, sellRow{potions, 4, 50}))
	h.srv.Settle(t)
	drainUntilQuiet(t, h.first)

	h.second.Send(encodeRequestGameStart(0))
	h.second.Read() // SSQInfo
	h.second.Read() // CharSelected
	h.second.Send(encodeEnterWorld())
	h.srv.Settle(t)
	viewerFrames := h.second.ReadQueued()
	ownerFrames := drainFrames(t, h.first)

	info := frameIndex(viewerFrames, serverpackets.OpcodeCharInfo, h.firstID)
	if info < 0 {
		t.Fatalf("viewer frames %x carry no CharInfo of the store owner", opcodeList(viewerFrames))
	}
	if info+2 >= len(viewerFrames) ||
		!isAbout(viewerFrames[info+1], serverpackets.OpcodeRelationChanged, h.firstID) ||
		!isAbout(viewerFrames[info+2], serverpackets.OpcodePrivateStoreMsgSell, h.firstID) {
		t.Fatalf("viewer frames from the owner's CharInfo = %x, want CharInfo, RelationChanged, PrivateStoreMsgSell of the owner", opcodeList(viewerFrames[info:]))
	}
	// Both players discover each other, and each discovery tells both
	// clients the relation, so the owner is sent the viewer's twice.
	if n := countAbout(ownerFrames, serverpackets.OpcodeRelationChanged, h.secondID); n != 2 {
		t.Fatalf("owner frames %x carry %d RelationChanged of the viewer, want 2", opcodeList(ownerFrames), n)
	}
}

// frameIndex returns the index of the first frame with opcode that describes
// objectID, -1 when there is none. A CharInfo carries the object id after
// its position and heading; every other frame leads with it.
func frameIndex(frames [][]byte, opcode byte, objectID int32) int {
	for i, f := range frames {
		if isAbout(f, opcode, objectID) {
			return i
		}
	}
	return -1
}

func countAbout(frames [][]byte, opcode byte, objectID int32) int {
	n := 0
	for _, f := range frames {
		if isAbout(f, opcode, objectID) {
			n++
		}
	}
	return n
}

func isAbout(frame []byte, opcode byte, objectID int32) bool {
	if frame[0] != opcode {
		return false
	}
	r := wire.NewReader(frame[1:])
	if opcode == serverpackets.OpcodeCharInfo {
		for range 4 {
			r.ReadInt32()
		}
	}
	return r.ReadInt32() == objectID
}
