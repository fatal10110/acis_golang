package clan

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestAdminPledgeDismissReadsWarehouseOffTheQueue holds the items table
// under a write lock while a game master dismisses a clan whose warehouse
// no member opened yet. While its rows are read, the game master's queue
// and the selected leader's still run work posted to them; once the lock is
// released the clan is dismissed as a dismiss of a restored warehouse is,
// and the warehouse's items are destroyed.
func TestAdminPledgeDismissReadsWarehouseOffTheQueue(t *testing.T) {
	w := bootPledgeWorld(t, clanSeed{level: 5})
	w.recruit(t)
	w.selectFor(t, w.leaderID)
	stored := w.srv.GiveItem(t, seededClanID, potionID, 7)
	if _, err := w.srv.DB.ExecContext(context.Background(), `UPDATE items SET loc = 'CLANWH' WHERE object_id = ?`, stored); err != nil {
		t.Fatal(err)
	}

	unlock := lockItems(t, w.srv.DB)
	defer unlock()
	w.gm.Send(encodeBuildCmd("pledge dismiss"))
	awaitItemReadOnLock(t, w.srv.DB)
	for _, id := range []int32{w.gmID, w.leaderID} {
		ran := make(chan struct{})
		if !w.srv.PlayerQueue(t, id).Post(func() { close(ran) }) {
			t.Fatalf("the queue of %d refused the probe", id)
		}
		<-ran
	}

	unlock()
	wantAnswer(t, "//pledge dismiss", pledgeAnswer(t, drainFrames(t, w.gm)), "The clan is now disbanded.", gameMenu)
	if got := messages(t, drainFrames(t, w.leader)); len(got) == 0 || got[0] != serverpackets.SystemMessageClanHasDispersed {
		t.Fatalf("leader messages = %v, want CLAN_HAS_DISPERSED first", got)
	}
	w.srv.FlushPersistence(t)
	if n := queryInt(t, w.clanWorld, `SELECT COUNT(*) FROM items WHERE object_id = ?`, stored); n != 0 {
		t.Fatalf("dismissed clan's warehouse rows = %d, want 0", n)
	}
	if n := queryInt(t, w.clanWorld, `SELECT COUNT(*) FROM clan_data WHERE clan_id = ?`, seededClanID); n != 0 {
		t.Fatalf("dismissed clan rows = %d, want 0", n)
	}
}
