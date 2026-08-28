package idfactory

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/rs/zerolog"
)

func seedRow(t *testing.T, db *sql.DB, stmt string, args ...any) {
	t.Helper()
	if _, err := db.Exec(stmt, args...); err != nil {
		t.Fatalf("seed %q: %v", stmt, err)
	}
}

// clearIDScanRows empties the ad hoc tables idScanTables creates, since they
// aren't in sqltest's truncate-between-tests list. Call it before seeding a
// test's own rows so an earlier test's leftovers can't affect assertions.
func clearIDScanRows(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, table := range []string{"clan_data", "clan_subpledges", "castle", "clanhall", "auctions", "mods_wedding", "petition"} {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			t.Fatalf("clear %s: %v", table, err)
		}
	}
}

func seedCharacter(t *testing.T, db *sql.DB, objectID int32) {
	t.Helper()
	seedRow(t, db,
		"INSERT INTO characters (obj_Id, char_name, online) VALUES (?, ?, ?)", objectID, "Debris", 1)
}

// idScanTables lists the remaining tables the boot id scan and repair pass
// read that the shared test database does not create; fresh installs get
// them from the shipped SQL baseline. clan_data carries every column an
// orphanRepairStatements UPDATE touches, since tests across this package
// share one CREATE TABLE IF NOT EXISTS for it.
var idScanTables = []string{
	"CREATE TABLE IF NOT EXISTS clan_data (" +
		"clan_id INT UNSIGNED NOT NULL DEFAULT 0, " +
		"leader_id INT UNSIGNED NOT NULL DEFAULT 0, " +
		"hasCastle INT NOT NULL DEFAULT 0, " +
		"auction_bid_at INT NOT NULL DEFAULT 0, " +
		"new_leader_id INT UNSIGNED NOT NULL DEFAULT 0" +
		")",
	"CREATE TABLE IF NOT EXISTS mods_wedding (id INT NOT NULL DEFAULT 0)",
	"CREATE TABLE IF NOT EXISTS petition (oid INT NOT NULL DEFAULT 0)",
	"CREATE TABLE IF NOT EXISTS clan_subpledges (" +
		"clan_id INT UNSIGNED NOT NULL DEFAULT 0, " +
		"leader_id INT UNSIGNED NOT NULL DEFAULT 0" +
		")",
	"CREATE TABLE IF NOT EXISTS castle (" +
		"id INT UNSIGNED NOT NULL DEFAULT 0, " +
		"currentTaxPercent INT NOT NULL DEFAULT 0, " +
		"nextTaxPercent INT NOT NULL DEFAULT 0" +
		")",
	"CREATE TABLE IF NOT EXISTS clanhall (" +
		"id INT UNSIGNED NOT NULL DEFAULT 0, " +
		"ownerId INT UNSIGNED NOT NULL DEFAULT 0, " +
		"paidUntil BIGINT NOT NULL DEFAULT 0, " +
		"paid INT NOT NULL DEFAULT 0, " +
		"sellerClanName VARCHAR(35) NOT NULL DEFAULT ''" +
		")",
	"CREATE TABLE IF NOT EXISTS auctions (" +
		"clanhall_id INT UNSIGNED NOT NULL DEFAULT 0, " +
		"clan_oid INT UNSIGNED NOT NULL DEFAULT 0" +
		")",
}

// TestBootCleanupRestoresDatabaseIntegrity drives the allocator's boot
// repair pass over a real database carrying crash debris: a stale online
// flag, an orphaned augmentation row, and expired skill-reuse timestamps.
// The repair must run before the id scan and leave none of it behind.
func TestBootCleanupRestoresDatabaseIntegrity(t *testing.T) {
	db := sqltest.SharedDB(t)
	ctx := context.Background()

	for _, stmt := range idScanTables {
		seedRow(t, db, stmt)
	}

	const charID = 0x10000001
	const liveItemID = 0x10000002
	seedCharacter(t, db, charID)
	seedRow(t, db,
		"INSERT INTO items (owner_id, object_id, item_id, count, loc) VALUES (?, ?, 57, 1, 'INVENTORY')", charID, liveItemID)

	// Orphaned by an item row that no longer exists.
	seedRow(t, db,
		"INSERT INTO augmentations (item_oid, attributes, skill_id, skill_level) VALUES (57391, 1, 1, 1)")
	seedRow(t, db,
		"INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, restore_type, systime) VALUES (?, 248, 3, 1, ?)",
		charID, time.Now().Add(-time.Hour).UnixMilli())

	if _, err := idfactory.New(ctx, db, zerolog.Nop()); err != nil {
		t.Fatalf("boot allocator: %v", err)
	}

	var online int
	if err := db.QueryRow("SELECT online FROM characters WHERE obj_Id = ?", charID).Scan(&online); err != nil {
		t.Fatalf("read online flag: %v", err)
	}
	if online != 0 {
		t.Fatalf("online flag after boot = %d, want 0", online)
	}

	var augmentations int
	if err := db.QueryRow("SELECT COUNT(*) FROM augmentations WHERE item_oid NOT IN (SELECT object_id FROM items)").Scan(&augmentations); err != nil {
		t.Fatalf("count orphan augmentations: %v", err)
	}
	if augmentations != 0 {
		t.Fatalf("orphaned augmentation rows survived boot: %d", augmentations)
	}

	var expired int
	if err := db.QueryRow("SELECT COUNT(*) FROM character_skills_save WHERE restore_type = 1 AND systime <= ?", time.Now().UnixMilli()).Scan(&expired); err != nil {
		t.Fatalf("count expired timestamps: %v", err)
	}
	if expired != 0 {
		t.Fatalf("expired reuse timestamps survived boot: %d", expired)
	}

	var rows int
	if err := db.QueryRow("SELECT COUNT(*) FROM characters WHERE obj_Id = ?", charID).Scan(&rows); err != nil {
		t.Fatalf("read character row: %v", err)
	}
	if rows != 1 {
		t.Fatalf("character row count = %d, want 1 (cleanup deletes orphans, not owners)", rows)
	}
}
