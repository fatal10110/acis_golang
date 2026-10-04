package sql

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
)

// TestCastleStoreRoundTrip pins the castle table and the clan_data castle
// column: the shipped rows load (rates 15, 300 certificates, registration
// over), each single-column write lands on its castle only, an owner
// change clears every row holding the castle before giving it to the new
// clan, and only an offline owner's equipped circlet and Lord's Crown go
// back to the inventory.
func TestCastleStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := sqltest.SharedDB(t)
	store := NewCastleStore(db)

	rows, err := store.Load(ctx)
	if err != nil || len(rows) != 9 {
		t.Fatalf("Load() = %d rows, %v; want the 9 shipped", len(rows), err)
	}
	if want := (castle.Row{ID: 1, CurrentTaxPercent: 15, NextTaxPercent: 15, RegTimeOver: true, LeftCertificates: 300}); rows[0] != want {
		t.Fatalf("row 1 = %+v, want %+v", rows[0], want)
	}

	for _, err := range []error{
		store.UpdateTreasury(ctx, 2, 1_000_000_000_000),
		store.UpdateTaxRevenue(ctx, 2, 30),
		store.UpdateSeedIncome(ctx, 2, 7),
		store.UpdateCurrentTax(ctx, 2, 10),
		store.UpdateNextTax(ctx, 2, 20),
		store.UpdateCertificates(ctx, 2, 120),
		store.UpdateFinances(ctx, 3, castle.Finances{Treasury: 5, TaxRevenue: 6, SeedIncome: 8, CurrentTaxPercent: 1, NextTaxPercent: 2}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE castle SET siegeDate=1767225600000, regTimeOver='false' WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	rows, err = store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []castle.Row{
		{ID: 2, CurrentTaxPercent: 10, NextTaxPercent: 20, Treasury: 1_000_000_000_000, TaxRevenue: 30, SeedIncome: 7, SiegeDate: 1767225600000, LeftCertificates: 120},
		{ID: 3, CurrentTaxPercent: 1, NextTaxPercent: 2, Treasury: 5, TaxRevenue: 6, SeedIncome: 8, RegTimeOver: true, LeftCertificates: 300},
	}
	if !slices.Equal(rows[1:3], want) {
		t.Fatalf("rows 2-3 = %+v, want %+v", rows[1:3], want)
	}

	for _, q := range []string{
		`INSERT INTO clan_data (clan_id, clan_name, hasCastle) VALUES (0x10000001, 'Lords', 4), (0x10000002, 'Rivals', 0), (0x10000003, 'Kings', 5)`,
		`INSERT INTO items (owner_id, object_id, item_id, count, loc, loc_data) VALUES
			(0x20000001, 0x30000001, 6837, 1, 'PAPERDOLL', 14), (0x20000001, 0x30000002, 6841, 1, 'PAPERDOLL', 14),
			(0x20000001, 0x30000003, 6838, 1, 'PAPERDOLL', 14), (0x20000001, 0x30000004, 6837, 1, 'WAREHOUSE', 0),
			(0x20000002, 0x30000005, 6837, 1, 'PAPERDOLL', 14)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.UpdateOwner(ctx, 4, 0x10000002); err != nil {
		t.Fatal(err)
	}
	owners, err := store.LoadOwners(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []castle.Owner{{ClanID: 0x10000002, CastleID: 4}, {ClanID: 0x10000003, CastleID: 5}}; !slices.Equal(owners, want) {
		t.Fatalf("owners after transfer = %+v, want %+v", owners, want)
	}
	if err := store.UpdateOwner(ctx, 4, 0); err != nil {
		t.Fatal(err)
	}
	if owners, _ = store.LoadOwners(ctx); !slices.Equal(owners, []castle.Owner{{ClanID: 0x10000003, CastleID: 5}}) {
		t.Fatalf("owners after removal = %+v", owners)
	}

	if err := store.UnequipCirclets(ctx, 6837, 0x20000001); err != nil {
		t.Fatal(err)
	}
	locs := map[int]string{}
	itemRows, err := db.QueryContext(ctx, `SELECT object_id, loc FROM items ORDER BY object_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer itemRows.Close()
	for itemRows.Next() {
		var id int
		var loc string
		if err := itemRows.Scan(&id, &loc); err != nil {
			t.Fatal(err)
		}
		locs[id] = loc
	}
	wantLocs := map[int]string{
		0x30000001: "INVENTORY", 0x30000002: "INVENTORY", 0x30000003: "PAPERDOLL", 0x30000004: "WAREHOUSE", 0x30000005: "PAPERDOLL",
	}
	for id, want := range wantLocs {
		if locs[id] != want {
			t.Fatalf("item %#x loc = %q, want %q", id, locs[id], want)
		}
	}
}
