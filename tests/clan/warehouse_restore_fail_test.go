package clan

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestClanWarehouseFirstOpenReadFailureReleasesClient kills the read of
// the clan warehouse rows that the leader's first open waits on. The
// client is only released: no withdrawal list, and no warehouse becomes
// the active store, so a withdrawal sent next moves nothing. The failed
// read is not kept, so once the table is free the next open reads the
// rows again and lists the stored potions.
func TestClanWarehouseFirstOpenReadFailureReleasesClient(t *testing.T) {
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
	heldBefore := ww.held(t, w.leaderID, potionID)

	unlock := lockItems(t, w.srv.DB)
	defer unlock()
	w.leader.Send(encodeBypass("npc_" + itoa(keeper.ObjectID()) + "_WithdrawC"))
	awaitItemReadOnLock(t, w.srv.DB)
	killItemRead(t, w.srv.DB)
	frames := drainFrames(t, w.leader)
	requireAnswer(t, "WithdrawC with a failed read", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed)

	w.leader.Send(encodeWarehouseRows(clientpackets.OpcodeSendWarehouseWithdraw, whRow{stored, 7}))
	drainFrames(t, w.leader)
	if got := ww.held(t, w.leaderID, potionID); got != heldBefore {
		t.Fatalf("leader holds %d potions after a withdrawal with no open warehouse, want %d", got, heldBefore)
	}

	unlock()
	frames = ww.keeperCommand(t, w.leader, "WithdrawC")
	requireAnswer(t, "WithdrawC after the failed read", frames, serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed)
	if whType, listed, _ := warehouseList(t, frames[0], serverpackets.OpcodeWarehouseWithdrawList); whType != 2 || len(listed) != 1 || listed[stored] != 7 {
		t.Fatalf("withdraw list = type %d %v, want type 2 with the 7 stored potions", whType, listed)
	}
}

// killItemRead interrupts the read of an owner's item rows that waits on
// the items table lock (see awaitItemReadOnLock), so it fails.
func killItemRead(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM information_schema.PROCESSLIST
		  WHERE db = DATABASE() AND state LIKE 'Waiting for table%' AND info LIKE 'SELECT i.object_id%' LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "KILL QUERY "+strconv.FormatInt(id, 10)); err != nil {
		t.Fatal(err)
	}
}
