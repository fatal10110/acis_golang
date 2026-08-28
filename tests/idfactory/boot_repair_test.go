package idfactory

import (
	"context"
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/rs/zerolog"
)

// seedRepairSchema creates the tables the orphanRepairStatements reference
// and clears whatever rows earlier tests left behind, so each test starts
// from a known-empty state for them.
func seedRepairSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, stmt := range idScanTables {
		seedRow(t, db, stmt)
	}
	clearIDScanRows(t, db)
}

func bootRepair(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := idfactory.New(context.Background(), db, zerolog.Nop()); err != nil {
		t.Fatalf("boot allocator: %v", err)
	}
}

// TestBootRepairResetsClanDataAuctionBidAt covers
// "UPDATE clan_data SET auction_bid_at = 0 WHERE auction_bid_at NOT IN
// (SELECT clanhall_id FROM auctions)" (IdFactory.java:219-296).
func TestBootRepairResetsClanDataAuctionBidAt(t *testing.T) {
	db := sqltest.SharedDB(t)
	seedRepairSchema(t, db)

	const leaderID = 0x10000001
	const clanID = 1
	seedCharacter(t, db, leaderID)
	// leader_id must resolve to a real character, or the orphan-cleanup
	// pass (which runs before the repair pass) deletes this clan_data row
	// outright instead of exercising the repair statement.
	seedRow(t, db,
		"INSERT INTO clan_data (clan_id, leader_id, auction_bid_at) VALUES (?, ?, 999)", clanID, leaderID)

	bootRepair(t, db)

	var auctionBidAt int
	if err := db.QueryRow("SELECT auction_bid_at FROM clan_data WHERE clan_id = ?", clanID).Scan(&auctionBidAt); err != nil {
		t.Fatalf("read clan_data.auction_bid_at: %v", err)
	}
	if auctionBidAt != 0 {
		t.Fatalf("clan_data.auction_bid_at after boot = %d, want 0", auctionBidAt)
	}
}

// TestBootRepairResetsClanDataNewLeaderID covers
// "UPDATE clan_data SET new_leader_id = 0 WHERE new_leader_id NOT IN
// (SELECT obj_Id FROM characters)" (IdFactory.java:219-296).
func TestBootRepairResetsClanDataNewLeaderID(t *testing.T) {
	db := sqltest.SharedDB(t)
	seedRepairSchema(t, db)

	const leaderID = 0x10000001
	const danglingNewLeaderID = 0x10000099
	const clanID = 1
	seedCharacter(t, db, leaderID)
	seedRow(t, db,
		"INSERT INTO clan_data (clan_id, leader_id, new_leader_id) VALUES (?, ?, ?)", clanID, leaderID, danglingNewLeaderID)

	bootRepair(t, db)

	var newLeaderID int
	if err := db.QueryRow("SELECT new_leader_id FROM clan_data WHERE clan_id = ?", clanID).Scan(&newLeaderID); err != nil {
		t.Fatalf("read clan_data.new_leader_id: %v", err)
	}
	if newLeaderID != 0 {
		t.Fatalf("clan_data.new_leader_id after boot = %d, want 0", newLeaderID)
	}
}

// TestBootRepairResetsClanSubpledgesLeaderID covers
// "UPDATE clan_subpledges SET leader_id=0 WHERE clan_subpledges.leader_id
// NOT IN (SELECT obj_Id FROM characters) AND leader_id > 0"
// (IdFactory.java:219-296).
func TestBootRepairResetsClanSubpledgesLeaderID(t *testing.T) {
	db := sqltest.SharedDB(t)
	seedRepairSchema(t, db)

	const clanLeaderID = 0x10000001
	const danglingSubLeaderID = 0x10000099
	const clanID = 1
	seedCharacter(t, db, clanLeaderID)
	// clan_data.clan_id must exist, or the orphan-cleanup pass deletes this
	// clan_subpledges row before the repair pass runs.
	seedRow(t, db,
		"INSERT INTO clan_data (clan_id, leader_id) VALUES (?, ?)", clanID, clanLeaderID)
	seedRow(t, db,
		"INSERT INTO clan_subpledges (clan_id, leader_id) VALUES (?, ?)", clanID, danglingSubLeaderID)

	bootRepair(t, db)

	var subLeaderID int
	if err := db.QueryRow("SELECT leader_id FROM clan_subpledges WHERE clan_id = ?", clanID).Scan(&subLeaderID); err != nil {
		t.Fatalf("read clan_subpledges.leader_id: %v", err)
	}
	if subLeaderID != 0 {
		t.Fatalf("clan_subpledges.leader_id after boot = %d, want 0", subLeaderID)
	}
}

