package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// expectGroundClickAck reads the ActionFailed a ground click away from any
// boat entrance is answered with ahead of its walk
// (MoveBackwardToLocation.java:121, Playable.tryToPassBoatEntrance).
func expectGroundClickAck(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	if frame := c.Read(); frame[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("ground click answered %#x first, want ActionFailed (%#x)", frame[0], serverpackets.OpcodeActionFailed)
	}
}
