package clan

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestClanWarehouseFirstOpenHoldsNoQueue holds the items table under a
// write lock while the leader opens the clan warehouse no member opened
// yet. While its rows are read, the leader's own queue still runs work
// posted to it; once the lock is released the withdrawal list comes with
// the stored rows, in the order an open of a restored warehouse answers.
func TestClanWarehouseFirstOpenHoldsNoQueue(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0, seedClan(t, clanSeed{level: 1}), keeperPage(t))
	stored := w.srv.GiveItem(t, seededClanID, potionID, 7)
	ctx := context.Background()
	if _, err := w.srv.DB.ExecContext(ctx, `UPDATE items SET loc = 'CLANWH' WHERE object_id = ?`, stored); err != nil {
		t.Fatal(err)
	}
	keeper := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("WarehouseKeeper", clanKeeperID), location.Location{X: w.at.X - 30, Y: w.at.Y, Z: w.at.Z})
	drainFrames(t, w.leader)
	ww := &whClanWorld{clanWorld: w, clanID: seededClanID, keeper: keeper}
	ww.talkToKeeper(t, w.leader, w.leaderID)

	unlock := lockItems(t, w.srv.DB)
	defer unlock()
	w.leader.Send(encodeBypass("npc_" + itoa(keeper.ObjectID()) + "_WithdrawC"))
	awaitItemReadOnLock(t, w.srv.DB)
	ran := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.leaderID).Post(func() { close(ran) }) {
		t.Fatal("the leader's queue refused the probe")
	}
	<-ran

	unlock()
	frames := drainFrames(t, w.leader)
	requireAnswer(t, "WithdrawC", frames, serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed)
	if whType, listed, _ := warehouseList(t, frames[0], serverpackets.OpcodeWarehouseWithdrawList); whType != 2 || len(listed) != 1 || listed[stored] != 7 {
		t.Fatalf("withdraw list = type %d %v, want type 2 with the 7 stored potions", whType, listed)
	}
}

// lockItems takes a write lock on the items table on its own connection
// and returns what releases it; releasing twice is harmless.
func lockItems(t *testing.T, db *sql.DB) func() {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `LOCK TABLES items WRITE`); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	released := false
	return func() {
		if released {
			return
		}
		released = true
		if _, err := conn.ExecContext(ctx, `UNLOCK TABLES`); err != nil {
			t.Error(err)
		}
		conn.Close()
	}
}

// awaitItemReadOnLock returns once a read of an owner's item rows is
// waiting on the items table lock, failing the test if none gets there.
func awaitItemReadOnLock(t *testing.T, db *sql.DB) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("no item read waited on the items table lock")
		}
		var n int
		if err := db.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM information_schema.PROCESSLIST
			  WHERE db = DATABASE() AND state LIKE 'Waiting for table%' AND info LIKE 'SELECT i.object_id%'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
	}
}
