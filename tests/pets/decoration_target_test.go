package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSelectingDecorationSendsValidateLocationFirst clicks a spawned
// Christmas Tree: like any creature target it answers ValidateLocation
// before MyTargetSelected.
func TestSelectingDecorationSendsValidateLocationFirst(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: treeKitID, Count: 1})
	h.client.Send(encodeUseItem(h.seededItem(t, treeKitID), false))
	frame := mustRead(t, h.client, "tree NPCInfo")
	assertFrameOpcode(t, frame, serverpackets.OpcodeNPCInfo, "tree NPCInfo")
	treeID := wire.NewReader(frame[1:]).ReadInt32()
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeAction(treeID, 0, 0, 0, false))
	frame = mustRead(t, h.client, "tree ValidateLocation")
	assertFrameOpcode(t, frame, serverpackets.OpcodeValidateLocation, "tree ValidateLocation")
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != treeID {
		t.Fatalf("ValidateLocation object id = %d, want tree %d", got, treeID)
	}
	frame = mustRead(t, h.client, "tree MyTargetSelected")
	assertFrameOpcode(t, frame, serverpackets.OpcodeMyTargetSelected, "tree MyTargetSelected")
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != treeID {
		t.Fatalf("MyTargetSelected object id = %d, want tree %d", got, treeID)
	}
}
