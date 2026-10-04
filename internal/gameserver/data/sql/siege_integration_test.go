package sql

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// TestSiegeStoreRoundTrip pins the siege_clans statements of Siege.java:
// a registration is inserted or has its type replaced (ADD_OR_UPDATE),
// one clan, the pending clans or every clan of one castle are deleted,
// and a row whose type names no side is skipped on load. It also pins
// UPDATE_SIEGE_INFOS: the castle's siegeDate and regTimeOver as 'true' or
// 'false'.
func TestSiegeStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewSiegeStore(db)

	for _, err := range []error{
		store.SaveClan(ctx, 1, 0x10000001, siege.SidePending),
		store.SaveClan(ctx, 1, 0x10000002, siege.SideAttacker),
		store.SaveClan(ctx, 1, 0x10000003, siege.SidePending),
		store.SaveClan(ctx, 2, 0x10000004, siege.SideAttacker),
		store.SaveClan(ctx, 1, 0x10000001, siege.SideDefender),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (3, 0x10000005, 'BOGUS')`); err != nil {
		t.Fatal(err)
	}
	load := func(want ...siege.ClanRow) {
		t.Helper()
		rows, err := store.LoadClans(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(rows, want) {
			t.Fatalf("LoadClans() = %+v, want %+v", rows, want)
		}
	}
	load(
		siege.ClanRow{CastleID: 1, ClanID: 0x10000001, Side: siege.SideDefender},
		siege.ClanRow{CastleID: 1, ClanID: 0x10000002, Side: siege.SideAttacker},
		siege.ClanRow{CastleID: 1, ClanID: 0x10000003, Side: siege.SidePending},
		siege.ClanRow{CastleID: 2, ClanID: 0x10000004, Side: siege.SideAttacker},
	)

	if err := store.DeletePending(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteClan(ctx, 1, 0x10000002); err != nil {
		t.Fatal(err)
	}
	load(
		siege.ClanRow{CastleID: 1, ClanID: 0x10000001, Side: siege.SideDefender},
		siege.ClanRow{CastleID: 2, ClanID: 0x10000004, Side: siege.SideAttacker},
	)
	if err := store.DeleteClans(ctx, 2); err != nil {
		t.Fatal(err)
	}
	load(siege.ClanRow{CastleID: 1, ClanID: 0x10000001, Side: siege.SideDefender})

	castles := NewCastleStore(db)
	if err := castles.UpdateSiegeInfo(ctx, 4, 1_792_000_000_000, false); err != nil {
		t.Fatal(err)
	}
	var date int64
	var over string
	if err := db.QueryRowContext(ctx, `SELECT siegeDate, regTimeOver FROM castle WHERE id=4`).Scan(&date, &over); err != nil {
		t.Fatal(err)
	}
	if date != 1_792_000_000_000 || over != "false" {
		t.Fatalf("castle 4 siege info = %d/%q, want 1792000000000/\"false\"", date, over)
	}
	if err := castles.UpdateSiegeInfo(ctx, 4, 1_792_000_000_000, true); err != nil {
		t.Fatal(err)
	}
	rows, err := castles.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[3]; got.ID != 4 || got.SiegeDate != 1_792_000_000_000 || !got.RegTimeOver {
		t.Fatalf("castle 4 row = %+v", got)
	}
}
