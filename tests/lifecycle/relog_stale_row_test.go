package lifecycle

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestDestroyedStackDoesNotReturnOnRelog destroys a whole stack and relogs
// inside the lazy persistence window: no tick runs and the detach flush only
// writes the items the container still holds, so the destroyed row is still
// in the table when the character re-enters. The restore must treat a row
// with an unflushed change as stale and leave the stack gone, rather than
// handing it back and duplicating it.
func TestDestroyedStackDoesNotReturnOnRelog(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithCapturedLog(),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potions := srv.GiveItem(t, objID, 20, 5)
	startInWorld(t, c)

	c.Send(encodeRequestDestroyItem(potions, 5))
	testsupport.SyncBarrier(t, c, func() { c.Send(encodeRequestItemList()) }, serverpackets.OpcodeItemList)

	// Back to character selection with no flush of the pending delete, which
	// is what a logout inside the one-minute tick window looks like.
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, c, serverpackets.OpcodeCharSelectInfo)

	if !persistedRowExists(t, srv, objID, potions) {
		t.Fatalf("precondition failed: row %d was already deleted, so this run never entered the stale-row window the test covers", potions)
	}

	frames := startInWorld(t, c)
	entries := readItemListEntries(t, burstFrame(t, frames, serverpackets.OpcodeItemList))
	if e := findItemListEntry(entries, potions); e != nil {
		t.Fatalf("destroyed stack returned in the relog ItemList: %+v", *e)
	}
	// The drop is the one point in the login path that removes rows a
	// player may still own, so it has to be greppable: without this line a
	// detach flush that never landed would empty an inventory in silence.
	if !strings.Contains(srv.LogText(), "skipped item rows with an unflushed change") {
		t.Fatalf("no log line for the skipped stale row; login-side diagnostics missing")
	}
}

// readUntilOpcode reads frames until one carries opcode, so a caller can
// skip the acks that precede a state change without pinning them here.
func readUntilOpcode(t *testing.T, c *testsupport.ScriptedClient, opcode byte) []byte {
	t.Helper()
	for i := 0; i < 20; i++ {
		frame := c.Read()
		if frame[0] == opcode {
			return frame
		}
	}
	t.Fatalf("no frame with opcode %#x within 20 frames", opcode)
	return nil
}

// persistedRowExists reports whether ownerID still has an items row for
// objectID.
func persistedRowExists(t *testing.T, srv *gameservertest.Server, ownerID, objectID int32) bool {
	t.Helper()
	for _, inst := range persistedItems(t, srv, ownerID) {
		if inst.ObjectID == objectID {
			return true
		}
	}
	return false
}