// TestBootRepairResetsCastleTax covers
// "UPDATE castle SET currentTaxPercent=0, nextTaxPercent=0 WHERE castle.id
// NOT IN (SELECT hasCastle FROM clan_data)" (IdFactory.java:219-296).
func TestBootRepairResetsCastleTax(t *testing.T) {
	db := sqltest.SharedDB(t)
	seedRepairSchema(t, db)

	const castleID = 3
	seedRow(t, db,
		"INSERT INTO castle (id, currentTaxPercent, nextTaxPercent) VALUES (?, 10, 20)", castleID)

	bootRepair(t, db)

	var currentTax, nextTax int
	if err := db.QueryRow("SELECT currentTaxPercent, nextTaxPercent FROM castle WHERE id = ?", castleID).Scan(&currentTax, &nextTax); err != nil {
		t.Fatalf("read castle tax: %v", err)
	}
	if currentTax != 0 || nextTax != 0 {
		t.Fatalf("castle tax after boot = (%d, %d), want (0, 0)", currentTax, nextTax)
	}
}

// TestBootRepairResetsCharactersClanID covers
// "UPDATE characters SET clanid=0 WHERE characters.clanid NOT IN (SELECT
// clan_id FROM clan_data)" (IdFactory.java:219-296).
func TestBootRepairResetsCharactersClanID(t *testing.T) {
	db := sqltest.SharedDB(t)
	seedRepairSchema(t, db)

	const charID = 0x10000001
	const danglingClanID = 42
	seedCharacter(t, db, charID)
	seedRow(t, db, "UPDATE characters SET clanid = ? WHERE obj_Id = ?", danglingClanID, charID)

	bootRepair(t, db)

	var clanID int
	if err := db.QueryRow("SELECT clanid FROM characters WHERE obj_Id = ?", charID).Scan(&clanID); err != nil {
		t.Fatalf("read characters.clanid: %v", err)
	}
	if clanID != 0 {
		t.Fatalf("characters.clanid after boot = %d, want 0", clanID)
	}
}

// TestBootRepairResetsClanhallOwner covers
// "UPDATE clanhall SET ownerId=0, paidUntil=0, paid=0 WHERE
// clanhall.ownerId NOT IN (SELECT clan_id FROM clan_data)"
// (IdFactory.java:219-296).
func TestBootRepairResetsClanhallOwner(t *testing.T) {
	db := sqltest.SharedDB(t)
	seedRepairSchema(t, db)

	const hallID = 5
	const danglingOwnerID = 42
	seedRow(t, db,
		"INSERT INTO clanhall (id, ownerId, paidUntil, paid) VALUES (?, ?, 123456789, 1)", hallID, danglingOwnerID)

	bootRepair(t, db)

	var ownerID, paidUntil, paid int
	if err := db.QueryRow("SELECT ownerId, paidUntil, paid FROM clanhall WHERE id = ?", hallID).Scan(&ownerID, &paidUntil, &paid); err != nil {
		t.Fatalf("read clanhall row: %v", err)
	}
	if ownerID != 0 || paidUntil != 0 || paid != 0 {
		t.Fatalf("clanhall (ownerId, paidUntil, paid) after boot = (%d, %d, %d), want (0, 0, 0)", ownerID, paidUntil, paid)
	}
}
