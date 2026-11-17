package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestEnchantSelectionEndsWithTheSession pins the scroll selection living
// on the in-world character: a player who picks a scroll and then drops the
// connection comes back with none. After the relog the first walk sends no
// enchant cancel, RequestEnchantItem is silent and leaves weapon and
// scroll as they were, and RequestRestart lets the player out.
func TestEnchantSelectionEndsWithTheSession(t *testing.T) {
	t.Parallel()
	srv, objID, weapon, scroll := bootEnchanter(t, func() float64 { return 0.0 }, 0, false, nil)
	openEnchantSelection(t, srv.Client, scroll, 955)

	if err := srv.Client.Close(); err != nil {
		t.Fatal(err)
	}
	srv.AdvanceUntil(t, "player left world", func() bool {
		_, ok := srv.State.Player(objID)
		return !ok
	})
	c := srv.DialClient(t, "player1", 1)
	startInWorld(t, c)

	c.Send(encodeMoveBackwardToLocation(80, 70, 30, spawnX, spawnY, spawnZ))
	expectGroundClickAck(t, c)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMoveToLocation, "first walk after the relog")
	drainUntilQuiet(t, c)

	c.Send(encodeRequestEnchantItem(weapon))
	if queued := c.ReadQueued(); len(queued) != 0 {
		t.Fatalf("enchant after the relog replied %x, want no reply", queued[0])
	}
	if inst := mustFindItem(t, srv, objID, weapon); inst.EnchantLevel != 0 {
		t.Fatalf("weapon enchant after the relog = %d, want 0", inst.EnchantLevel)
	}
	if inst := mustFindItem(t, srv, objID, scroll); inst.Count != 1 {
		t.Fatalf("scroll count after the relog = %d, want 1", inst.Count)
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	for {
		reply := c.Read()
		if len(reply) == 0 || reply[0] != serverpackets.OpcodeRestartResponse {
			continue
		}
		if ok := wire.NewReader(reply[1:]).ReadInt32(); ok != 1 {
			t.Fatalf("RestartResponse result = %d, want 1 (no selection after the relog)", ok)
		}
		return
	}
}
