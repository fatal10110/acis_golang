package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// assertStanceShown reads an NPC's stance switch as observers see it
// (Npc.setWalkOrRun): ChangeMoveType to running, then the NPC's NpcInfo.
func assertStanceShown(t *testing.T, c *scriptedClient, what string, objectID int32, running bool) {
	t.Helper()
	assertChangeMoveType(t, mustRead(t, c, what), objectID, running)
	assertStanceNpcInfo(t, mustRead(t, c, what+" NpcInfo"), objectID)
}

// assertStanceNpcInfo checks that frame is the NpcInfo of objectID a stance
// switch shows after its ChangeMoveType.
func assertStanceNpcInfo(t *testing.T, frame []byte, objectID int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeNPCInfo, "stance NpcInfo")
	if got := wireReader(frame[1:]).ReadInt32(); got != objectID {
		t.Fatalf("stance NpcInfo object id = %d, want %d", got, objectID)
	}
}
